package hooks

import (
	"path/filepath"
	"testing"

	"github.com/goliatone/cascade/pkg/config"
)

func TestResolveLocalUpdatePlan_MatchesRuleCriteria(t *testing.T) {
	tmpDir := t.TempDir()
	workspace := filepath.Join(tmpDir, "workspace")
	moduleDir := filepath.Join(workspace, "app")
	baseDir := filepath.Join(tmpDir, "config")

	local := config.LocalUpdateHooksConfig{
		After: []config.HookConfig{hookRun("unconditional", "echo unconditional")},
		Rules: []config.LocalUpdateHookRule{{
			Name: "match-all",
			Match: config.LocalUpdateHookMatch{
				Modules:           []string{"github.com/goliatone/app"},
				ModulePrefixes:    []string{"github.com/goliatone/"},
				Workspaces:        []string{workspace},
				WorkspacePrefixes: []string{tmpDir},
				ModuleDirs:        []string{filepath.Join("..", "workspace", "app")},
				ModuleDirPrefixes: []string{filepath.Join("..", "workspace")},
			},
			AfterSuccess: []config.HookConfig{hookRun("rule", "echo rule")},
		}},
	}

	plan := ResolveLocalUpdatePlan(local, Context{
		Module:    "github.com/goliatone/app",
		Workspace: workspace,
		ModuleDir: moduleDir,
	}, []config.ConfigLayer{layer(config.ConfigLayerScopeGlobal, baseDir, local)})

	if len(plan.MatchedRules) != 1 || plan.MatchedRules[0].Name != "match-all" {
		t.Fatalf("expected match-all to match, got %#v", plan.MatchedRules)
	}
	if len(plan.AfterSuccess) != 2 {
		t.Fatalf("expected unconditional plus rule hooks, got %#v", plan.AfterSuccess)
	}
	if plan.AfterSuccess[0].Name != "unconditional" || plan.AfterSuccess[1].Name != "rule" {
		t.Fatalf("unexpected hook order: %#v", plan.AfterSuccess)
	}
}

func TestResolveLocalUpdatePlan_EmptyMatchIsUnconditional(t *testing.T) {
	local := config.LocalUpdateHooksConfig{
		Rules: []config.LocalUpdateHookRule{{
			Name:   "always-rule",
			Always: []config.HookConfig{hookRun("always", "echo always")},
		}},
	}

	plan := ResolveLocalUpdatePlan(local, Context{Module: "github.com/other/app"}, nil)
	if len(plan.MatchedRules) != 1 || plan.MatchedRules[0].Name != "always-rule" {
		t.Fatalf("expected empty match to match, got %#v", plan.MatchedRules)
	}
	if len(plan.Always) != 1 || plan.Always[0].Name != "always" {
		t.Fatalf("unexpected always hooks: %#v", plan.Always)
	}
}

func TestResolveLocalUpdatePlan_ExcludesWin(t *testing.T) {
	local := config.LocalUpdateHooksConfig{
		Rules: []config.LocalUpdateHookRule{{
			Name: "excluded",
			Match: config.LocalUpdateHookMatch{
				ModulePrefixes:        []string{"github.com/goliatone/"},
				ExcludeModulePrefixes: []string{"github.com/goliatone/legacy"},
			},
			AfterSuccess: []config.HookConfig{hookRun("rule", "echo rule")},
		}},
	}

	plan := ResolveLocalUpdatePlan(local, Context{Module: "github.com/goliatone/legacy/app"}, nil)
	if len(plan.MatchedRules) != 0 {
		t.Fatalf("expected rule to be excluded, got %#v", plan.MatchedRules)
	}
	if len(plan.SkippedRules) != 1 || plan.SkippedRules[0].Reason != "module prefix excluded" {
		t.Fatalf("unexpected skipped metadata: %#v", plan.SkippedRules)
	}
}

func TestResolveLocalUpdatePlan_AllPositiveCategoriesMustMatch(t *testing.T) {
	local := config.LocalUpdateHooksConfig{
		Rules: []config.LocalUpdateHookRule{{
			Name: "partial",
			Match: config.LocalUpdateHookMatch{
				Modules:    []string{"github.com/goliatone/app"},
				Workspaces: []string{"/different/workspace"},
			},
			AfterSuccess: []config.HookConfig{hookRun("rule", "echo rule")},
		}},
	}

	plan := ResolveLocalUpdatePlan(local, Context{
		Module:    "github.com/goliatone/app",
		Workspace: "/tmp/workspace",
	}, nil)
	if len(plan.MatchedRules) != 0 {
		t.Fatalf("expected partial match to be skipped, got %#v", plan.MatchedRules)
	}
	if len(plan.SkippedRules) != 1 || plan.SkippedRules[0].Reason != "workspace did not match" {
		t.Fatalf("unexpected skipped metadata: %#v", plan.SkippedRules)
	}
}

