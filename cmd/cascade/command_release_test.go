package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/goliatone/cascade/internal/broker"
	execpkg "github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/pkg/config"
	"github.com/goliatone/cascade/pkg/di"
)

// TestReleaseCommand_WithDependencyChecking tests release with skip-up-to-date functionality
func TestReleaseCommand_WithDependencyChecking(t *testing.T) {
	// Create a test workspace with mock repositories
	tempWorkspace := t.TempDir()

	// Create mock repository structure
	repoUpToDate := filepath.Join(tempWorkspace, "repo-up-to-date")
	repoOutdated := filepath.Join(tempWorkspace, "repo-outdated")

	if err := os.MkdirAll(repoUpToDate, 0755); err != nil {
		t.Fatalf("failed to create up-to-date repo: %v", err)
	}
	if err := os.MkdirAll(repoOutdated, 0755); err != nil {
		t.Fatalf("failed to create outdated repo: %v", err)
	}

	// Create go.mod files
	upToDateGoMod := `module github.com/example/repo-up-to-date

go 1.21

require github.com/example/lib v1.2.3
`
	outdatedGoMod := `module github.com/example/repo-outdated

go 1.21

require github.com/example/lib v1.0.0
`

	if err := os.WriteFile(filepath.Join(repoUpToDate, "go.mod"), []byte(upToDateGoMod), 0644); err != nil {
		t.Fatalf("failed to write up-to-date go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoOutdated, "go.mod"), []byte(outdatedGoMod), 0644); err != nil {
		t.Fatalf("failed to write outdated go.mod: %v", err)
	}

	tests := []struct {
		name                string
		skipUpToDate        bool
		forceAll            bool
		expectedWorkItems   int
		expectedDescription string
	}{
		{
			name:                "skip up-to-date enabled - only outdated",
			skipUpToDate:        true,
			forceAll:            false,
			expectedWorkItems:   1,
			expectedDescription: "should skip repo-up-to-date, process repo-outdated",
		},
		{
			name:                "force all - process all",
			skipUpToDate:        false,
			forceAll:            true,
			expectedWorkItems:   2,
			expectedDescription: "should process all repos regardless of version",
		},
		{
			name:                "skip disabled - process all",
			skipUpToDate:        false,
			forceAll:            false,
			expectedWorkItems:   2,
			expectedDescription: "should process all repos when skip is disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test manifest
			manifestPath := filepath.Join(t.TempDir(), "test.yaml")
			manifestContent := `defaults:
  commit_template: "chore: bump {{module}} to {{version}}"

modules:
  - module: github.com/example/lib
    dependents:
      - repo: example/repo-up-to-date
        module: github.com/example/repo-up-to-date
        module_path: .
        branch: main
      - repo: example/repo-outdated
        module: github.com/example/repo-outdated
        module_path: .
        branch: main
`
			if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
				t.Fatalf("failed to write manifest: %v", err)
			}

			// Setup config
			cfg := &config.Config{
				Module:  "github.com/example/lib",
				Version: "v1.2.3",
				Executor: config.ExecutorConfig{
					DryRun:       true,
					SkipUpToDate: tt.skipUpToDate,
					ForceAll:     tt.forceAll,
				},
				Workspace: config.WorkspaceConfig{
					Path: tempWorkspace,
				},
			}

			// Create mock container with real dependency checker
			logger := &mockLogger{}
			loader := manifest.NewLoader()
			m, err := loader.Load(manifestPath)
			if err != nil {
				t.Fatalf("failed to load manifest: %v", err)
			}

			// Create planner with appropriate configuration
			var plannerOpts []planner.Option
			if tt.skipUpToDate && !tt.forceAll {
				checker := planner.NewDependencyChecker(logger)
				plannerOpts = append(plannerOpts,
					planner.WithDependencyChecker(checker),
					planner.WithWorkspace(tempWorkspace))
			}
			p := planner.New(plannerOpts...)

			// Generate plan
			target := planner.Target{Module: cfg.Module, Version: cfg.Version}
			plan, err := p.Plan(context.Background(), m, target)
			if err != nil {
				t.Fatalf("failed to generate plan: %v", err)
			}

			// Verify work items count
			if len(plan.Items) != tt.expectedWorkItems {
				t.Errorf("expected %d work items (%s), got %d",
					tt.expectedWorkItems, tt.expectedDescription, len(plan.Items))
			}

			// If skipping, verify the correct repo was processed
			if tt.skipUpToDate && !tt.forceAll && len(plan.Items) > 0 {
				// Should only have outdated repo
				foundOutdated := false
				for _, item := range plan.Items {
					if item.Repo == "example/repo-outdated" {
						foundOutdated = true
					}
					if item.Repo == "example/repo-up-to-date" {
						t.Errorf("unexpected work item for up-to-date repo: %s", item.Repo)
					}
				}
				if !foundOutdated {
					t.Error("expected work item for outdated repo but didn't find it")
				}
			}
		})
	}
}

