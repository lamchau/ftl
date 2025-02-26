package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	osExec "os/exec" //nolint:depguard
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"github.com/alecthomas/kong"
	"golang.org/x/sync/errgroup"

	"github.com/TBD54566975/ftl/backend/controller"
	"github.com/TBD54566975/ftl/backend/controller/scaling/localscaling"
	"github.com/TBD54566975/ftl/backend/controller/sql/databasetesting"
	ftlv1 "github.com/TBD54566975/ftl/backend/protos/xyz/block/ftl/v1"
	"github.com/TBD54566975/ftl/backend/protos/xyz/block/ftl/v1/ftlv1connect"
	"github.com/TBD54566975/ftl/internal/bind"
	"github.com/TBD54566975/ftl/internal/exec"
	"github.com/TBD54566975/ftl/internal/log"
	"github.com/TBD54566975/ftl/internal/rpc"
	"github.com/TBD54566975/ftl/internal/slices"
)

type serveCmd struct {
	Bind           *url.URL      `help:"Starting endpoint to bind to and advertise to. Each controller and runner will increment the port by 1" default:"http://localhost:8892"`
	AllowOrigins   []*url.URL    `help:"Allow CORS requests to ingress endpoints from these origins." env:"FTL_CONTROLLER_ALLOW_ORIGIN"`
	NoConsole      bool          `help:"Disable the console."`
	DBPort         int           `help:"Port to use for the database." default:"54320"`
	Recreate       bool          `help:"Recreate the database even if it already exists." default:"false"`
	Controllers    int           `short:"c" help:"Number of controllers to start." default:"1"`
	Runners        int           `short:"r" help:"Number of runners to start." default:"0"`
	Background     bool          `help:"Run in the background." default:"false"`
	Stop           bool          `help:"Stop the running FTL instance. Can be used to --background to restart the server" default:"false"`
	StartupTimeout time.Duration `help:"Timeout for the server to start up." default:"20s"`
}

const ftlContainerName = "ftl-db-1"

func (s *serveCmd) Run(ctx context.Context) error {
	logger := log.FromContext(ctx)
	client := rpc.ClientFromContext[ftlv1connect.ControllerServiceClient](ctx)

	if s.Background {
		if s.Stop {
			// allow usage of --background and --stop together to "restart" the background process
			// ignore error here if the process is not running
			_ = killBackgroundProcess(logger)
		}

		runInBackground(logger)

		err := s.pollControllerOnine(ctx, client)
		if err != nil {
			return err
		}

		os.Exit(0)
	}

	if s.Stop {
		return killBackgroundProcess(logger)
	}

	logger.Infof("Starting FTL with %d controller(s) and %d runner(s)", s.Controllers, s.Runners)

	dsn, err := s.setupDB(ctx)
	if err != nil {
		return err
	}

	wg, ctx := errgroup.WithContext(ctx)

	bindAllocator, err := bind.NewBindAllocator(s.Bind)
	if err != nil {
		return err
	}

	controllerAddresses := make([]*url.URL, 0, s.Controllers)
	for i := 0; i < s.Controllers; i++ {
		controllerAddresses = append(controllerAddresses, bindAllocator.Next())
	}

	runnerScaling, err := localscaling.NewLocalScaling(bindAllocator, controllerAddresses)
	if err != nil {
		return err
	}

	for i := 0; i < s.Controllers; i++ {
		i := i
		config := controller.Config{
			Bind:         controllerAddresses[i],
			DSN:          dsn,
			AllowOrigins: s.AllowOrigins,
			NoConsole:    s.NoConsole,
		}
		if err := kong.ApplyDefaults(&config); err != nil {
			return err
		}

		scope := fmt.Sprintf("controller%d", i)
		controllerCtx := log.ContextWithLogger(ctx, logger.Scope(scope))

		wg.Go(func() error {
			if err := controller.Start(controllerCtx, config, runnerScaling); err != nil {
				return fmt.Errorf("controller%d failed: %w", i, err)
			}
			return nil
		})
	}

	err = runnerScaling.SetReplicas(ctx, s.Runners, nil)
	if err != nil {
		return err
	}

	if err := wg.Wait(); err != nil {
		return fmt.Errorf("serve failed: %w", err)
	}
	return nil
}

func runInBackground(logger *log.Logger) {
	if running, _ := isServeRunning(logger); running {
		logger.Warnf("'ftl serve' is already running in the background. Use --stop to stop it.")
		return
	}

	args := make([]string, 0, len(os.Args))
	for _, arg := range os.Args[1:] {
		if arg == "--background" || arg == "--stop" {
			continue
		}
		args = append(args, arg)
	}

	cmd := osExec.Command(os.Args[0], args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		logger.Errorf(err, "failed to start background process")
		return
	}

	pidFilePath, _ := pidFilePath()
	if err := os.MkdirAll(filepath.Dir(pidFilePath), 0750); err != nil {
		logger.Errorf(err, "failed to create directory for pid file")
		return
	}

	if err := os.WriteFile(pidFilePath, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0600); err != nil {
		logger.Errorf(err, "failed to write pid file")
		return
	}

	logger.Infof("`ftl serve` running in background with pid: %d", cmd.Process.Pid)
}

