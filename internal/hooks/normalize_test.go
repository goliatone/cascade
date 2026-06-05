package hooks_test

import (
	"testing"

	"github.com/goliatone/cascade/internal/hooks"
	"github.com/goliatone/cascade/pkg/config"
)

func TestNormalizeLocalUpdate_AppendsAfterAliasBeforeAfterSuccess(t *testing.T) {
	plan := hooks.NormalizeLocalUpdate(config.LocalUpdateHooksConfig{
		After: []config.HookConfig{{
			Name: "alias",
			Run:  "echo alias",
		}},
		AfterSuccess: []config.HookConfig{{
			Name: "success",
			Run:  "echo success",
		}},
		AfterFailure: []config.HookConfig{{
			Name: "failure",
			Run:  "echo failure",
		}},
		Always: []config.HookConfig{{
			Name: "always",
			Run:  "echo always",
		}},
	})

	if len(plan.AfterSuccess) != 2 {
		t.Fatalf("expected two success hooks, got %d", len(plan.AfterSuccess))
	}
	if plan.AfterSuccess[0].Name != "alias" || plan.AfterSuccess[1].Name != "success" {
		t.Fatalf("unexpected success hook order: %#v", plan.AfterSuccess)
	}
	if len(plan.AfterFailure) != 1 || plan.AfterFailure[0].Name != "failure" {
		t.Fatalf("unexpected failure hooks: %#v", plan.AfterFailure)
	}
	if len(plan.Always) != 1 || plan.Always[0].Name != "always" {
		t.Fatalf("unexpected always hooks: %#v", plan.Always)
	}
}

func TestSelectedPhases(t *testing.T) {
	plan := hooks.LocalUpdatePlan{
		AfterSuccess: []config.HookConfig{{Name: "success", Run: "echo success"}},
		AfterFailure: []config.HookConfig{{Name: "failure", Run: "echo failure"}},
		Always:       []config.HookConfig{{Name: "always", Run: "echo always"}},
	}

	success := plan.SelectedPhases(true)
	if len(success) != 2 || success[0].Phase != hooks.PhaseAfterSuccess || success[1].Phase != hooks.PhaseAlways {
		t.Fatalf("unexpected success phases: %#v", success)
	}

	failure := plan.SelectedPhases(false)
	if len(failure) != 2 || failure[0].Phase != hooks.PhaseAfterFailure || failure[1].Phase != hooks.PhaseAlways {
		t.Fatalf("unexpected failure phases: %#v", failure)
	}
}
