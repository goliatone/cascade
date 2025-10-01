package di

import "github.com/goliatone/cascade/pkg/config"

// provideConfig creates a default configuration.
// Loads configuration from environment variables and defaults.
func provideConfig() *config.Config {
	cfg := config.New()
	// Configuration loading is handled by pkg/config
	return cfg
}

// provideConfigWithDefaults creates a configuration with defaults applied.
// Configuration precedence is enforced upstream by pkg/config: flags override
// environment variables, which override file values, which override defaults.
// The DI layer relies on that behavior when wiring services so the order stays
// consistent across commands.
func provideConfigWithDefaults() (*config.Config, error) {
	return config.NewWithDefaults()
}
