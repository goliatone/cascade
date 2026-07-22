package localupdate

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestPlanRepositoryPlansEveryModuleBeforeApply(t *testing.T) {
	workspace := t.TempDir()
	repositoryRoot := filepath.Join(workspace, "app")
	rootDir := repositoryRoot
	nestedDir := filepath.Join(repositoryRoot, "adapter")
	mustWriteModuleFile(t, rootDir, "github.com/goliatone/app", "\nrequire github.com/goliatone/dep-a v1.0.0\n")
	mustWriteModuleFile(t, nestedDir, "github.com/goliatone/app/adapter", "\nrequire github.com/goliatone/dep-b v1.0.0\n")
	writeVersionedLocalModule(t, workspace, "dep-a", "v1.1.0")
	writeVersionedLocalModule(t, workspace, "dep-b", "v1.2.0")
	repository := Repository{Root: repositoryRoot, Modules: []ModuleTarget{
		{ModulePath: "github.com/goliatone/app", ModuleDir: rootDir},
		{ModulePath: "github.com/goliatone/app/adapter", ModuleDir: nestedDir},
	}}

	plan, err := PlanRepository(repository, Request{Workspace: workspace, WorkspaceExplicit: true})
	if err != nil {
		t.Fatalf("plan repository: %v", err)
	}
	if len(plan.Plans) != 2 || plan.Updates() != 2 || plan.Candidates() != 2 {
		t.Fatalf("unexpected repository plan: %#v", plan)
	}
}

func TestPlanRepositoryInfersSiblingWorkspaceWithoutRootModule(t *testing.T) {
	workspace := t.TempDir()
	repositoryRoot := filepath.Join(workspace, "app")
	nestedDir := filepath.Join(repositoryRoot, "adapter")
	mustWriteModuleFile(t, nestedDir, "github.com/goliatone/app/adapter", "\nrequire github.com/goliatone/dep-a v1.0.0\n")
	writeVersionedLocalModule(t, workspace, "dep-a", "v1.1.0")
	repository := Repository{Root: repositoryRoot, Modules: []ModuleTarget{{
		ModulePath: "github.com/goliatone/app/adapter",
		ModuleDir:  nestedDir,
	}}}

	plan, err := PlanRepository(repository, Request{})
	if err != nil {
		t.Fatalf("plan repository: %v", err)
	}
	if plan.Workspace() != workspace || plan.Updates() != 1 {
		t.Fatalf("expected inferred sibling workspace %q and one update, got %#v", workspace, plan)
	}
}

func TestApplyRepositoryContinuesAcrossModuleFailureAndTidiesSuccess(t *testing.T) {
	firstDir := filepath.Join(t.TempDir(), "first")
	secondDir := filepath.Join(t.TempDir(), "second")
	plan := RepositoryPlan{Repository: Repository{Root: filepath.Dir(firstDir)}, Plans: []Plan{
		repositoryApplyPlan("github.com/goliatone/first", firstDir, "github.com/goliatone/dep-a"),
		repositoryApplyPlan("github.com/goliatone/second", secondDir, "github.com/goliatone/dep-b"),
	}}
	ops := &mockGoOps{getErrors: map[string]error{"github.com/goliatone/dep-a": errors.New("planned failure")}}

	result, err := ApplyRepository(context.Background(), plan, ops, ApplyOptions{Tidy: true})
	if err == nil {
		t.Fatal("expected aggregate failure")
	}
	if len(ops.getCalls) != 2 {
		t.Fatalf("expected both modules to be attempted, got %#v", ops.getCalls)
	}
	if len(ops.tidyCalls) != 1 || ops.tidyCalls[0] != secondDir {
		t.Fatalf("expected only successful module to be tidied, got %#v", ops.tidyCalls)
	}
	if result.UpdatedCount() != 1 || result.TidyCount() != 1 || !result.HasFailures {
		t.Fatalf("unexpected aggregate result: %#v", result)
	}
}

func TestApplyRepositoryStopsRemainingModulesAfterCancellation(t *testing.T) {
	firstDir := filepath.Join(t.TempDir(), "first")
	secondDir := filepath.Join(t.TempDir(), "second")
	plan := RepositoryPlan{Repository: Repository{Root: filepath.Dir(firstDir)}, Plans: []Plan{
		repositoryApplyPlan("github.com/goliatone/first", firstDir, "github.com/goliatone/dep-a"),
		repositoryApplyPlan("github.com/goliatone/second", secondDir, "github.com/goliatone/dep-b"),
	}}
	ops := &blockingGoOps{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	result, err := ApplyRepository(ctx, plan, ops, ApplyOptions{Tidy: true})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline failure, got %v", err)
	}
	if len(ops.getCalls) != 1 {
		t.Fatalf("expected only first module command, got %#v", ops.getCalls)
	}
	if len(result.Results) != 2 || result.Results[1].Interruption != context.DeadlineExceeded {
		t.Fatalf("expected remaining module to be marked interrupted, got %#v", result.Results)
	}
}

func repositoryApplyPlan(modulePath, moduleDir, dependency string) Plan {
	return Plan{CurrentModule: modulePath, ModuleDir: moduleDir, Items: []Item{{
		Module: dependency, CurrentVersion: "v1.0.0", LocalVersion: "v1.1.0", Status: StatusUpdate, NeedsUpdate: true,
	}}}
}

func writeVersionedLocalModule(t *testing.T, workspace, name, version string) {
	t.Helper()
	dir := filepath.Join(workspace, name)
	mustWriteModuleFile(t, dir, "github.com/goliatone/"+name, "")
	mustWriteFile(t, filepath.Join(dir, ".version"), version)
}
