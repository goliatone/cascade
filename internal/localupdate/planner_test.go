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
	writeLocalModule(t, workspace, "current", "github.com/goliatone/current")
	writeLocalModule(t, workspace, "old", "github.com/goliatone/old")
	writeLocalModule(t, workspace, "indirect", "github.com/goliatone/indirect")
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
	writeLocalModule(t, workspace, "a", "github.com/goliatone/a")
	writeLocalModule(t, workspace, "b", "github.com/goliatone/b")
	writeLocalModule(t, workspace, "c", "github.com/goliatone/c")
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
	writeLocalModule(t, workspace, "missing-version", "github.com/goliatone/missing-version")
	writeLocalModule(t, workspace, "no-v", "github.com/goliatone/no-v")
	writeLocalModule(t, workspace, "invalid", "github.com/goliatone/invalid")
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

func TestPlannerResolvesNestedModuleWithParentVersion(t *testing.T) {
	moduleDir := writeModule(t, `module github.com/goliatone/app

go 1.24

require github.com/goliatone/go-admin/quickstart v0.83.0
`)
	workspace := t.TempDir()
	writeLocalModule(t, workspace, "go-admin", "github.com/goliatone/go-admin")
	writeLocalModule(t, workspace, filepath.Join("go-admin", "quickstart"), "github.com/goliatone/go-admin/quickstart")
	writeSiblingVersion(t, workspace, "go-admin", "0.90.0")

	plan, err := PlanLocal(Request{
		ModuleDir:         moduleDir,
		Workspace:         workspace,
		WorkspaceExplicit: true,
	})
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	item := assertItemStatus(t, plan, "github.com/goliatone/go-admin/quickstart", StatusUpdate, true)
	if item.LocalVersion != "v0.90.0" {
		t.Fatalf("expected parent version v0.90.0, got %q", item.LocalVersion)
	}
	if item.LocalPath != filepath.Join(workspace, "go-admin", "quickstart") {
		t.Fatalf("expected nested local path, got %q", item.LocalPath)
	}
}

func TestPlannerNestedModuleVersionFileWinsOverParent(t *testing.T) {
	moduleDir := writeModule(t, `module github.com/goliatone/app

go 1.24

require github.com/goliatone/go-admin/quickstart v0.83.0
`)
	workspace := t.TempDir()
	writeLocalModule(t, workspace, "go-admin", "github.com/goliatone/go-admin")
	writeLocalModule(t, workspace, filepath.Join("go-admin", "quickstart"), "github.com/goliatone/go-admin/quickstart")
	writeSiblingVersion(t, workspace, "go-admin", "0.90.0")
	writeSiblingVersion(t, workspace, filepath.Join("go-admin", "quickstart"), "0.91.0")

	plan, err := PlanLocal(Request{
		ModuleDir:         moduleDir,
		Workspace:         workspace,
		WorkspaceExplicit: true,
	})
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	item := assertItemStatus(t, plan, "github.com/goliatone/go-admin/quickstart", StatusUpdate, true)
	if item.LocalVersion != "v0.91.0" {
		t.Fatalf("expected module version v0.91.0, got %q", item.LocalVersion)
	}
}

func TestPlannerMajorVersionModuleResolvesRepoRoot(t *testing.T) {
	moduleDir := writeModule(t, `module github.com/goliatone/app

go 1.24

require github.com/goliatone/lib/v2 v2.0.0
`)
	workspace := t.TempDir()
	writeLocalModule(t, workspace, "lib", "github.com/goliatone/lib/v2")
	writeSiblingVersion(t, workspace, "lib", "2.1.0")

	plan, err := PlanLocal(Request{
		ModuleDir:         moduleDir,
		Workspace:         workspace,
		WorkspaceExplicit: true,
	})
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	item := assertItemStatus(t, plan, "github.com/goliatone/lib/v2", StatusUpdate, true)
	if item.LocalPath != filepath.Join(workspace, "lib") {
		t.Fatalf("expected major-version module to resolve to repo root, got %q", item.LocalPath)
	}
	if item.LocalVersion != "v2.1.0" {
		t.Fatalf("expected v2.1.0, got %q", item.LocalVersion)
	}
}

