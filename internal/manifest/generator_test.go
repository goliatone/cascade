package manifest_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/pkg/testsupport"
	"github.com/google/go-cmp/cmp"
)

func TestGenerator_Generate_MinimalConfiguration(t *testing.T) {
	generator := manifest.NewGenerator()
	ctx := context.Background()

	options := manifest.GenerateOptions{
		ModuleName: "go-errors",
		ModulePath: "github.com/goliatone/go-errors",
		Repository: "goliatone/go-errors",
	}

	got, err := generator.Generate(ctx, options)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	var want manifest.Manifest
	if err := testsupport.LoadGolden(filepath.Join("testdata", "generator_minimal.json"), &want); err != nil {
		// If golden file doesn't exist, write it for the first time
		if err := testsupport.WriteGolden(filepath.Join("testdata", "generator_minimal.json"), got); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}
		t.Skip("Golden file created, re-run test to validate")
	}

	if diff := cmp.Diff(&want, got); diff != "" {
		t.Fatalf("manifest mismatch (-want +got):\n%s", diff)
	}
}

func TestGenerator_Generate_FullConfiguration(t *testing.T) {
	generator := manifest.NewGenerator()
	ctx := context.Background()

	options := manifest.GenerateOptions{
		ModuleName:      "go-logger",
		ModulePath:      "github.com/goliatone/go-logger",
		Repository:      "goliatone/go-logger",
		Version:         "v1.2.3",
		ReleaseArtifact: "release.json",
		Dependents: []manifest.DependentOptions{
			{
				Repository:      "goliatone/go-router",
				ModulePath:      "github.com/goliatone/go-router",
				LocalModulePath: ".",
				Branch:          "develop",
				Tests: []manifest.Command{
					{Cmd: []string{"task", "test"}, Dir: "."},
				},
				ExtraCommands: []manifest.Command{
					{Cmd: []string{"task", "lint"}, Dir: "."},
				},
				Labels: []string{"package:go-router", "priority:high"},
				Notifications: manifest.Notifications{
					SlackChannel: "#go-router-releases",
				},
				PRConfig: manifest.PRConfig{
					TitleTemplate: "chore: update router deps",
					BodyTemplate:  "router.pr.tpl.md",
					Reviewers:     []string{"reviewer1"},
					TeamReviewers: []string{"team-go"},
				},
				Canary: true,
				Env: map[string]string{
					"ENV": "test",
				},
				Timeout: 30 * time.Minute,
			},
			{
				Repository:      "goliatone/go-auth",
				ModulePath:      "github.com/goliatone/go-auth",
				LocalModulePath: ".",
				Branch:          "main",
				Tests: []manifest.Command{
					{Cmd: []string{"task", "test:integration"}, Dir: "."},
				},
				ExtraCommands: []manifest.Command{
					{Cmd: []string{"go", "run", "./cmd/db/migrate", "--dry-run"}, Dir: "."},
				},
				Labels: []string{"package:go-auth"},
				Notifications: manifest.Notifications{
					SlackChannel: "#go-auth-releases",
				},
				Skip: false,
			},
		},
		DefaultBranch:     "main",
		DefaultLabels:     []string{"automation:cascade", "type:dependency"},
		DefaultCommitTmpl: "chore(deps): bump {{ module }} to {{ version }}",
		DefaultTests: []manifest.Command{
			{Cmd: []string{"go", "test", "./...", "-race", "-count=1"}, Dir: "."},
		},
		DefaultExtraCommands: []manifest.Command{
			{Cmd: []string{"go", "mod", "tidy"}, Dir: "."},
		},
		DefaultNotifications: manifest.Notifications{
			SlackChannel: "#general-releases",
			Webhook:      "https://hooks.slack.com/test",
		},
		DefaultPRConfig: manifest.PRConfig{
			TitleTemplate: "chore(deps): update {{ module }} to {{ version }}",
			BodyTemplate:  "Automated dependency update",
		},
	}

	got, err := generator.Generate(ctx, options)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	var want manifest.Manifest
	if err := testsupport.LoadGolden(filepath.Join("testdata", "generator_full.json"), &want); err != nil {
		// If golden file doesn't exist, write it for the first time
		if err := testsupport.WriteGolden(filepath.Join("testdata", "generator_full.json"), got); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}
		t.Skip("Golden file created, re-run test to validate")
	}

	if diff := cmp.Diff(&want, got); diff != "" {
		t.Fatalf("manifest mismatch (-want +got):\n%s", diff)
	}
}

