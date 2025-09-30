package config

import "strings"

// CommandSpec describes a command invocation for manifest defaults.
type CommandSpec struct {
	Cmd []string
	Dir string
}

// ManifestDefaultBranch returns the configured default branch or "main".
func ManifestDefaultBranch(cfg *Config) string {
	if cfg != nil && strings.TrimSpace(cfg.ManifestGenerator.DefaultBranch) != "" {
		return strings.TrimSpace(cfg.ManifestGenerator.DefaultBranch)
	}
	return "main"
}

// ManifestDefaultTests returns the configured default test commands or the built-in fallback.
func ManifestDefaultTests(cfg *Config) []CommandSpec {
	if cfg != nil && strings.TrimSpace(cfg.ManifestGenerator.Tests.Command) != "" {
		cmdText := strings.TrimSpace(cfg.ManifestGenerator.Tests.Command)
		spec := CommandSpec{}

		if parts := strings.Fields(cmdText); len(parts) > 0 {
			spec.Cmd = parts
		} else {
			spec.Cmd = []string{"sh", "-c", cmdText}
		}

		spec.Dir = strings.TrimSpace(cfg.ManifestGenerator.Tests.WorkingDirectory)
		return []CommandSpec{spec}
	}

	return []CommandSpec{{Cmd: []string{"go", "test", "./...", "-race", "-count=1"}}}
}

// ManifestDefaultSlackChannel returns the configured Slack channel if available.
func ManifestDefaultSlackChannel(cfg *Config) string {
	if cfg == nil {
		return ""
	}

	if len(cfg.ManifestGenerator.Notifications.Channels) > 0 {
		return cfg.ManifestGenerator.Notifications.Channels[0]
	}

	return strings.TrimSpace(cfg.Integration.Slack.Channel)
}

// ManifestDefaultWebhook returns the configured webhook URL.
func ManifestDefaultWebhook(cfg *Config) string {
	if cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.Integration.Slack.WebhookURL)
}

// DiscoveryMaxDepth returns the configured discovery depth (0 for unlimited).
func DiscoveryMaxDepth(cfg *Config) int {
	if cfg != nil && cfg.ManifestGenerator.Discovery.MaxDepth > 0 {
		return cfg.ManifestGenerator.Discovery.MaxDepth
	}
	return 0
}

// DiscoveryIncludePatterns returns include patterns configured for workspace discovery.
func DiscoveryIncludePatterns(cfg *Config) []string {
	if cfg != nil && len(cfg.ManifestGenerator.Discovery.IncludePatterns) > 0 {
		return cfg.ManifestGenerator.Discovery.IncludePatterns
	}
	return nil
}

// DiscoveryExcludePatterns returns exclude patterns configured for workspace discovery or sensible defaults.
func DiscoveryExcludePatterns(cfg *Config) []string {
	if cfg != nil && len(cfg.ManifestGenerator.Discovery.ExcludePatterns) > 0 {
		return cfg.ManifestGenerator.Discovery.ExcludePatterns
	}
	return []string{"vendor/*", ".git/*", "node_modules/*"}
}

// GitHubDiscoveryIncludePatterns returns include patterns for GitHub discovery.
func GitHubDiscoveryIncludePatterns(cfg *Config) []string {
	if cfg != nil && len(cfg.ManifestGenerator.Discovery.GitHub.IncludePatterns) > 0 {
		return cfg.ManifestGenerator.Discovery.GitHub.IncludePatterns
	}
	return nil
}

// GitHubDiscoveryExcludePatterns returns exclude patterns for GitHub discovery.
func GitHubDiscoveryExcludePatterns(cfg *Config) []string {
	if cfg != nil && len(cfg.ManifestGenerator.Discovery.GitHub.ExcludePatterns) > 0 {
		return cfg.ManifestGenerator.Discovery.GitHub.ExcludePatterns
	}
	return nil
}