func TestPlannerMajorVersionModuleResolvesSubmoduleWhenRepoRootIsV1(t *testing.T) {
	moduleDir := writeModule(t, `module github.com/goliatone/app

go 1.24

require github.com/goliatone/lib/v2 v2.0.0
`)
	workspace := filepath.Join(t.TempDir(), "goliatone")
	writeLocalModule(t, workspace, "lib", "github.com/goliatone/lib")
	writeLocalModule(t, workspace, filepath.Join("lib", "v2"), "github.com/goliatone/lib/v2")
	writeSiblingVersion(t, workspace, "lib", "2.1.0")

	plan, err := PlanLocal(Request{
		ModuleDir:         moduleDir,
		Workspace:         workspace,
		WorkspaceExplicit: true,
	})
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	item := assertItemStatus(t, plan, "github.com/goliatone/lib/v2", StatusUpdate, true)
	if item.LocalPath != filepath.Join(workspace, "lib", "v2") {
		t.Fatalf("expected major-version submodule to resolve after v1 repo root, got %q", item.LocalPath)
	}
	if item.LocalVersion != "v2.1.0" {
		t.Fatalf("expected parent repo version v2.1.0, got %q", item.LocalVersion)
	}
}

func TestPlannerMajorVersionV10ModuleResolvesRepoRoot(t *testing.T) {
	moduleDir := writeModule(t, `module github.com/goliatone/app

go 1.24

require github.com/goliatone/lib/v10 v10.0.0
`)
	workspace := t.TempDir()
	writeLocalModule(t, workspace, "lib", "github.com/goliatone/lib/v10")
	writeSiblingVersion(t, workspace, "lib", "10.1.0")

	plan, err := PlanLocal(Request{
		ModuleDir:         moduleDir,
		Workspace:         workspace,
		WorkspaceExplicit: true,
	})
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	item := assertItemStatus(t, plan, "github.com/goliatone/lib/v10", StatusUpdate, true)
	if item.LocalPath != filepath.Join(workspace, "lib") {
		t.Fatalf("expected v10 module to resolve to repo root, got %q", item.LocalPath)
	}
	if item.LocalVersion != "v10.1.0" {
		t.Fatalf("expected v10.1.0, got %q", item.LocalVersion)
	}
}

func TestPlannerScansWorkspaceWhenWorkspaceIsRepoRoot(t *testing.T) {
	moduleDir := writeModule(t, `module github.com/goliatone/app

go 1.24

require github.com/goliatone/go-admin/quickstart v0.83.0
`)
	workspace := filepath.Join(t.TempDir(), "go-admin")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	writeLocalModule(t, workspace, "quickstart", "github.com/goliatone/go-admin/quickstart")
	if err := os.WriteFile(filepath.Join(workspace, ".version"), []byte("0.90.0"), 0o644); err != nil {
		t.Fatalf("write version: %v", err)
	}

	plan, err := PlanLocal(Request{
		ModuleDir:         moduleDir,
		Workspace:         workspace,
		WorkspaceExplicit: true,
	})
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	item := assertItemStatus(t, plan, "github.com/goliatone/go-admin/quickstart", StatusUpdate, true)
	if item.LocalPath != filepath.Join(workspace, "quickstart") {
		t.Fatalf("expected scanned nested module path, got %q", item.LocalPath)
	}
	if item.LocalVersion != "v0.90.0" {
		t.Fatalf("expected repo root version fallback, got %q", item.LocalVersion)
	}
}

func TestPlannerUsesDirectCandidatePrecedence(t *testing.T) {
	moduleDir := writeModule(t, `module github.com/goliatone/app

go 1.24

require github.com/goliatone/go-admin/quickstart v0.83.0
`)
	workspace := t.TempDir()
	writeLocalModule(t, workspace, filepath.Join("go-admin", "quickstart"), "github.com/goliatone/go-admin/quickstart")
	writeLocalModule(t, workspace, "quickstart", "github.com/goliatone/go-admin/quickstart")
	writeSiblingVersion(t, workspace, "go-admin", "0.90.0")
	writeSiblingVersion(t, workspace, "quickstart", "0.91.0")

	plan, err := PlanLocal(Request{
		ModuleDir:         moduleDir,
		Workspace:         workspace,
		WorkspaceExplicit: true,
	})
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	item := assertItemStatus(t, plan, "github.com/goliatone/go-admin/quickstart", StatusUpdate, true)
	if item.LocalPath != filepath.Join(workspace, "go-admin", "quickstart") {
		t.Fatalf("expected highest-priority direct candidate, got %q", item.LocalPath)
	}
	if item.LocalVersion != "v0.90.0" {
		t.Fatalf("expected parent version from preferred candidate, got %q", item.LocalVersion)
	}
}

