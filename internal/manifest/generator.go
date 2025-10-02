package manifest

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Generator exposes manifest generation behaviors.
type Generator interface {
	Generate(ctx context.Context, options GenerateOptions) (*Manifest, error)
}

// GenerateOptions defines the configuration for manifest generation.
type GenerateOptions struct {
	// Module metadata
	ModuleName      string // Human-friendly identifier (e.g., "go-errors")
	ModulePath      string // Go module path (e.g., "github.com/goliatone/go-errors")
	Repository      string // GitHub repository (e.g., "goliatone/go-errors")
	Version         string // Target version (e.g., "v1.2.3")
	ReleaseArtifact string // Optional release artifact path

	// Dependent repositories
	Dependents []DependentOptions

	// Global defaults
	DefaultBranch        string
	DefaultLabels        []string
	DefaultCommitTmpl    string
	DefaultTests         []Command
	DefaultExtraCommands []Command
	DefaultNotifications Notifications
	DefaultPRConfig      PRConfig
}

// DependentOptions defines configuration for a dependent repository.
type DependentOptions struct {
	Repository      string            // GitHub repository (e.g., "goliatone/go-logger")
	CloneURL        string            // Git clone URL (optional, falls back to Repository if empty)
	ModulePath      string            // Go module path within the repo
	LocalModulePath string            // Local path within the repo (defaults to ".")
	Branch          string            // Target branch (inherits from default if empty)
	Tests           []Command         // Custom test commands
	ExtraCommands   []Command         // Additional commands to run
	Labels          []string          // Custom labels
	Notifications   Notifications     // Custom notification settings
	PRConfig        PRConfig          // Custom PR configuration
	Canary          bool              // Whether this is a canary deployment
	Skip            bool              // Whether to skip this dependent
	Env             map[string]string // Environment variables
	Timeout         time.Duration     // Operation timeout
	DiscoverySource string            // Source of discovery (workspace, github, workspace+github)
}

// GeneratorConfig defines configuration options for the manifest generator.
type GeneratorConfig struct {
	// DefaultWorkspace is the default workspace directory for discovering dependent modules.
	DefaultWorkspace string

	// Tests contains default test command configurations.
	Tests TestsConfig

	// Notifications contains default notification settings.
	Notifications NotificationsConfig

	// DefaultBranch is the default branch name to use for dependency updates.
	DefaultBranch string

	// Discovery contains settings for automatic dependent discovery.
	Discovery DiscoveryConfig
}

// TestsConfig contains default test command configurations.
type TestsConfig struct {
	Command          string
	Timeout          time.Duration
	WorkingDirectory string
}

// NotificationsConfig contains default notification settings.
type NotificationsConfig struct {
	Enabled    bool
	Channels   []string
	OnSuccess  bool
	OnFailure  bool
	WebhookURL string
}

// DiscoveryConfig contains settings for automatic dependent discovery.
type DiscoveryConfig struct {
	Enabled         bool
	MaxDepth        int
	IncludePatterns []string
	ExcludePatterns []string
	Interactive     bool
}

// NewGenerator returns a default manifest generator implementation.
func NewGenerator() Generator {
	return &generator{}
}

// NewGeneratorWithConfig returns a manifest generator implementation with configuration.
func NewGeneratorWithConfig(config *GeneratorConfig) Generator {
	if config == nil {
		return &generator{}
	}
	return &generator{config: config}
}

type generator struct {
	config *GeneratorConfig
}

// Generate creates a new manifest based on the provided options.
func (g *generator) Generate(ctx context.Context, options GenerateOptions) (*Manifest, error) {
	if options.ModuleName == "" {
		return nil, fmt.Errorf("module name is required")
	}
	if options.ModulePath == "" {
		return nil, fmt.Errorf("module path is required")
	}
	if options.Repository == "" {
		return nil, fmt.Errorf("repository is required")
	}

	manifest := &Manifest{
		ManifestVersion: 1,
		Defaults:        g.buildDefaults(options),
		Modules: []Module{
			g.buildModule(options),
		},
	}

	return manifest, nil
}

// buildDefaults creates the default configuration section.
func (g *generator) buildDefaults(options GenerateOptions) Defaults {
	// Use config defaults first, then options, then hard-coded defaults
	defaultBranch := g.getConfigBranch(options.DefaultBranch)
	defaultTests := g.getConfigTests(options.DefaultTests)

	defaults := Defaults{
		Branch:         g.getOrDefault(defaultBranch, "main"),
		Labels:         g.getLabelsOrDefault(options.DefaultLabels),
		CommitTemplate: g.getOrDefault(options.DefaultCommitTmpl, "chore(deps): bump {{ module }} to {{ version }}"),
		Tests:          defaultTests,
		ExtraCommands:  options.DefaultExtraCommands,
		Notifications:  g.mergeNotifications(options.DefaultNotifications),
		PR:             g.getPRConfigOrDefault(options.DefaultPRConfig),
	}

	// Ensure non-nil slices
	if defaults.Tests == nil {
		defaults.Tests = []Command{}
	}
	if defaults.ExtraCommands == nil {
		defaults.ExtraCommands = []Command{}
	}
	if defaults.Labels == nil {
		defaults.Labels = []string{}
	}

	return defaults
}

