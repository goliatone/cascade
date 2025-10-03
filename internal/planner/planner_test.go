package planner_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/pkg/testsupport"
)

func TestPlanner_PlanProducesExpectedWorkItems(t *testing.T) {

	loader := manifest.NewLoader()
	m, err := loader.Load(filepath.Join("..", "manifest", "testdata", "basic.yaml"))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	target := planner.Target{Module: "github.com/goliatone/go-errors", Version: "v1.2.3"}

	p := planner.New()
	plan, err := p.Plan(context.Background(), m, target)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	var want planner.Plan
	if err := testsupport.LoadGolden(filepath.Join("testdata", "basic_plan.json"), &want); err != nil {
		t.Fatalf("load golden: %v", err)
	}

	if !reflect.DeepEqual(plan, &want) {
		gotJSON, _ := json.MarshalIndent(plan, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Fatalf("plan mismatch\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}

func TestPlanner_InvalidTarget_EmptyModule(t *testing.T) {
	loader := manifest.NewLoader()
	m, err := loader.Load(filepath.Join("..", "manifest", "testdata", "basic.yaml"))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	target := planner.Target{Module: "", Version: "v1.2.3"}

	p := planner.New()
	_, err = p.Plan(context.Background(), m, target)

	if err == nil {
		t.Fatal("expected error for empty module, got nil")
	}

	if !planner.IsInvalidTarget(err) {
		t.Fatalf("expected InvalidTargetError, got %T: %v", err, err)
	}

	expectedMsg := "planner: invalid target: module field is empty"
	if err.Error() != expectedMsg {
		t.Fatalf("expected error message %q, got %q", expectedMsg, err.Error())
	}
}

func TestPlanner_InvalidTarget_EmptyVersion(t *testing.T) {
	loader := manifest.NewLoader()
	m, err := loader.Load(filepath.Join("..", "manifest", "testdata", "basic.yaml"))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	target := planner.Target{Module: "github.com/goliatone/go-errors", Version: ""}

	p := planner.New()
	_, err = p.Plan(context.Background(), m, target)

	if err == nil {
		t.Fatal("expected error for empty version, got nil")
	}

	if !planner.IsInvalidTarget(err) {
		t.Fatalf("expected InvalidTargetError, got %T: %v", err, err)
	}

	expectedMsg := "planner: invalid target: version field is empty"
	if err.Error() != expectedMsg {
		t.Fatalf("expected error message %q, got %q", expectedMsg, err.Error())
	}
}

func TestPlanner_TargetNotFound(t *testing.T) {
	loader := manifest.NewLoader()
	m, err := loader.Load(filepath.Join("..", "manifest", "testdata", "basic.yaml"))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	target := planner.Target{Module: "github.com/nonexistent/module", Version: "v1.0.0"}

	p := planner.New()
	_, err = p.Plan(context.Background(), m, target)

	if err == nil {
		t.Fatal("expected error for nonexistent module, got nil")
	}

	if !planner.IsTargetNotFound(err) {
		t.Fatalf("expected TargetNotFoundError, got %T: %v", err, err)
	}

	expectedMsg := "planner: target module not found: github.com/nonexistent/module"
	if err.Error() != expectedMsg {
		t.Fatalf("expected error message %q, got %q", expectedMsg, err.Error())
	}
}

func TestPlanner_ValidationErrors(t *testing.T) {
	tests := []struct {
		name         string
		manifestYAML string
		target       planner.Target
		expectError  string
	}{
		{
			name: "empty_repo",
			manifestYAML: `
defaults:
  commit_template: "chore: bump {{module}} to {{version}}"

modules:
  - module: github.com/example/test
    dependents:
      - repo: ""
        module: github.com/example/dependent
        module_path: .
        branch: main
`,
			target:      planner.Target{Module: "github.com/example/test", Version: "v1.0.0"},
			expectError: "work item repo is empty",
		},
		{
			name: "empty_module",
			manifestYAML: `
defaults:
  commit_template: "chore: bump {{module}} to {{version}}"

modules:
  - module: github.com/example/test
    dependents:
      - repo: example/dependent
        module: ""
        module_path: .
        branch: main
`,
			target:      planner.Target{Module: "github.com/example/test", Version: "v1.0.0"},
			expectError: "work item module is empty",
		},
		{
			name: "empty_branch",
			manifestYAML: `
defaults:
  commit_template: "chore: bump {{module}} to {{version}}"

modules:
  - module: github.com/example/test
    dependents:
      - repo: example/dependent
        module: github.com/example/dependent
        module_path: .
        branch: ""
`,
			target:      planner.Target{Module: "github.com/example/test", Version: "v1.0.0"},
			expectError: "work item branch is empty",
		},
		{
			name: "negative_timeout",
			manifestYAML: `
defaults:
  commit_template: "chore: bump {{module}} to {{version}}"

modules:
  - module: github.com/example/test
    dependents:
      - repo: example/dependent
        module: github.com/example/dependent
        module_path: .
        branch: main
        timeout: -5s
`,
			target:      planner.Target{Module: "github.com/example/test", Version: "v1.0.0"},
			expectError: "work item timeout cannot be negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary manifest file
			tmpFile, err := os.CreateTemp("", "manifest_*.yaml")
			if err != nil {
				t.Fatalf("create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())

			if _, err := tmpFile.WriteString(tt.manifestYAML); err != nil {
				t.Fatalf("write temp file: %v", err)
			}
			if err := tmpFile.Close(); err != nil {
				t.Fatalf("close temp file: %v", err)
			}

			loader := manifest.NewLoader()
			m, err := loader.Load(tmpFile.Name())
			if err != nil {
				t.Fatalf("load manifest: %v", err)
			}

			p := planner.New()
			_, err = p.Plan(context.Background(), m, tt.target)

			if err == nil {
				t.Fatal("expected validation error, got nil")
			}

			if !planner.IsPlanningError(err) {
				t.Fatalf("expected PlanningError, got %T: %v", err, err)
			}

			if !contains(err.Error(), tt.expectError) {
				t.Fatalf("expected error containing %q, got %q", tt.expectError, err.Error())
			}
		})
	}
}

func TestPlanner_AppliesDependentOverrides(t *testing.T) {
	workspace := t.TempDir()
	dependentDir := filepath.Join(workspace, "go-logger")
	if err := os.MkdirAll(dependentDir, 0o755); err != nil {
		t.Fatalf("mkdir dependent: %v", err)
	}

	manifestContents := `manifest_version: 1
module:
  module: github.com/goliatone/go-logger
  branch: module-default
  tests:
    - cmd: ["task", "module:test"]
      dir: ""
  extra_commands:
    - cmd: ["task", "module:lint"]
      dir: ""
  labels: ["module:label"]
  notifications:
    slack_channel: "#module"
  pr:
    title: "module pr"
  env:
    MODULE_ENV: "module"
  timeout: 2m
dependents:
  github.com/goliatone/go-errors:
    branch: override-branch
    tests:
      - cmd: ["task", "override:test"]
        dir: ""
    env:
      OVERRIDE_ENV: "override"
    timeout: 1m
`

	if err := os.WriteFile(filepath.Join(dependentDir, ".cascade.yaml"), []byte(manifestContents), 0o644); err != nil {
		t.Fatalf("write dependent manifest: %v", err)
	}

	loader := manifest.NewLoader()
	m, err := loader.Load(filepath.Join("..", "manifest", "testdata", "basic.yaml"))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	target := planner.Target{Module: "github.com/goliatone/go-errors", Version: "v1.2.3"}

	p := planner.New(planner.WithWorkspace(workspace))
	plan, err := p.Plan(context.Background(), m, target)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	var loggerItem *planner.WorkItem
	for i := range plan.Items {
		if plan.Items[i].Repo == "goliatone/go-logger" {
			loggerItem = &plan.Items[i]
			break
		}
	}

	if loggerItem == nil {
		t.Fatalf("expected work item for go-logger")
	}

	if loggerItem.Branch != "override-branch" {
		t.Fatalf("expected override branch, got %s", loggerItem.Branch)
	}

	if len(loggerItem.Tests) != 1 || strings.Join(loggerItem.Tests[0].Cmd, " ") != "task override:test" {
		t.Fatalf("expected override tests, got %#v", loggerItem.Tests)
	}

	if len(loggerItem.ExtraCommands) != 1 || strings.Join(loggerItem.ExtraCommands[0].Cmd, " ") != "task module:lint" {
		t.Fatalf("expected module extra command, got %#v", loggerItem.ExtraCommands)
	}

	if len(loggerItem.Labels) != 1 || loggerItem.Labels[0] != "module:label" {
		t.Fatalf("expected module labels, got %#v", loggerItem.Labels)
	}

	if loggerItem.Notifications.SlackChannel != "#module" {
		t.Fatalf("expected module notifications, got %#v", loggerItem.Notifications)
	}

	if loggerItem.PR.TitleTemplate != "module pr" {
		t.Fatalf("expected module PR title, got %s", loggerItem.PR.TitleTemplate)
	}

	if loggerItem.Env["MODULE_ENV"] != "module" || loggerItem.Env["OVERRIDE_ENV"] != "override" {
		t.Fatalf("expected merged env, got %#v", loggerItem.Env)
	}

	if loggerItem.Timeout != time.Minute {
		t.Fatalf("expected override timeout, got %s", loggerItem.Timeout)
	}
}

func TestPlanner_EmptyPlan_ZeroDependents(t *testing.T) {
	loader := manifest.NewLoader()
	m, err := loader.Load(filepath.Join("testdata", "empty.yaml"))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	target := planner.Target{Module: "github.com/goliatone/go-repository-bun", Version: "v2.0.0"}

	p := planner.New()
	plan, err := p.Plan(context.Background(), m, target)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	var want planner.Plan
	if err := testsupport.LoadGolden(filepath.Join("testdata", "empty_plan.json"), &want); err != nil {
		t.Fatalf("load golden: %v", err)
	}

	if !reflect.DeepEqual(plan, &want) {
		gotJSON, _ := json.MarshalIndent(plan, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Fatalf("plan mismatch\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}

func TestPlanner_FilteredPlan_SkippedDependents(t *testing.T) {
	loader := manifest.NewLoader()
	m, err := loader.Load(filepath.Join("testdata", "filtered.yaml"))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	target := planner.Target{Module: "github.com/goliatone/go-errors", Version: "v1.2.3"}

	p := planner.New()
	plan, err := p.Plan(context.Background(), m, target)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	var want planner.Plan
	if err := testsupport.LoadGolden(filepath.Join("testdata", "filtered_plan.json"), &want); err != nil {
		t.Fatalf("load golden: %v", err)
	}

	if !reflect.DeepEqual(plan, &want) {
		gotJSON, _ := json.MarshalIndent(plan, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Fatalf("plan mismatch\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}

func TestPlanner_AllSkipped_EmptyPlan(t *testing.T) {
	loader := manifest.NewLoader()
	m, err := loader.Load(filepath.Join("testdata", "all_skipped.yaml"))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	target := planner.Target{Module: "github.com/goliatone/go-errors", Version: "v1.2.3"}

	p := planner.New()
	plan, err := p.Plan(context.Background(), m, target)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	var want planner.Plan
	if err := testsupport.LoadGolden(filepath.Join("testdata", "all_skipped_plan.json"), &want); err != nil {
		t.Fatalf("load golden: %v", err)
	}

	if !reflect.DeepEqual(plan, &want) {
		gotJSON, _ := json.MarshalIndent(plan, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Fatalf("plan mismatch\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}

func TestPlanner_InvalidTargetFromManifest(t *testing.T) {
	loader := manifest.NewLoader()
	m, err := loader.Load(filepath.Join("testdata", "invalid_target.yaml"))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	// Try to plan for a module that doesn't exist in this manifest
	target := planner.Target{Module: "github.com/goliatone/go-errors", Version: "v1.2.3"}

	p := planner.New()
	_, err = p.Plan(context.Background(), m, target)

	if err == nil {
		t.Fatal("expected error for nonexistent module in manifest, got nil")
	}

	if !planner.IsTargetNotFound(err) {
		t.Fatalf("expected TargetNotFoundError, got %T: %v", err, err)
	}

	expectedMsg := "planner: target module not found: github.com/goliatone/go-errors"
	if err.Error() != expectedMsg {
		t.Fatalf("expected error message %q, got %q", expectedMsg, err.Error())
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || strings.Contains(s, substr)))
}

// mockDependencyChecker is a test double for dependency checking
type mockDependencyChecker struct {
	needsUpdateFunc func(ctx context.Context, dependent manifest.Dependent, target planner.Target, workspace string) (bool, error)
}

func (m *mockDependencyChecker) NeedsUpdate(ctx context.Context, dependent manifest.Dependent, target planner.Target, workspace string) (bool, error) {
	if m.needsUpdateFunc != nil {
		return m.needsUpdateFunc(ctx, dependent, target, workspace)
	}
	return true, nil // default: needs update
}

func TestPlanner_WithDependencyChecker_SkipsUpToDateDependents(t *testing.T) {
	loader := manifest.NewLoader()
	m, err := loader.Load(filepath.Join("..", "manifest", "testdata", "basic.yaml"))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	target := planner.Target{Module: "github.com/goliatone/go-errors", Version: "v1.2.3"}

	// Mock checker that says all dependents are up-to-date
	checker := &mockDependencyChecker{
		needsUpdateFunc: func(ctx context.Context, dependent manifest.Dependent, target planner.Target, workspace string) (bool, error) {
			return false, nil // all up-to-date
		},
	}

	p := planner.New(
		planner.WithDependencyChecker(checker),
		planner.WithWorkspace("/tmp/workspace"),
	)

	plan, err := p.Plan(context.Background(), m, target)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	// Should have zero work items since all dependents are up-to-date
	if len(plan.Items) != 0 {
		t.Fatalf("expected 0 work items (all up-to-date), got %d", len(plan.Items))
	}
}

func TestPlanner_WithDependencyChecker_IncludesOutdatedDependents(t *testing.T) {
	loader := manifest.NewLoader()
	m, err := loader.Load(filepath.Join("..", "manifest", "testdata", "basic.yaml"))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	target := planner.Target{Module: "github.com/goliatone/go-errors", Version: "v1.2.3"}

	// Mock checker that says first dependent needs update, rest are up-to-date
	callCount := 0
	checker := &mockDependencyChecker{
		needsUpdateFunc: func(ctx context.Context, dependent manifest.Dependent, target planner.Target, workspace string) (bool, error) {
			callCount++
			return callCount == 1, nil // only first one needs update
		},
	}

	p := planner.New(
		planner.WithDependencyChecker(checker),
		planner.WithWorkspace("/tmp/workspace"),
	)

	plan, err := p.Plan(context.Background(), m, target)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	// Should have exactly 1 work item
	if len(plan.Items) != 1 {
		t.Fatalf("expected 1 work item, got %d", len(plan.Items))
	}
}

func TestPlanner_WithDependencyChecker_FailsOpenOnError(t *testing.T) {
	loader := manifest.NewLoader()
	m, err := loader.Load(filepath.Join("..", "manifest", "testdata", "basic.yaml"))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	target := planner.Target{Module: "github.com/goliatone/go-errors", Version: "v1.2.3"}

	// Mock checker that returns an error
	checker := &mockDependencyChecker{
		needsUpdateFunc: func(ctx context.Context, dependent manifest.Dependent, target planner.Target, workspace string) (bool, error) {
			return false, os.ErrNotExist
		},
	}

	p := planner.New(
		planner.WithDependencyChecker(checker),
		planner.WithWorkspace("/tmp/workspace"),
	)

	plan, err := p.Plan(context.Background(), m, target)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	// Should still include work items despite checker errors (fail-open behavior)
	// Load the expected plan to know how many items should be there
	var want planner.Plan
	if err := testsupport.LoadGolden(filepath.Join("testdata", "basic_plan.json"), &want); err != nil {
		t.Fatalf("load golden: %v", err)
	}

	if len(plan.Items) != len(want.Items) {
		t.Fatalf("expected %d work items (fail-open), got %d", len(want.Items), len(plan.Items))
	}
}

func TestPlanner_WithoutDependencyChecker_ProcessesAllDependents(t *testing.T) {
	loader := manifest.NewLoader()
	m, err := loader.Load(filepath.Join("..", "manifest", "testdata", "basic.yaml"))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	target := planner.Target{Module: "github.com/goliatone/go-errors", Version: "v1.2.3"}

	// No checker configured - should process all dependents
	p := planner.New()

	plan, err := p.Plan(context.Background(), m, target)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	var want planner.Plan
	if err := testsupport.LoadGolden(filepath.Join("testdata", "basic_plan.json"), &want); err != nil {
		t.Fatalf("load golden: %v", err)
	}

	if len(plan.Items) != len(want.Items) {
		t.Fatalf("expected %d work items (no checker), got %d", len(want.Items), len(plan.Items))
	}
}

func TestPlanner_WithDependencyChecker_NoWorkspace(t *testing.T) {
	loader := manifest.NewLoader()
	m, err := loader.Load(filepath.Join("..", "manifest", "testdata", "basic.yaml"))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	target := planner.Target{Module: "github.com/goliatone/go-errors", Version: "v1.2.3"}

	// Checker configured but no workspace - should process all dependents
	checker := &mockDependencyChecker{
		needsUpdateFunc: func(ctx context.Context, dependent manifest.Dependent, target planner.Target, workspace string) (bool, error) {
			return false, nil // would skip if workspace was set
		},
	}

	p := planner.New(
		planner.WithDependencyChecker(checker),
		// No WithWorkspace call
	)

	plan, err := p.Plan(context.Background(), m, target)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	var want planner.Plan
	if err := testsupport.LoadGolden(filepath.Join("testdata", "basic_plan.json"), &want); err != nil {
		t.Fatalf("load golden: %v", err)
	}

	// Should process all since workspace is not configured
	if len(plan.Items) != len(want.Items) {
		t.Fatalf("expected %d work items (no workspace), got %d", len(want.Items), len(plan.Items))
	}
}

func TestPlanner_PlanStatistics(t *testing.T) {
	loader := manifest.NewLoader()
	m, err := loader.Load(filepath.Join("..", "manifest", "testdata", "basic.yaml"))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	target := planner.Target{Module: "github.com/goliatone/go-errors", Version: "v1.2.3"}

	t.Run("no checker configured", func(t *testing.T) {
		p := planner.New()
		plan, err := p.Plan(context.Background(), m, target)
		if err != nil {
			t.Fatalf("Plan returned error: %v", err)
		}

		// Without checker, stats should still be populated with basic counts
		if plan.Stats.TotalDependents == 0 {
			t.Error("expected TotalDependents > 0")
		}
		if plan.Stats.WorkItemsCreated != len(plan.Items) {
			t.Errorf("expected WorkItemsCreated=%d, got %d", len(plan.Items), plan.Stats.WorkItemsCreated)
		}
		if plan.Stats.SkippedUpToDate != 0 {
			t.Errorf("expected SkippedUpToDate=0 without checker, got %d", plan.Stats.SkippedUpToDate)
		}
		if len(plan.Stats.SkippedUpToDateRepos) != 0 {
			t.Errorf("expected no skipped repos recorded, got %v", plan.Stats.SkippedUpToDateRepos)
		}
	})

	t.Run("with checker - all up to date", func(t *testing.T) {
		checker := &mockDependencyChecker{
			needsUpdateFunc: func(ctx context.Context, dependent manifest.Dependent, target planner.Target, workspace string) (bool, error) {
				return false, nil // all up to date
			},
		}

		p := planner.New(
			planner.WithDependencyChecker(checker),
			planner.WithWorkspace("/test/workspace"),
		)

		plan, err := p.Plan(context.Background(), m, target)
		if err != nil {
			t.Fatalf("Plan returned error: %v", err)
		}

		if plan.Stats.WorkItemsCreated != 0 {
			t.Errorf("expected WorkItemsCreated=0, got %d", plan.Stats.WorkItemsCreated)
		}
		if plan.Stats.SkippedUpToDate == 0 {
			t.Error("expected SkippedUpToDate > 0")
		}
		if plan.Stats.TotalDependents != plan.Stats.SkippedUpToDate {
			t.Errorf("expected all dependents skipped: total=%d, skipped=%d",
				plan.Stats.TotalDependents, plan.Stats.SkippedUpToDate)
		}
		if len(plan.Stats.SkippedUpToDateRepos) != plan.Stats.SkippedUpToDate {
			t.Errorf("expected skipped repo list length %d, got %d", plan.Stats.SkippedUpToDate, len(plan.Stats.SkippedUpToDateRepos))
		}
	})

	t.Run("with checker - mixed results", func(t *testing.T) {
		callCount := 0
		checker := &mockDependencyChecker{
			needsUpdateFunc: func(ctx context.Context, dependent manifest.Dependent, target planner.Target, workspace string) (bool, error) {
				callCount++
				// First one needs update, rest are up to date
				return callCount == 1, nil
			},
		}

		p := planner.New(
			planner.WithDependencyChecker(checker),
			planner.WithWorkspace("/test/workspace"),
		)

		plan, err := p.Plan(context.Background(), m, target)
		if err != nil {
			t.Fatalf("Plan returned error: %v", err)
		}

		if plan.Stats.WorkItemsCreated != 1 {
			t.Errorf("expected WorkItemsCreated=1, got %d", plan.Stats.WorkItemsCreated)
		}
		if plan.Stats.SkippedUpToDate == 0 {
			t.Error("expected some skipped dependents")
		}
		totalProcessed := plan.Stats.WorkItemsCreated + plan.Stats.SkippedUpToDate
		if totalProcessed != plan.Stats.TotalDependents {
			t.Errorf("expected total=%d, got created=%d + skipped=%d = %d",
				plan.Stats.TotalDependents, plan.Stats.WorkItemsCreated,
				plan.Stats.SkippedUpToDate, totalProcessed)
		}
		if len(plan.Stats.SkippedUpToDateRepos) != plan.Stats.SkippedUpToDate {
			t.Errorf("expected skipped repo list length %d, got %d", plan.Stats.SkippedUpToDate, len(plan.Stats.SkippedUpToDateRepos))
		}
	})

	t.Run("with checker - errors counted", func(t *testing.T) {
		callCount := 0
		checker := &mockDependencyChecker{
			needsUpdateFunc: func(ctx context.Context, dependent manifest.Dependent, target planner.Target, workspace string) (bool, error) {
				callCount++
				if callCount == 1 {
					return false, &planner.DependencyCheckError{
						Dependent: dependent.Repo,
						Target:    target,
						Err:       os.ErrNotExist,
					}
				}
				return true, nil
			},
		}

		p := planner.New(
			planner.WithDependencyChecker(checker),
			planner.WithWorkspace("/test/workspace"),
		)

		plan, err := p.Plan(context.Background(), m, target)
		if err != nil {
			t.Fatalf("Plan returned error: %v", err)
		}

		if plan.Stats.CheckErrors != 1 {
			t.Errorf("expected CheckErrors=1, got %d", plan.Stats.CheckErrors)
		}
		// Error should fail open - work item should be created
		if plan.Stats.WorkItemsCreated < 1 {
			t.Error("expected at least 1 work item created (fail-open on error)")
		}
	})
}
