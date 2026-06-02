package localupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlannerDirectGoliatoneDependenciesByDefault(t *testing.T) {
	moduleDir := writeModule(t, `module github.com/goliatone/app

go 1.24

require (
	github.com/goliatone/current v1.2.0
	github.com/goliatone/old v1.0.0
	github.com/goliatone/indirect v1.0.0 // indirect
	github.com/acme/other v1.0.0
)
`)
	workspace := t.TempDir()
	writeSiblingVersion(t, workspace, "current", "v1.2.0")
	writeSiblingVersion(t, workspace, "old", "v1.1.0")
	writeSiblingVersion(t, workspace, "indirect", "v1.1.0")

	plan, err := PlanLocal(Request{
		CurrentModule:     "github.com/goliatone/app",
		ModuleDir:         moduleDir,
		Workspace:         workspace,
		WorkspaceExplicit: true,
	})
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	assertItemStatus(t, plan, "github.com/goliatone/current", StatusCurrent, false)
	assertItemStatus(t, plan, "github.com/goliatone/old", StatusUpdate, true)
	assertItemStatus(t, plan, "github.com/goliatone/indirect", StatusSkippedIndirect, false)
	assertMissingItem(t, plan, "github.com/acme/other")
}

func TestPlannerIncludeIndirectAndFilters(t *testing.T) {
	moduleDir := writeModule(t, `module github.com/goliatone/app

go 1.24

require (
	github.com/goliatone/a v1.0.0
	github.com/goliatone/b v1.0.0 // indirect
	github.com/goliatone/c v1.0.0
)
`)
	workspace := t.TempDir()
	writeSiblingVersion(t, workspace, "a", "v1.1.0")
	writeSiblingVersion(t, workspace, "b", "v1.1.0")
	writeSiblingVersion(t, workspace, "c", "v1.1.0")

	plan, err := PlanLocal(Request{
		ModuleDir:         moduleDir,
		Workspace:         workspace,
		WorkspaceExplicit: true,
		IncludeIndirect:   true,
		Only:              []string{"a,b"},
		Exclude:           []string{"a"},
	})
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	assertItemStatus(t, plan, "github.com/goliatone/a", StatusSkippedFilter, false)
	assertItemStatus(t, plan, "github.com/goliatone/b", StatusUpdate, true)
	assertMissingItem(t, plan, "github.com/goliatone/c")
}

func TestPlannerSkipsReplaceDirective(t *testing.T) {
	moduleDir := writeModule(t, `module github.com/goliatone/app

go 1.24

require github.com/goliatone/replaced v1.0.0

replace github.com/goliatone/replaced => ../replaced
`)
	workspace := t.TempDir()
	writeSiblingVersion(t, workspace, "replaced", "v1.1.0")

	plan, err := PlanLocal(Request{
		ModuleDir:         moduleDir,
		Workspace:         workspace,
		WorkspaceExplicit: true,
	})
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	assertItemStatus(t, plan, "github.com/goliatone/replaced", StatusSkippedReplace, false)
}

func TestPlannerLocalVersionResolutionAndFailures(t *testing.T) {
	moduleDir := writeModule(t, `module github.com/goliatone/app

go 1.24

require (
	github.com/goliatone/missing-repo v1.0.0
	github.com/goliatone/missing-version v1.0.0
	github.com/goliatone/no-v v1.0.0
	github.com/goliatone/invalid v1.0.0
)
`)
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "missing-version"), 0o755); err != nil {
		t.Fatalf("create sibling: %v", err)
	}
	writeSiblingVersion(t, workspace, "no-v", "1.2.0")
	writeSiblingVersion(t, workspace, "invalid", "not-a-version")

	plan, err := PlanLocal(Request{
		ModuleDir:         moduleDir,
		Workspace:         workspace,
		WorkspaceExplicit: true,
	})
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	assertItemStatus(t, plan, "github.com/goliatone/missing-repo", StatusMissingLocalRepo, false)
	assertItemStatus(t, plan, "github.com/goliatone/missing-version", StatusMissingVersionFile, false)
	item := assertItemStatus(t, plan, "github.com/goliatone/no-v", StatusUpdate, true)
	if item.LocalVersion != "v1.2.0" {
		t.Fatalf("expected normalized local version v1.2.0, got %q", item.LocalVersion)
	}
	assertItemStatus(t, plan, "github.com/goliatone/invalid", StatusInvalidVersion, false)
}

func TestPlannerRejectsCacheFallbackWorkspace(t *testing.T) {
	moduleDir := writeModule(t, `module github.com/goliatone/app

go 1.24

require github.com/goliatone/dep v1.0.0
`)
	planner := &Planner{
		ResolveWorkspace: func(req Request, currentModule, moduleDir string) (WorkspaceResolution, error) {
			return WorkspaceResolution{Path: filepath.Join(t.TempDir(), ".cache", "cascade"), CacheFallback: true}, nil
		},
	}

	_, err := planner.Plan(Request{ModuleDir: moduleDir})
	if err == nil {
		t.Fatal("expected cache fallback error")
	}
}

func writeModule(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	return dir
}

func writeSiblingVersion(t *testing.T, workspace, name, version string) {
	t.Helper()
	dir := filepath.Join(workspace, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create sibling: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".version"), []byte(version), 0o644); err != nil {
		t.Fatalf("write version: %v", err)
	}
}

func assertItemStatus(t *testing.T, plan Plan, module string, status Status, needsUpdate bool) Item {
	t.Helper()
	for _, item := range plan.Items {
		if item.Module == module {
			if item.Status != status {
				t.Fatalf("expected %s status %s, got %s (%s)", module, status, item.Status, item.Reason)
			}
			if item.NeedsUpdate != needsUpdate {
				t.Fatalf("expected %s NeedsUpdate=%t, got %t", module, needsUpdate, item.NeedsUpdate)
			}
			return item
		}
	}
	t.Fatalf("missing item %s in %#v", module, plan.Items)
	return Item{}
}

func assertMissingItem(t *testing.T, plan Plan, module string) {
	t.Helper()
	for _, item := range plan.Items {
		if item.Module == module {
			t.Fatalf("expected %s to be absent, got %#v", module, item)
		}
	}
}
