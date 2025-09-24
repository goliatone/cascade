package planner_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

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
