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
