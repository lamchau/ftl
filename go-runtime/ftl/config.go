package ftl

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/TBD54566975/ftl/common/configuration"
)

// ConfigType is a type that can be used as a configuration value.
type ConfigType interface{ any }

// Config declares a typed configuration key for the current module.
func Config[T ConfigType](name string) ConfigValue[T] {
	module := callerModule()
	return ConfigValue[T]{module, name}
}

// ConfigValue is a typed configuration key for the current module.
type ConfigValue[T ConfigType] struct {
	module string
	name   string
}

func (c *ConfigValue[T]) GoString() string {
	var t T
	return fmt.Sprintf("ftl.ConfigValue[%T](\"%s.%s\")", t, c.module, c.name)
}

// Get returns the value of the configuration key from FTL.
func (c *ConfigValue[T]) Get(ctx context.Context) (out T) {
	cm := configuration.ConfigFromContext(ctx)
	err := cm.Get(ctx, configuration.NewRef(c.module, c.name), &out)
	if err != nil {
		panic(fmt.Errorf("failed to get %s: %w", c, err))
	}
	return
}

func callerModule() string {
	pc, _, _, ok := runtime.Caller(2)
	if !ok {
		panic("failed to get caller")
	}
	details := runtime.FuncForPC(pc)
	if details == nil {
		panic("failed to get caller")
	}
	module := details.Name()
	if strings.HasPrefix(module, "github.com/TBD54566975/ftl/go-runtime/ftl") {
		return "testing"
	}
	if !strings.HasPrefix(module, "ftl/") {
		panic(fmt.Sprintf("must be called from an FTL module not %s", module))
	}
	return strings.Split(strings.Split(module, "/")[1], ".")[0]
}