func TestResolveLocalUpdatePlan_OverrideAndDisable(t *testing.T) {
	globalLocal := config.LocalUpdateHooksConfig{
		Rules: []config.LocalUpdateHookRule{
			{
				Name:         "test",
				AfterSuccess: []config.HookConfig{hookRun("global-test", "echo global")},
			},
			{
				Name:         "lint",
				AfterSuccess: []config.HookConfig{hookRun("lint", "echo lint")},
			},
		},
	}
	projectLocal := config.LocalUpdateHooksConfig{
		Rules: []config.LocalUpdateHookRule{{
			Name:         "test",
			AfterSuccess: []config.HookConfig{hookRun("project-test", "echo project")},
		}},
		DisabledRules: []string{"lint"},
	}

	plan := ResolveLocalUpdatePlan(config.LocalUpdateHooksConfig{}, Context{}, []config.ConfigLayer{
		layer(config.ConfigLayerScopeGlobal, "/tmp/global", globalLocal),
		layer(config.ConfigLayerScopeProject, "/tmp/project", projectLocal),
	})

	if len(plan.MatchedRules) != 1 || plan.MatchedRules[0].Name != "test" {
		t.Fatalf("expected only overridden test rule to match, got %#v", plan.MatchedRules)
	}
	if plan.MatchedRules[0].Source.Scope != config.ConfigLayerScopeProject {
		t.Fatalf("expected project override source, got %#v", plan.MatchedRules[0].Source)
	}
	if len(plan.AfterSuccess) != 1 || plan.AfterSuccess[0].Name != "project-test" {
		t.Fatalf("expected project hook, got %#v", plan.AfterSuccess)
	}
	if len(plan.DisabledRules) != 1 || plan.DisabledRules[0].Name != "lint" {
		t.Fatalf("expected lint to be disabled, got %#v", plan.DisabledRules)
	}
	if plan.DisabledRules[0].DisabledBy.Scope != config.ConfigLayerScopeProject {
		t.Fatalf("expected project disable source, got %#v", plan.DisabledRules[0])
	}
}

func TestResolveLocalUpdatePlan_HigherPrecedenceRuleOverridesLowerDisable(t *testing.T) {
	globalLocal := config.LocalUpdateHooksConfig{
		DisabledRules: []string{"test"},
	}
	projectLocal := config.LocalUpdateHooksConfig{
		Rules: []config.LocalUpdateHookRule{{
			Name:         "test",
			AfterSuccess: []config.HookConfig{hookRun("project-test", "echo project")},
		}},
	}

	plan := ResolveLocalUpdatePlan(config.LocalUpdateHooksConfig{}, Context{}, []config.ConfigLayer{
		layer(config.ConfigLayerScopeGlobal, "/tmp/global", globalLocal),
		layer(config.ConfigLayerScopeProject, "/tmp/project", projectLocal),
	})

	if len(plan.MatchedRules) != 1 || plan.MatchedRules[0].Name != "test" {
		t.Fatalf("expected project rule to override lower disable, got matched=%#v disabled=%#v", plan.MatchedRules, plan.DisabledRules)
	}
	if len(plan.DisabledRules) != 0 {
		t.Fatalf("expected lower disable to be cleared by higher rule, got %#v", plan.DisabledRules)
	}
}

func TestResolveLocalUpdatePlan_SameLayerDisableWins(t *testing.T) {
	projectLocal := config.LocalUpdateHooksConfig{
		Rules: []config.LocalUpdateHookRule{{
			Name:         "test",
			AfterSuccess: []config.HookConfig{hookRun("project-test", "echo project")},
		}},
		DisabledRules: []string{"test"},
	}

	plan := ResolveLocalUpdatePlan(config.LocalUpdateHooksConfig{}, Context{}, []config.ConfigLayer{
		layer(config.ConfigLayerScopeProject, "/tmp/project", projectLocal),
	})

	if len(plan.MatchedRules) != 0 {
		t.Fatalf("expected same-layer disable to suppress rule, got %#v", plan.MatchedRules)
	}
	if len(plan.DisabledRules) != 1 || plan.DisabledRules[0].Name != "test" {
		t.Fatalf("expected same-layer disabled metadata, got %#v", plan.DisabledRules)
	}
}

func layer(scope config.ConfigLayerScope, baseDir string, local config.LocalUpdateHooksConfig) config.ConfigLayer {
	return config.ConfigLayer{
		Scope:   scope,
		Path:    filepath.Join(baseDir, ".cascade.yaml"),
		BaseDir: baseDir,
		Config: &config.Config{
			Hooks: config.HooksConfig{
				Update: config.UpdateHooksConfig{
					Local: local,
				},
			},
		},
	}
}

func hookRun(name, run string) config.HookConfig {
	return config.HookConfig{Name: name, Run: run}
}