func TestPlannerIgnoresMismatchedBasenameFallback(t *testing.T) {
	moduleDir := writeModule(t, `module github.com/goliatone/app

go 1.24

require github.com/goliatone/foo/bar v0.1.0
`)
	workspace := t.TempDir()
	writeLocalModule(t, workspace, "bar", "github.com/goliatone/bar")
	writeSiblingVersion(t, workspace, "bar", "0.2.0")

	plan, err := PlanLocal(Request{
		ModuleDir:         moduleDir,
		Workspace:         workspace,
		WorkspaceExplicit: true,
	})
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	assertItemStatus(t, plan, "github.com/goliatone/foo/bar", StatusMissingLocalRepo, false)
}

func TestPlannerMajorVersionRootPrecedesSuffixedMismatch(t *testing.T) {
	moduleDir := writeModule(t, `module github.com/goliatone/app

go 1.24

require github.com/goliatone/lib/v2 v2.0.0
`)
	workspace := t.TempDir()
	writeLocalModule(t, workspace, "lib", "github.com/goliatone/lib/v2")
	writeLocalModule(t, workspace, filepath.Join("lib", "v2"), "github.com/goliatone/lib/v2/internal")
	writeSiblingVersion(t, workspace, "lib", "2.1.0")

	plan, err := PlanLocal(Request{
		ModuleDir:         moduleDir,
		Workspace:         workspace,
		WorkspaceExplicit: true,
	})
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	item := assertItemStatus(t, plan, "github.com/goliatone/lib/v2", StatusUpdate, true)
	if item.LocalPath != filepath.Join(workspace, "lib") {
		t.Fatalf("expected major-version root to win, got %q", item.LocalPath)
	}
}

func TestPlannerReportsLocalModulePathMismatch(t *testing.T) {
	moduleDir := writeModule(t, `module github.com/goliatone/app

go 1.24

require github.com/goliatone/go-admin/quickstart v0.83.0
`)
	workspace := filepath.Join(t.TempDir(), "goliatone")
	writeLocalModule(t, workspace, filepath.Join("go-admin", "quickstart"), "github.com/goliatone/go-admin/not-quickstart")
	writeSiblingVersion(t, workspace, "go-admin", "0.90.0")

	plan, err := PlanLocal(Request{
		ModuleDir:         moduleDir,
		Workspace:         workspace,
		WorkspaceExplicit: true,
	})
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	assertItemStatus(t, plan, "github.com/goliatone/go-admin/quickstart", StatusInvalidLocalModule, false)
}

func TestPlannerIgnoresMalformedScannedCandidate(t *testing.T) {
	moduleDir := writeModule(t, `module github.com/goliatone/app

go 1.24

require github.com/goliatone/go-admin/quickstart v0.83.0
`)
	workspace := t.TempDir()
	writeLocalModule(t, workspace, "go-admin", "github.com/goliatone/go-admin")
	writeRawGoMod(t, workspace, filepath.Join("go-admin", "broken"), "module")
	writeSiblingVersion(t, workspace, "go-admin", "0.90.0")

	plan, err := PlanLocal(Request{
		ModuleDir:         moduleDir,
		Workspace:         workspace,
		WorkspaceExplicit: true,
	})
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	assertItemStatus(t, plan, "github.com/goliatone/go-admin/quickstart", StatusMissingLocalRepo, false)
}

func TestPlannerIgnoresMalformedBroadWorkspaceScanCandidate(t *testing.T) {
	moduleDir := writeModule(t, `module github.com/goliatone/app

go 1.24

require github.com/goliatone/missing v0.1.0
`)
	workspace := t.TempDir()
	writeRawGoMod(t, workspace, "unrelated", "module")

	plan, err := PlanLocal(Request{
		ModuleDir:         moduleDir,
		Workspace:         workspace,
		WorkspaceExplicit: true,
	})
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	assertItemStatus(t, plan, "github.com/goliatone/missing", StatusMissingLocalRepo, false)
}

func TestPlannerReportsMalformedLocalModule(t *testing.T) {
	moduleDir := writeModule(t, `module github.com/goliatone/app

go 1.24

require github.com/goliatone/go-admin/quickstart v0.83.0
`)
	workspace := filepath.Join(t.TempDir(), "goliatone")
	writeRawGoMod(t, workspace, filepath.Join("go-admin", "quickstart"), "module")
	writeSiblingVersion(t, workspace, "go-admin", "0.90.0")

	plan, err := PlanLocal(Request{
		ModuleDir:         moduleDir,
		Workspace:         workspace,
		WorkspaceExplicit: true,
	})
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	assertItemStatus(t, plan, "github.com/goliatone/go-admin/quickstart", StatusInvalidLocalModule, false)
}