func killBackgroundProcess(logger *log.Logger) error {
	pidFilePath, err := pidFilePath()
	if err != nil {
		logger.Infof("No background process found")
		return err
	}

	pid, err := getPIDFromPath(pidFilePath)
	if err != nil || pid == 0 {
		logger.Debugf("FTL serve is not running in the background")
		return nil
	}

	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		if !errors.Is(err, syscall.ESRCH) {
			return err
		}
	}

	if err := os.Remove(pidFilePath); err != nil {
		logger.Errorf(err, "Failed to remove pid file: %v", err)
	}

	logger.Infof("`ftl serve` stopped (pid: %d)", pid)
	return nil
}

func pidFilePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".ftl", "ftl-serve.pid"), nil
}

func getPIDFromPath(path string) (int, error) {
	pidBytes, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(string(pidBytes))
	if err != nil {
		return 0, err
	}
	return pid, nil
}

func isServeRunning(logger *log.Logger) (bool, error) {
	pidFilePath, err := pidFilePath()
	if err != nil {
		return false, err
	}

	pid, err := getPIDFromPath(pidFilePath)
	if err != nil || pid == 0 {
		return false, err
	}

	err = syscall.Kill(pid, 0)
	if err != nil {
		if errors.Is(err, syscall.ESRCH) {
			logger.Infof("Process with PID %d does not exist.", pid)
			return false, nil
		}
		if errors.Is(err, syscall.EPERM) {
			logger.Infof("Process with PID %d exists but no permission to signal it.", pid)
			return true, nil
		}
		return false, err
	}

	return true, nil
}

func (s *serveCmd) setupDB(ctx context.Context) (string, error) {
	logger := log.FromContext(ctx)

	nameFlag := fmt.Sprintf("name=^/%s$", ftlContainerName)
	output, err := exec.Capture(ctx, ".", "docker", "ps", "-a", "--filter", nameFlag, "--format", "{{.Names}}")
	if err != nil {
		logger.Errorf(err, "%s", output)
		return "", err
	}

	recreate := s.Recreate
	port := strconv.Itoa(s.DBPort)

	if len(output) == 0 {
		logger.Debugf("Creating docker container '%s' for postgres db", ftlContainerName)

		// check if port s.DBPort is already in use
		_, err := exec.Capture(ctx, ".", "sh", "-c", fmt.Sprintf("lsof -i:%d", s.DBPort))
		if err == nil {
			return "", fmt.Errorf("port %d is already in use", s.DBPort)
		}

		err = exec.Command(ctx, log.Debug, "./", "docker", "run",
			"-d", // run detached so we can follow with other commands
			"--name", ftlContainerName,
			"--user", "postgres",
			"--restart", "always",
			"-e", "POSTGRES_PASSWORD=secret",
			"-p", fmt.Sprintf("%s:5432", port),
			"--health-cmd=pg_isready",
			"--health-interval=1s",
			"--health-timeout=60s",
			"--health-retries=60",
			"--health-start-period=80s",
			"postgres:latest", "postgres",
		).RunBuffered(ctx)
		if err != nil {
			return "", err
		}

		recreate = true
	} else {
		// Start the existing container
		_, err = exec.Capture(ctx, ".", "docker", "start", ftlContainerName)
		if err != nil {
			return "", err
		}

		// Grab the port from the existing container
		portOutput, err := exec.Capture(ctx, ".", "docker", "port", ftlContainerName, "5432/tcp")
		if err != nil {
			logger.Errorf(err, "%s", portOutput)
			return "", err
		}
		port = slices.Reduce(strings.Split(string(portOutput), "\n"), "", func(port string, line string) string {
			if parts := strings.Split(line, ":"); len(parts) == 2 {
				return parts[1]
			}
			return port
		})

		logger.Debugf("Reusing existing docker container %q on port %q for postgres db", ftlContainerName, port)
	}

	err = pollContainerHealth(ctx, ftlContainerName, 10*time.Second)
	if err != nil {
		return "", err
	}

	dsn := fmt.Sprintf("postgres://postgres:secret@localhost:%s/ftl?sslmode=disable", port)
	logger.Debugf("Postgres DSN: %s", dsn)

	_, err = databasetesting.CreateForDevel(ctx, dsn, recreate)
	if err != nil {
		return "", err
	}

	return dsn, nil
}

func pollContainerHealth(ctx context.Context, containerName string, timeout time.Duration) error {
	logger := log.FromContext(ctx)
	logger.Debugf("Waiting for %s to be healthy", containerName)

	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-pollCtx.Done():
			return errors.New("timed out waiting for container to be healthy")

		case <-time.After(1 * time.Millisecond):
			output, err := exec.Capture(pollCtx, ".", "docker", "inspect", "--format", "{{.State.Health.Status}}", containerName)
			if err != nil {
				return err
			}

			status := strings.TrimSpace(string(output))
			if status == "healthy" {
				return nil
			}
		}
	}
}

func (s *serveCmd) pollControllerOnine(ctx context.Context, client ftlv1connect.ControllerServiceClient) error {
	logger := log.FromContext(ctx)

	ctx, cancel := context.WithTimeout(ctx, s.StartupTimeout)
	defer cancel()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			_, err := client.Status(ctx, connect.NewRequest(&ftlv1.StatusRequest{
				AllControllers: true,
			}))
			if err != nil {
				logger.Tracef("Error getting status, retrying...: %v", err)
				continue // retry
			}

			return nil

		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				logger.Errorf(ctx.Err(), "Timeout reached while polling for controller status")
			}

			return ctx.Err()
		}
	}
}
