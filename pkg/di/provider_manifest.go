package di

import (
	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/pkg/config"
)

// provideManifest creates a default manifest loader implementation.
// Uses the basic file-based loader that reads YAML manifests from disk.
func provideManifest() manifest.Loader {
	return manifest.NewLoader()
}

// provideManifestGenerator creates a default manifest generator implementation.
// Uses the basic generator that creates manifest structures from options.
func provideManifestGenerator() manifest.Generator {
	return manifest.NewGenerator()
}

// provideManifestGeneratorWithConfig creates a manifest generator with configuration-driven defaults.
// Maps from pkg/config types to manifest generator config types.
func provideManifestGeneratorWithConfig(cfg *config.Config, logger Logger) manifest.Generator {
	if cfg == nil {
		logger.Warn("No configuration provided, using default manifest generator")
		return manifest.NewGenerator()
	}

	manifestConfig := &manifest.GeneratorConfig{
		DefaultWorkspace: cfg.ManifestGenerator.DefaultWorkspace,
		DefaultBranch:    cfg.ManifestGenerator.DefaultBranch,
		Tests: manifest.TestsConfig{
			Command:          cfg.ManifestGenerator.Tests.Command,
			Timeout:          cfg.ManifestGenerator.Tests.Timeout,
			WorkingDirectory: cfg.ManifestGenerator.Tests.WorkingDirectory,
		},
		Notifications: manifest.NotificationsConfig{
			Enabled:   cfg.ManifestGenerator.Notifications.Enabled,
			Channels:  cfg.ManifestGenerator.Notifications.Channels,
			OnSuccess: cfg.ManifestGenerator.Notifications.OnSuccess,
			OnFailure: cfg.ManifestGenerator.Notifications.OnFailure,
		},
		Discovery: manifest.DiscoveryConfig{
			Enabled:         cfg.ManifestGenerator.Discovery.Enabled,
			MaxDepth:        cfg.ManifestGenerator.Discovery.MaxDepth,
			IncludePatterns: cfg.ManifestGenerator.Discovery.IncludePatterns,
			ExcludePatterns: cfg.ManifestGenerator.Discovery.ExcludePatterns,
			Interactive:     cfg.ManifestGenerator.Discovery.Interactive,
		},
	}

	logger.Debug("Created manifest generator with config",
		"default_workspace", manifestConfig.DefaultWorkspace,
		"default_branch", manifestConfig.DefaultBranch,
		"test_command", manifestConfig.Tests.Command,
		"discovery_enabled", manifestConfig.Discovery.Enabled,
	)

	return manifest.NewGeneratorWithConfig(manifestConfig)
}
