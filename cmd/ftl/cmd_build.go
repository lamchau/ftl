package main

import (
	"context"
	"fmt"

	"github.com/TBD54566975/ftl/backend/common/exec"
	"github.com/TBD54566975/ftl/backend/common/log"
	"github.com/TBD54566975/ftl/backend/common/moduleconfig"
)

type buildCmd struct {
	ModuleDir string `arg:"" help:"Directory containing ftl.toml" type:"existingdir" default:"."`
}

func (b *buildCmd) Run(ctx context.Context) error {
	// Load the TOML file.
	config, err := moduleconfig.LoadConfig(b.ModuleDir)
	if err != nil {
		return err
	}

	switch config.Language {
	case "kotlin":
		return b.buildKotlin(ctx, config)
	default:
		return fmt.Errorf("unable to build. unknown language %q", config.Language)
	}
}

func (b *buildCmd) buildKotlin(ctx context.Context, config moduleconfig.ModuleConfig) error {
	logger := log.FromContext(ctx)

	logger.Infof("Building kotlin module '%s'", config.Module)
	logger.Infof("Using build command '%s'", config.Build)

	err := exec.Command(ctx, logger.GetLevel(), b.ModuleDir, "bash", "-c", config.Build).Run()
	if err != nil {
		return err
	}

	return nil
}