func TestGenerator_Generate_MediumConfiguration(t *testing.T) {
	generator := manifest.NewGenerator()
	ctx := context.Background()

	options := manifest.GenerateOptions{
		ModuleName: "go-repository-bun",
		ModulePath: "github.com/goliatone/go-repository-bun",
		Repository: "goliatone/go-repository-bun",
		Dependents: []manifest.DependentOptions{
			{
				Repository: "goliatone/go-auth",
				ModulePath: "github.com/goliatone/go-auth",
				Tests: []manifest.Command{
					{Cmd: []string{"task", "test:integration"}, Dir: "."},
				},
				Labels: []string{"package:go-auth"},
			},
		},
		DefaultBranch: "main",
		DefaultLabels: []string{"automation:cascade"},
	}

	got, err := generator.Generate(ctx, options)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	var want manifest.Manifest
	if err := testsupport.LoadGolden(filepath.Join("testdata", "generator_medium.json"), &want); err != nil {
		// If golden file doesn't exist, write it for the first time
		if err := testsupport.WriteGolden(filepath.Join("testdata", "generator_medium.json"), got); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}
		t.Skip("Golden file created, re-run test to validate")
	}

	if diff := cmp.Diff(&want, got); diff != "" {
		t.Fatalf("manifest mismatch (-want +got):\n%s", diff)
	}
}

