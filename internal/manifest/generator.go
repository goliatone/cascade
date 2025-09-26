package manifest

import (
	"context"
	"fmt"
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
}

// NewGenerator returns a default manifest generator implementation.
func NewGenerator() Generator {
	return &generator{}
}

type generator struct{}

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
	defaults := Defaults{
		Branch:         g.getOrDefault(options.DefaultBranch, "main"),
		Labels:         g.getLabelsOrDefault(options.DefaultLabels),
		CommitTemplate: g.getOrDefault(options.DefaultCommitTmpl, "chore(deps): bump {{ module }} to {{ version }}"),
		Tests:          g.getTestsOrDefault(options.DefaultTests),
		ExtraCommands:  options.DefaultExtraCommands,
		Notifications:  options.DefaultNotifications,
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

	for i, dep := range options.Dependents {
		dependent := Dependent{
			Repo:          dep.Repository,
			Module:        dep.ModulePath,
			ModulePath:    g.getOrDefault(dep.LocalModulePath, "."),
			Branch:        g.getOrDefault(dep.Branch, options.DefaultBranch),
			Tests:         dep.Tests,
			ExtraCommands: dep.ExtraCommands,
			Labels:        dep.Labels,
			Notifications: dep.Notifications,
			PR:            dep.PRConfig,
			Canary:        dep.Canary,
			Skip:          dep.Skip,
			Env:           dep.Env,
			Timeout:       dep.Timeout,
		}

		// Ensure non-nil slices
		if dependent.Tests == nil {
			dependent.Tests = []Command{}
		}
		if dependent.ExtraCommands == nil {
			dependent.ExtraCommands = []Command{}
		}
		if dependent.Labels == nil {
			dependent.Labels = []string{}
		}
		if dependent.Env == nil {
			dependent.Env = map[string]string{}
		}

		dependents[i] = dependent
	}

	return dependents
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