func TestPlannerHostRootPrefersOrgRepoPath(t *testing.T) {
	moduleDir := writeModule(t, `module github.com/goliatone/app

go 1.24

require github.com/goliatone/go-admin v0.89.0
`)
	workspace := filepath.Join(t.TempDir(), "github.com")
	writeLocalModule(t, workspace, "go-admin", "github.com/other/go-admin")
	writeLocalModule(t, workspace, filepath.Join("goliatone", "go-admin"), "github.com/goliatone/go-admin")
	writeSiblingVersion(t, workspace, filepath.Join("goliatone", "go-admin"), "0.90.0")

	plan, err := PlanLocal(Request{
		ModuleDir:         moduleDir,
		Workspace:         workspace,
		WorkspaceExplicit: true,
	})
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	item := assertItemStatus(t, plan, "github.com/goliatone/go-admin", StatusUpdate, true)
	if item.LocalPath != filepath.Join(workspace, "goliatone", "go-admin") {
		t.Fatalf("expected host-root org/repo path, got %q", item.LocalPath)
	}
}

func TestPlannerGenericRootPrefersFullModulePath(t *testing.T) {
	moduleDir := writeModule(t, `module github.com/goliatone/app

go 1.24

require github.com/goliatone/go-admin v0.89.0
`)
	workspace := filepath.Join(t.TempDir(), "src")
	writeLocalModule(t, workspace, "go-admin", "github.com/other/go-admin")
	writeLocalModule(t, workspace, filepath.Join("github.com", "goliatone", "go-admin"), "github.com/goliatone/go-admin")
	writeSiblingVersion(t, workspace, filepath.Join("github.com", "goliatone", "go-admin"), "0.90.0")

	plan, err := PlanLocal(Request{
		ModuleDir:         moduleDir,
		Workspace:         workspace,
		WorkspaceExplicit: true,
	})
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	item := assertItemStatus(t, plan, "github.com/goliatone/go-admin", StatusUpdate, true)
	if item.LocalPath != filepath.Join(workspace, "github.com", "goliatone", "go-admin") {
		t.Fatalf("expected generic-root full module path, got %q", item.LocalPath)
	}
}

func TestPlannerOrgRootPrefersRepoPath(t *testing.T) {
	moduleDir := writeModule(t, `module github.com/goliatone/app

go 1.24

require github.com/goliatone/go-admin v0.89.0
`)
	workspace := filepath.Join(t.TempDir(), "goliatone")
	writeLocalModule(t, workspace, "go-admin", "github.com/goliatone/go-admin")
	writeLocalModule(t, workspace, filepath.Join("github.com", "goliatone", "go-admin"), "github.com/other/go-admin")
	writeSiblingVersion(t, workspace, "go-admin", "0.90.0")

	plan, err := PlanLocal(Request{
		ModuleDir:         moduleDir,
		Workspace:         workspace,
		WorkspaceExplicit: true,
	})
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	item := assertItemStatus(t, plan, "github.com/goliatone/go-admin", StatusUpdate, true)
	if item.LocalPath != filepath.Join(workspace, "go-admin") {
		t.Fatalf("expected org-root repo path, got %q", item.LocalPath)
	}
}

func TestPlannerResolvesVanityNestedModuleWithParentVersion(t *testing.T) {
	moduleDir := writeModule(t, `module go.example.com/app

go 1.24

require go.example.com/foo/bar v0.1.0
`)
	workspace := t.TempDir()
	writeLocalModule(t, workspace, filepath.Join("foo", "bar"), "go.example.com/foo/bar")
	writeSiblingVersion(t, workspace, "foo", "0.2.0")

	plan, err := PlanLocal(Request{
		ModuleDir:         moduleDir,
		Workspace:         workspace,
		WorkspaceExplicit: true,
		Prefixes:          []string{"go.example.com/"},
	})
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	item := assertItemStatus(t, plan, "go.example.com/foo/bar", StatusUpdate, true)
	if item.LocalPath != filepath.Join(workspace, "foo", "bar") {
		t.Fatalf("expected vanity nested module path, got %q", item.LocalPath)
	}
	if item.LocalVersion != "v0.2.0" {
		t.Fatalf("expected parent version v0.2.0, got %q", item.LocalVersion)
	}
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

func writeLocalModule(t *testing.T, workspace, relativePath, modulePath string) {
	t.Helper()
	dir := filepath.Join(workspace, relativePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create local module: %v", err)
	}
	content := "module " + modulePath + "\n\ngo 1.24\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0o644); err != nil {
		t.Fatalf("write local go.mod: %v", err)
	}
}

func writeRawGoMod(t *testing.T, workspace, relativePath, content string) {
	t.Helper()
	dir := filepath.Join(workspace, relativePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create local module: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0o644); err != nil {
		t.Fatalf("write raw go.mod: %v", err)
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