// buildModule creates the module configuration.
func (g *generator) buildModule(options GenerateOptions) Module {
	module := Module{
		Name:            options.ModuleName,
		Module:          options.ModulePath,
		Repo:            options.Repository,
		ReleaseArtifact: options.ReleaseArtifact,
		Dependents:      g.buildDependents(options),
	}

	return module
}

// buildDependents creates the dependent configurations.
func (g *generator) buildDependents(options GenerateOptions) []Dependent {
	dependents := make([]Dependent, len(options.Dependents))

	// Get the resolved default branch (includes config defaults)
	defaultBranch := g.getConfigBranch(options.DefaultBranch)
	resolvedDefaultBranch := g.getOrDefault(defaultBranch, "main")

	for i, dep := range options.Dependents {
		modulePath := dep.LocalModulePath
		if modulePath == "" {
			modulePath = "."
		}

		dependent := Dependent{
			Repo:       dep.Repository,
			Module:     dep.ModulePath,
			ModulePath: modulePath,
			Canary:     dep.Canary,
			Skip:       dep.Skip,
			Timeout:    dep.Timeout,
		}

		if dep.CloneURL != "" {
			dependent.CloneURL = dep.CloneURL
		}

		branch := strings.TrimSpace(dep.Branch)
		if branch != "" && branch != resolvedDefaultBranch {
			dependent.Branch = branch
		}

		if len(dep.Tests) > 0 {
			dependent.Tests = dep.Tests
		}
		if len(dep.ExtraCommands) > 0 {
			dependent.ExtraCommands = dep.ExtraCommands
		}
		if len(dep.Labels) > 0 {
			dependent.Labels = dep.Labels
		}

		if !isNotificationsEmpty(dep.Notifications) {
			dependent.Notifications = dep.Notifications
		}

		if !isPRConfigEmpty(dep.PRConfig) {
			dependent.PR = dep.PRConfig
		}

		if len(dep.Env) > 0 {
			dependent.Env = dep.Env
		}

		dependents[i] = dependent
	}

	return dependents
}

func isNotificationsEmpty(notifications Notifications) bool {
	githubIssuesConfigured := false
	if notifications.GitHubIssues != nil {
		if notifications.GitHubIssues.Enabled {
			githubIssuesConfigured = true
		}
		if len(notifications.GitHubIssues.Labels) > 0 {
			githubIssuesConfigured = true
		}
	}

	return notifications.SlackChannel == "" &&
		!notifications.OnFailure &&
		!notifications.OnSuccess &&
		notifications.Webhook == "" &&
		!githubIssuesConfigured
}

func isPRConfigEmpty(pr PRConfig) bool {
	return pr.TitleTemplate == "" && pr.BodyTemplate == "" && len(pr.Reviewers) == 0 && len(pr.TeamReviewers) == 0
}

// Helper functions for defaults

func (g *generator) getOrDefault(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

func (g *generator) getLabelsOrDefault(labels []string) []string {
	if labels == nil || len(labels) == 0 {
		return []string{"automation:cascade"}
	}
	return labels
}

func (g *generator) getTestsOrDefault(tests []Command) []Command {
	if tests == nil || len(tests) == 0 {
		return []Command{
			{Cmd: []string{"go", "test", "./...", "-race", "-count=1"}},
		}
	}
	return tests
}

func (g *generator) getPRConfigOrDefault(pr PRConfig) PRConfig {
	if pr.TitleTemplate == "" {
		pr.TitleTemplate = "chore(deps): bump {{ module }} to {{ version }}"
	}
	if pr.BodyTemplate == "" {
		pr.BodyTemplate = "Automated dependency update for {{ module }} to {{ version }}"
	}
	return pr
}

// Helper functions that use config defaults

func (g *generator) getConfigBranch(optionsBranch string) string {
	if optionsBranch != "" {
		return optionsBranch
	}
	if g.config != nil && g.config.DefaultBranch != "" {
		return g.config.DefaultBranch
	}
	return ""
}

func (g *generator) getConfigTests(optionsTests []Command) []Command {
	if optionsTests != nil && len(optionsTests) > 0 {
		return optionsTests
	}

	// Use config default test command if available
	if g.config != nil && g.config.Tests.Command != "" {
		cmd := []string{"go", "test", "./..."}
		if g.config.Tests.Command != "go test ./..." {
			// Parse the command string into slice - for now, simple split
			// TODO: Implement proper shell command parsing if needed
			cmd = []string{"sh", "-c", g.config.Tests.Command}
		}

		testCmd := Command{Cmd: cmd}
		if g.config.Tests.WorkingDirectory != "" {
			testCmd.Dir = g.config.Tests.WorkingDirectory
		}

		return []Command{testCmd}
	}

	// Fall back to default
	return g.getTestsOrDefault(nil)
}

func (g *generator) mergeNotifications(optionsNotifications Notifications) Notifications {
	if g.config == nil {
		return optionsNotifications
	}

	result := optionsNotifications

	// Apply config notification defaults if not already set
	if result.SlackChannel == "" && len(g.config.Notifications.Channels) > 0 {
		// Use first channel as slack channel if it looks like a slack channel
		for _, channel := range g.config.Notifications.Channels {
			if strings.HasPrefix(channel, "#") || strings.HasPrefix(channel, "@") {
				result.SlackChannel = channel
				break
			}
		}
	}

	if result.Webhook == "" && g.config.Notifications.WebhookURL != "" {
		result.Webhook = g.config.Notifications.WebhookURL
	}

	return result
}