func TestGenerator_Generate_ValidationErrors(t *testing.T) {
	generator := manifest.NewGenerator()
	ctx := context.Background()

	tests := []struct {
		name    string
		options manifest.GenerateOptions
		wantErr string
	}{
		{
			name:    "missing module name",
			options: manifest.GenerateOptions{},
			wantErr: "module name is required",
		},
		{
			name: "missing module path",
			options: manifest.GenerateOptions{
				ModuleName: "go-errors",
			},
			wantErr: "module path is required",
		},
		{
			name: "missing repository",
			options: manifest.GenerateOptions{
				ModuleName: "go-errors",
				ModulePath: "github.com/goliatone/go-errors",
			},
			wantErr: "repository is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := generator.Generate(ctx, tt.options)
			if err == nil {
				t.Fatal("expected error but got none")
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("expected error %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestGenerator_Generate_DefaultsApplication(t *testing.T) {
	generator := manifest.NewGenerator()
	ctx := context.Background()

	// Test that defaults are properly applied when not specified
	options := manifest.GenerateOptions{
		ModuleName: "go-errors",
		ModulePath: "github.com/goliatone/go-errors",
		Repository: "goliatone/go-errors",
		Dependents: []manifest.DependentOptions{
			{
				Repository: "goliatone/go-logger",
				ModulePath: "github.com/goliatone/go-logger",
				// No branch specified - should inherit default
			},
		},
		DefaultBranch: "develop", // Custom default
	}

	got, err := generator.Generate(ctx, options)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// Verify defaults are applied correctly
	if got.Defaults.Branch != "develop" {
		t.Errorf("expected default branch 'develop', got %q", got.Defaults.Branch)
	}

	if len(got.Modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(got.Modules))
	}

	module := got.Modules[0]
	if len(module.Dependents) != 1 {
		t.Fatalf("expected 1 dependent, got %d", len(module.Dependents))
	}

	dependent := module.Dependents[0]
	if dependent.Branch != "develop" {
		t.Errorf("expected dependent branch to inherit default 'develop', got %q", dependent.Branch)
	}

	if dependent.ModulePath != "." {
		t.Errorf("expected default module path '.', got %q", dependent.ModulePath)
	}

	// Verify default labels are applied
	if len(got.Defaults.Labels) != 1 || got.Defaults.Labels[0] != "automation:cascade" {
		t.Errorf("expected default labels ['automation:cascade'], got %v", got.Defaults.Labels)
	}

	// Verify default test command is applied
	if len(got.Defaults.Tests) != 1 {
		t.Fatalf("expected 1 default test, got %d", len(got.Defaults.Tests))
	}

	expectedTest := manifest.Command{Cmd: []string{"go", "test", "./...", "-race", "-count=1"}}
	if diff := cmp.Diff(expectedTest, got.Defaults.Tests[0]); diff != "" {
		t.Errorf("default test command mismatch (-want +got):\n%s", diff)
	}
}

func TestGenerator_Generate_ConfigDrivenDefaults(t *testing.T) {
	// Create generator with config defaults
	config := &manifest.GeneratorConfig{
		DefaultBranch: "cascade/update-deps",
		Tests: manifest.TestsConfig{
			Command:          "go test -race ./...",
			Timeout:          10 * time.Minute,
			WorkingDirectory: ".",
		},
		Notifications: manifest.NotificationsConfig{
			Enabled:   true,
			Channels:  []string{"#engineering", "#updates"},
			OnSuccess: false,
			OnFailure: true,
		},
		Discovery: manifest.DiscoveryConfig{
			Enabled:         true,
			MaxDepth:        3,
			IncludePatterns: []string{"*.go", "go.mod"},
			ExcludePatterns: []string{"vendor/*", ".git/*"},
			Interactive:     true,
		},
	}

	generator := manifest.NewGeneratorWithConfig(config)
	ctx := context.Background()

	// Test with minimal options - should use config defaults
	options := manifest.GenerateOptions{
		ModuleName: "go-errors",
		ModulePath: "github.com/goliatone/go-errors",
		Repository: "goliatone/go-errors",
		Dependents: []manifest.DependentOptions{
			{
				Repository: "goliatone/go-logger",
				ModulePath: "github.com/goliatone/go-logger",
				// No branch or tests specified - should use config defaults
			},
		},
		// No default branch specified - should use config default
	}

	got, err := generator.Generate(ctx, options)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// Verify config defaults are applied
	if got.Defaults.Branch != "cascade/update-deps" {
		t.Errorf("expected config default branch 'cascade/update-deps', got %q", got.Defaults.Branch)
	}

	// Verify config test command is applied
	if len(got.Defaults.Tests) != 1 {
		t.Fatalf("expected 1 default test from config, got %d", len(got.Defaults.Tests))
	}

	expectedTest := manifest.Command{
		Cmd: []string{"sh", "-c", "go test -race ./..."},
		Dir: ".",
	}
	if diff := cmp.Diff(expectedTest, got.Defaults.Tests[0]); diff != "" {
		t.Errorf("config test command mismatch (-want +got):\n%s", diff)
	}

	// Verify dependent inherits config defaults
	if len(got.Modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(got.Modules))
	}

	module := got.Modules[0]
	if len(module.Dependents) != 1 {
		t.Fatalf("expected 1 dependent, got %d", len(module.Dependents))
	}

	dependent := module.Dependents[0]
	if dependent.Branch != "cascade/update-deps" {
		t.Errorf("expected dependent to inherit config default branch 'cascade/update-deps', got %q", dependent.Branch)
	}
}

func TestGenerator_Generate_ConfigDefaultsOverridePrecedence(t *testing.T) {
	// Create generator with config defaults
	config := &manifest.GeneratorConfig{
		DefaultBranch: "cascade/config-default",
		Tests: manifest.TestsConfig{
			Command: "go test -short ./...",
		},
	}

	generator := manifest.NewGeneratorWithConfig(config)
	ctx := context.Background()

	// Test options override config defaults
	options := manifest.GenerateOptions{
		ModuleName:    "go-errors",
		ModulePath:    "github.com/goliatone/go-errors",
		Repository:    "goliatone/go-errors",
		DefaultBranch: "options-override", // This should override config default
		DefaultTests: []manifest.Command{
			{Cmd: []string{"go", "test", "./...", "-v"}}, // This should override config test
		},
		Dependents: []manifest.DependentOptions{
			{
				Repository: "goliatone/go-logger",
				ModulePath: "github.com/goliatone/go-logger",
			},
		},
	}

	got, err := generator.Generate(ctx, options)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// Verify options override config defaults
	if got.Defaults.Branch != "options-override" {
		t.Errorf("expected options branch 'options-override' to override config, got %q", got.Defaults.Branch)
	}

	// Verify options test command overrides config
	if len(got.Defaults.Tests) != 1 {
		t.Fatalf("expected 1 test from options, got %d", len(got.Defaults.Tests))
	}

	expectedTest := manifest.Command{Cmd: []string{"go", "test", "./...", "-v"}}
	if diff := cmp.Diff(expectedTest, got.Defaults.Tests[0]); diff != "" {
		t.Errorf("options test command should override config (-want +got):\n%s", diff)
	}

	// Verify dependent uses overridden defaults
	dependent := got.Modules[0].Dependents[0]
	if dependent.Branch != "options-override" {
		t.Errorf("expected dependent to use overridden branch 'options-override', got %q", dependent.Branch)
	}
}

func TestGenerator_Generate_ConfigDrivenMinimalFlags(t *testing.T) {
	// Create comprehensive config that provides all needed defaults
	config := &manifest.GeneratorConfig{
		DefaultWorkspace: "/workspace",
		DefaultBranch:    "feature/automated-updates",
		Tests: manifest.TestsConfig{
			Command:          "task test",
			Timeout:          15 * time.Minute,
			WorkingDirectory: ".",
		},
		Notifications: manifest.NotificationsConfig{
			Enabled:   true,
			Channels:  []string{"#alerts"},
			OnSuccess: true,
			OnFailure: true,
		},
		Discovery: manifest.DiscoveryConfig{
			Enabled:         true,
			MaxDepth:        5,
			IncludePatterns: []string{"**/*.go"},
			ExcludePatterns: []string{"vendor/**", "testdata/**"},
			Interactive:     false,
		},
	}

	generator := manifest.NewGeneratorWithConfig(config)
	ctx := context.Background()

	// Minimal options - only module info provided, everything else from config
	options := manifest.GenerateOptions{
		ModuleName: "go-core",
		ModulePath: "github.com/company/go-core",
		Repository: "company/go-core",
		// No defaults specified - should all come from config
	}

	got, err := generator.Generate(ctx, options)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// Verify all defaults come from config
	if got.Defaults.Branch != "feature/automated-updates" {
		t.Errorf("expected config branch 'feature/automated-updates', got %q", got.Defaults.Branch)
	}

	// Verify config test is used
	if len(got.Defaults.Tests) != 1 {
		t.Fatalf("expected 1 test from config, got %d", len(got.Defaults.Tests))
	}

	expectedTest := manifest.Command{
		Cmd: []string{"sh", "-c", "task test"},
		Dir: ".",
	}
	if diff := cmp.Diff(expectedTest, got.Defaults.Tests[0]); diff != "" {
		t.Errorf("config test command mismatch (-want +got):\n%s", diff)
	}

	// Test that manifest is usable for cascade operations
	if got.ManifestVersion != 1 {
		t.Errorf("expected manifest version 1, got %d", got.ManifestVersion)
	}

	if len(got.Modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(got.Modules))
	}

	module := got.Modules[0]
	if module.Name != "go-core" {
		t.Errorf("expected module name 'go-core', got %q", module.Name)
	}

	if module.Module != "github.com/company/go-core" {
		t.Errorf("expected module path 'github.com/company/go-core', got %q", module.Module)
	}

	if module.Repo != "company/go-core" {
		t.Errorf("expected repository 'company/go-core', got %q", module.Repo)
	}
}