// TestReleaseCommand_WorkspaceRequired tests that workspace is required for skip functionality
func TestReleaseCommand_WorkspaceRequired(t *testing.T) {
	// Create test manifest
	manifestPath := filepath.Join(t.TempDir(), "test.yaml")
	manifestContent := `defaults:
  commit_template: "chore: bump {{module}} to {{version}}"

modules:
  - module: github.com/example/lib
    dependents:
      - repo: example/test
        module: github.com/example/test
        module_path: .
        branch: main
`
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	// Setup config WITHOUT workspace
	cfg := &config.Config{
		Module:  "github.com/example/lib",
		Version: "v1.2.3",
		Executor: config.ExecutorConfig{
			DryRun:       true,
			SkipUpToDate: true, // Enabled but no workspace
		},
	}

	logger := &mockLogger{}
	loader := manifest.NewLoader()
	m, err := loader.Load(manifestPath)
	if err != nil {
		t.Fatalf("failed to load manifest: %v", err)
	}

	// Create planner with checker but no workspace
	checker := planner.NewDependencyChecker(logger)
	p := planner.New(
		planner.WithDependencyChecker(checker),
		// No workspace configured
	)

	// Generate plan
	target := planner.Target{Module: cfg.Module, Version: cfg.Version}
	plan, err := p.Plan(context.Background(), m, target)
	if err != nil {
		t.Fatalf("failed to generate plan: %v", err)
	}

	// Should process all items since workspace is not configured (fallback behavior)
	if len(plan.Items) == 0 {
		t.Error("expected work items to be created when workspace is not configured")
	}
}

// TestReleaseCommand_CheckerErrorsFailOpen tests that checker errors don't block planning
func TestReleaseCommand_CheckerErrorsFailOpen(t *testing.T) {
	// Create test manifest
	manifestPath := filepath.Join(t.TempDir(), "test.yaml")
	manifestContent := `defaults:
  commit_template: "chore: bump {{module}} to {{version}}"

modules:
  - module: github.com/example/lib
    dependents:
      - repo: example/test
        module: github.com/example/test
        module_path: .
        branch: main
`
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	// Setup config with workspace that doesn't exist (will cause checker errors)
	cfg := &config.Config{
		Module:  "github.com/example/lib",
		Version: "v1.2.3",
		Executor: config.ExecutorConfig{
			DryRun:       true,
			SkipUpToDate: true,
		},
		Workspace: config.WorkspaceConfig{
			Path: "/nonexistent/workspace",
		},
	}

	logger := &mockLogger{}
	loader := manifest.NewLoader()
	m, err := loader.Load(manifestPath)
	if err != nil {
		t.Fatalf("failed to load manifest: %v", err)
	}

	// Create planner with checker and invalid workspace
	checker := planner.NewDependencyChecker(logger)
	p := planner.New(
		planner.WithDependencyChecker(checker),
		planner.WithWorkspace("/nonexistent/workspace"),
	)

	// Generate plan
	target := planner.Target{Module: cfg.Module, Version: cfg.Version}
	plan, err := p.Plan(context.Background(), m, target)
	if err != nil {
		t.Fatalf("plan should not fail due to checker errors (fail-open): %v", err)
	}

	// Should still create work items despite checker errors (fail-open behavior)
	if len(plan.Items) == 0 {
		t.Error("expected work items to be created even with checker errors (fail-open)")
	}
}

