package ftl

import (
	"context"
	"testing"

	"github.com/alecthomas/assert/v2"

	"github.com/TBD54566975/ftl/common/configuration"
	"github.com/TBD54566975/ftl/internal/log"
)

func TestConfig(t *testing.T) {
	ctx := log.ContextWithNewDefaultLogger(context.Background())
	cm, err := configuration.NewConfigurationManager(ctx, "testdata/ftl-project.toml")
	assert.NoError(t, err)
	ctx = configuration.ContextWithConfig(ctx, cm)
	type C struct {
		One string
		Two string
	}
	config := Config[C]("test")
	assert.Equal(t, C{"one", "two"}, config.Get(ctx))
}
