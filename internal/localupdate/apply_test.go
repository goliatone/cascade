package localupdate

import (
	"context"
	"errors"
	"testing"
)

type mockGoOps struct {
	getCalls  []getCall
	tidyCalls []string
	getErrors map[string]error
	tidyError error
}

type getCall struct {
	repoPath string
	module   string
	version  string
}

func (m *mockGoOps) Get(ctx context.Context, repoPath, module, version string) error {
	m.getCalls = append(m.getCalls, getCall{repoPath: repoPath, module: module, version: version})
	if err := m.getErrors[module]; err != nil {
		return err
	}
	return nil
}

func (m *mockGoOps) Tidy(ctx context.Context, repoPath string) error {
	m.tidyCalls = append(m.tidyCalls, repoPath)
	return m.tidyError
}

func TestApplyPlanDryRunDoesNotCallGo(t *testing.T) {
	ops := &mockGoOps{}

	result, err := ApplyPlan(context.Background(), applyTestPlan(), ops, ApplyOptions{DryRun: true, Tidy: true})
	if err != nil {
		t.Fatalf("dry-run apply failed: %v", err)
	}
	if len(ops.getCalls) != 0 || len(ops.tidyCalls) != 0 {
		t.Fatalf("dry-run called go operations: %#v %#v", ops.getCalls, ops.tidyCalls)
	}
	if len(result.Items) != len(applyTestPlan().Items) {
		t.Fatalf("expected result items to mirror plan")
	}
}

func TestApplyPlanRunsGoGetAndTidyOnce(t *testing.T) {
	ops := &mockGoOps{}

	result, err := ApplyPlan(context.Background(), applyTestPlan(), ops, ApplyOptions{Tidy: true})
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if len(ops.getCalls) != 2 {
		t.Fatalf("expected two go get calls, got %#v", ops.getCalls)
	}
	if ops.getCalls[0].module != "github.com/goliatone/a" || ops.getCalls[0].version != "v1.1.0" {
		t.Fatalf("unexpected first get call: %#v", ops.getCalls[0])
	}
	if ops.getCalls[1].module != "github.com/goliatone/b" || ops.getCalls[1].version != "v1.2.0" {
		t.Fatalf("unexpected second get call: %#v", ops.getCalls[1])
	}
	if len(ops.tidyCalls) != 1 || !result.TidyRun {
		t.Fatalf("expected one tidy call, got %#v", ops.tidyCalls)
	}
	assertApplyStatus(t, result, "github.com/goliatone/a", StatusApplied)
	assertApplyStatus(t, result, "github.com/goliatone/b", StatusApplied)
}

func TestApplyPlanSkipsTidyWhenDisabled(t *testing.T) {
	ops := &mockGoOps{}

	result, err := ApplyPlan(context.Background(), applyTestPlan(), ops, ApplyOptions{Tidy: false})
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if len(ops.tidyCalls) != 0 || result.TidyRun {
		t.Fatalf("expected tidy to be skipped")
	}
}

func TestApplyPlanContinuesAfterGoGetFailure(t *testing.T) {
	ops := &mockGoOps{
		getErrors: map[string]error{
			"github.com/goliatone/a": errors.New("get a failed"),
		},
	}

	result, err := ApplyPlan(context.Background(), applyTestPlan(), ops, ApplyOptions{Tidy: true})
	if err == nil {
		t.Fatal("expected apply failure")
	}
	if len(ops.getCalls) != 2 {
		t.Fatalf("expected later update to be attempted, got %#v", ops.getCalls)
	}
	if len(ops.tidyCalls) != 1 {
		t.Fatalf("expected tidy after successful later update, got %#v", ops.tidyCalls)
	}
	assertApplyStatus(t, result, "github.com/goliatone/a", StatusApplyFailed)
	assertApplyStatus(t, result, "github.com/goliatone/b", StatusApplied)
	if !result.HasFailures {
		t.Fatalf("expected HasFailures")
	}
}

func TestApplyPlanTidyFailureFailsOverall(t *testing.T) {
	ops := &mockGoOps{tidyError: errors.New("tidy failed")}

	result, err := ApplyPlan(context.Background(), applyTestPlan(), ops, ApplyOptions{Tidy: true})
	if err == nil {
		t.Fatal("expected tidy failure")
	}
	if !result.TidyRun || !result.TidyFailed || result.TidyError == nil {
		t.Fatalf("expected tidy failure result, got %#v", result)
	}
}

func applyTestPlan() Plan {
	return Plan{
		ModuleDir: "/tmp/module",
		Items: []Item{
			{Module: "github.com/goliatone/a", CurrentVersion: "v1.0.0", LocalVersion: "v1.1.0", Status: StatusUpdate, NeedsUpdate: true},
			{Module: "github.com/goliatone/current", CurrentVersion: "v1.0.0", LocalVersion: "v1.0.0", Status: StatusCurrent},
			{Module: "github.com/goliatone/b", CurrentVersion: "v1.0.0", LocalVersion: "v1.2.0", Status: StatusUpdate, NeedsUpdate: true},
		},
	}
}

func assertApplyStatus(t *testing.T, result *ApplyResult, module string, status Status) {
	t.Helper()
	for _, item := range result.Items {
		if item.Module == module {
			if item.Status != status {
				t.Fatalf("expected %s status %s, got %s", module, status, item.Status)
			}
			return
		}
	}
	t.Fatalf("missing module %s", module)
}