// TestRunReleaseWithDependencyChecking tests the release command logic with dependency checking
func TestRunReleaseWithDependencyChecking(t *testing.T) {
	// Create a test workspace
	tempWorkspace := t.TempDir()

	// Create test manifest
	manifestPath := filepath.Join(t.TempDir(), "test.yaml")
	manifestContent := `defaults:
  commit_template: "chore: bump {{module}} to {{version}}"

modules:
  - module: github.com/example/lib
    dependents:
      - repo: example/test
        module: github.com/example/test
        module_path: .
        branch: main
`
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	tests := []struct {
		name         string
		skipUpToDate bool
		forceAll     bool
		workspace    string
		expectError  bool
	}{
		{
			name:         "with skip-up-to-date enabled",
			skipUpToDate: true,
			forceAll:     false,
			workspace:    tempWorkspace,
			expectError:  false,
		},
		{
			name:         "with force-all enabled",
			skipUpToDate: false,
			forceAll:     true,
			workspace:    tempWorkspace,
			expectError:  false,
		},
		{
			name:         "without workspace",
			skipUpToDate: false, // Can't skip without workspace
			forceAll:     false,
			workspace:    tempWorkspace, // Still need workspace for executor
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Module:  "github.com/example/lib",
				Version: "v1.2.3",
				Executor: config.ExecutorConfig{
					DryRun:       true,
					SkipUpToDate: tt.skipUpToDate,
					ForceAll:     tt.forceAll,
				},
				Workspace: config.WorkspaceConfig{
					Path: tt.workspace,
				},
			}

			// Create mock container
			logger := &mockLogger{}
			mockLoader := &mockManifestLoader{
				loadFunc: func(path string) (*manifest.Manifest, error) {
					loader := manifest.NewLoader()
					return loader.Load(path)
				},
			}
			mockExecutor := &mockExecutor{
				applyFunc: func(ctx context.Context, input execpkg.WorkItemContext) (*execpkg.Result, error) {
					return &execpkg.Result{Status: execpkg.StatusCompleted, Reason: "test"}, nil
				},
			}
			mockBroker := &mockBroker{
				ensurePRFunc: func(ctx context.Context, item planner.WorkItem, result *execpkg.Result) (*broker.PullRequest, error) {
					return &broker.PullRequest{Repo: item.Repo, URL: "https://example.com/pr/1"}, nil
				},
			}
			mockState := &mockStateManager{}

			var plannerOpts []planner.Option
			if tt.skipUpToDate && !tt.forceAll && tt.workspace != "" {
				checker := planner.NewDependencyChecker(logger)
				plannerOpts = append(plannerOpts,
					planner.WithDependencyChecker(checker),
					planner.WithWorkspace(tt.workspace))
			}
			mockPlanner := planner.New(plannerOpts...)

			mockContainer, err := di.New(
				di.WithConfig(cfg),
				di.WithLogger(logger),
				di.WithManifestLoader(mockLoader),
				di.WithPlanner(mockPlanner),
				di.WithExecutor(mockExecutor),
				di.WithBroker(mockBroker),
				di.WithStateManager(mockState),
			)
			if err != nil {
				t.Fatalf("failed to create mock container: %v", err)
			}

			// Set the global container for the test
			originalContainer := container
			container = mockContainer
			defer func() { container = originalContainer }()

			// Call the function under test
			err = runRelease("", manifestPath, "", "")

			// Check results
			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			} else if !tt.expectError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}
