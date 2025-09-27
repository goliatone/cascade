package main

import (
	"strings"

	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/pkg/config"
)

// Config defaults merging functions for manifest generation

// getDefaultBranch returns the configured default branch or fallback
func getDefaultBranch(cfg *config.Config) string {
	if cfg != nil && cfg.ManifestGenerator.DefaultBranch != "" {
		return cfg.ManifestGenerator.DefaultBranch
	}
	return "main"
}

// getDefaultTests returns the configured default test commands or fallback
func getDefaultTests(cfg *config.Config) []manifest.Command {
	if cfg != nil && cfg.ManifestGenerator.Tests.Command != "" {
		testCmd := manifest.Command{}

		// Parse command - respect the exact command from config
		parts := strings.Fields(cfg.ManifestGenerator.Tests.Command)
		if len(parts) > 0 {
			testCmd.Cmd = parts
		} else {
			// Fallback to shell execution for complex commands
			testCmd.Cmd = []string{"sh", "-c", cfg.ManifestGenerator.Tests.Command}
		}

		if cfg.ManifestGenerator.Tests.WorkingDirectory != "" {
			testCmd.Dir = cfg.ManifestGenerator.Tests.WorkingDirectory
		}

		return []manifest.Command{testCmd}
	}

	// Default fallback
	return []manifest.Command{
		{Cmd: []string{"go", "test", "./...", "-race", "-count=1"}},
	}
}

// getDefaultSlackChannel returns the CLI-provided channel or config default
func getDefaultSlackChannel(cliChannel string, cfg *config.Config) string {
	if cliChannel != "" {
		return cliChannel
	}
	if cfg != nil && len(cfg.ManifestGenerator.Notifications.Channels) > 0 {
		// Use first configured channel as Slack channel
		return cfg.ManifestGenerator.Notifications.Channels[0]
	}
	if cfg != nil && cfg.Integration.Slack.Channel != "" {
		return cfg.Integration.Slack.Channel
	}
	return ""
}

// getDefaultWebhook returns the CLI-provided webhook or config default
func getDefaultWebhook(cliWebhook string, cfg *config.Config) string {
	if cliWebhook != "" {
		return cliWebhook
	}
	if cfg != nil && cfg.Integration.Slack.WebhookURL != "" {
		return cfg.Integration.Slack.WebhookURL
	}
	return ""
}

// getDiscoveryMaxDepth returns the CLI-provided maxDepth or config default
func getDiscoveryMaxDepth(cliMaxDepth int, cfg *config.Config) int {
	if cliMaxDepth > 0 {
		return cliMaxDepth
	}
	if cfg != nil && cfg.ManifestGenerator.Discovery.MaxDepth > 0 {
		return cfg.ManifestGenerator.Discovery.MaxDepth
	}
	return 0 // 0 means unlimited depth
}

// getDiscoveryIncludePatterns returns the CLI-provided patterns or config defaults
func getDiscoveryIncludePatterns(cliPatterns []string, cfg *config.Config) []string {
	if len(cliPatterns) > 0 {
		return cliPatterns
	}
	if cfg != nil && len(cfg.ManifestGenerator.Discovery.IncludePatterns) > 0 {
		return cfg.ManifestGenerator.Discovery.IncludePatterns
	}
	return []string{} // Empty means include all
}

// getDiscoveryExcludePatterns returns the CLI-provided patterns or config defaults
func getDiscoveryExcludePatterns(cliPatterns []string, cfg *config.Config) []string {
	if len(cliPatterns) > 0 {
		return cliPatterns
	}
	if cfg != nil && len(cfg.ManifestGenerator.Discovery.ExcludePatterns) > 0 {
		return cfg.ManifestGenerator.Discovery.ExcludePatterns
	}
	return []string{"vendor/*", ".git/*", "node_modules/*"} // Sensible defaults
}

// getGitHubIncludePatterns returns the include patterns for GitHub discovery
func getGitHubIncludePatterns(cliPatterns []string, cfg *config.Config) []string {
	if len(cliPatterns) > 0 {
		return cliPatterns
	}
	if cfg != nil && len(cfg.ManifestGenerator.Discovery.GitHub.IncludePatterns) > 0 {
		return cfg.ManifestGenerator.Discovery.GitHub.IncludePatterns
	}
	return nil
}

// getGitHubExcludePatterns returns the exclude patterns for GitHub discovery
func getGitHubExcludePatterns(cliPatterns []string, cfg *config.Config) []string {
	if len(cliPatterns) > 0 {
		return cliPatterns
	}
	if cfg != nil && len(cfg.ManifestGenerator.Discovery.GitHub.ExcludePatterns) > 0 {
		return cfg.ManifestGenerator.Discovery.GitHub.ExcludePatterns
	}
	return nil
}
