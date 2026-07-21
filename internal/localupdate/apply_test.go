package localupdate

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/goliatone/cascade/internal/executor"
)

type mockGoOps struct {
	getCalls  []getCall
	tidyCalls []string
	getErrors map[string]error
	tidyError error
}

type mockBatchGoOps struct {
	mockGoOps
	batchCalls [][]executor.ModuleVersion
	batchError error
}

type blockingGoOps struct {
	mockGoOps
}

func (m *blockingGoOps) Get(ctx context.Context, repoPath, module, version string) error {
	m.getCalls = append(m.getCalls, getCall{repoPath: repoPath, module: module, version: version})
	<-ctx.Done()
	return ctx.Err()
}

type blockingBatchGoOps struct {
	mockGoOps
	batchCalls int
}

func (m *blockingBatchGoOps) GetBatch(ctx context.Context, _ string, _ []executor.ModuleVersion) error {
	m.batchCalls++
	<-ctx.Done()
	return ctx.Err()
}

type blockingTidyGoOps struct {
	mockGoOps
}

func (m *blockingTidyGoOps) Tidy(ctx context.Context, repoPath string) error {
	m.tidyCalls = append(m.tidyCalls, repoPath)
	<-ctx.Done()
	return ctx.Err()
}

func (m *mockBatchGoOps) GetBatch(_ context.Context, _ string, targets []executor.ModuleVersion) error {
	m.batchCalls = append(m.batchCalls, append([]executor.ModuleVersion(nil), targets...))
	return m.batchError
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

func TestApplyPlanBatchesMultipleUpdates(t *testing.T) {
	ops := &mockBatchGoOps{}
	var events []ApplyEvent

	result, err := ApplyPlan(context.Background(), applyTestPlan(), ops, ApplyOptions{
		Tidy:   true,
		Notify: func(event ApplyEvent) { events = append(events, event) },
	})
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if len(ops.batchCalls) != 1 {
		t.Fatalf("expected one batch call, got %#v", ops.batchCalls)
	}
	if len(ops.batchCalls[0]) != 2 {
		t.Fatalf("expected two batch targets, got %#v", ops.batchCalls[0])
	}
	if len(ops.getCalls) != 0 {
		t.Fatalf("expected no individual get calls, got %#v", ops.getCalls)
	}
	if result.GoCommandCount != 1 || result.GoGetCount != 2 {
		t.Fatalf("unexpected command/update counts: %#v", result)
	}
	if len(events) != 4 || events[0].Kind != ApplyBatchStarted || events[1].Kind != ApplyBatchFinished || events[2].Kind != ApplyTidyStarted || events[3].Kind != ApplyTidyFinished {
		t.Fatalf("unexpected apply events: %#v", events)
	}
}

func TestApplyPlanFallsBackAfterBatchFailure(t *testing.T) {
	ops := &mockBatchGoOps{batchError: errors.New("batch failed")}
	ops.getErrors = map[string]error{"github.com/goliatone/a": errors.New("get a failed")}
	var events []ApplyEvent

	result, err := ApplyPlan(context.Background(), applyTestPlan(), ops, ApplyOptions{
		Tidy:   true,
		Notify: func(event ApplyEvent) { events = append(events, event) },
	})
	if err == nil {
		t.Fatal("expected individual fallback failure")
	}
	if len(ops.batchCalls) != 1 || len(ops.getCalls) != 2 {
		t.Fatalf("expected one batch and two fallback calls, got batches=%#v gets=%#v", ops.batchCalls, ops.getCalls)
	}
	if result.GoCommandCount != 3 || result.GoGetCount != 1 {
		t.Fatalf("unexpected command/update counts: %#v", result)
	}
	assertApplyStatus(t, result, "github.com/goliatone/a", StatusApplyFailed)
	assertApplyStatus(t, result, "github.com/goliatone/b", StatusApplied)
	if len(ops.tidyCalls) != 1 {
		t.Fatalf("expected tidy after fallback success, got %#v", ops.tidyCalls)
	}
	foundFallback := false
	for _, event := range events {
		if event.Kind == ApplyBatchFallback {
			foundFallback = true
		}
	}
	if !foundFallback {
		t.Fatalf("expected fallback event, got %#v", events)
	}
}

func TestApplyPlanIgnoresRecoveredBatchFailure(t *testing.T) {
	ops := &mockBatchGoOps{batchError: errors.New("batch failed")}

	result, err := ApplyPlan(context.Background(), applyTestPlan(), ops, ApplyOptions{Tidy: true})
	if err != nil {
		t.Fatalf("expected successful fallback to recover batch failure, got %v", err)
	}
	if result.HasFailures {
		t.Fatalf("expected recovered batch failure not to mark result failed: %#v", result)
	}
}

func TestApplyPlanBatchTimeoutDoesNotFallBackOrRunTidy(t *testing.T) {
	ops := &blockingBatchGoOps{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	result, err := ApplyPlan(ctx, applyTestPlan(), ops, ApplyOptions{Tidy: true})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got %v", err)
	}
	if ops.batchCalls != 1 {
		t.Fatalf("expected one batch attempt, got %d", ops.batchCalls)
	}
	if len(ops.getCalls) != 0 {
		t.Fatalf("expected no sequential fallback after timeout, got %#v", ops.getCalls)
	}
	if len(ops.tidyCalls) != 0 || result.TidyRun {
		t.Fatalf("expected no tidy after timeout, got %#v", result)
	}
	if !errors.Is(result.Interruption, context.DeadlineExceeded) {
		t.Fatalf("expected result interruption, got %#v", result.Interruption)
	}
	assertApplyStatus(t, result, "github.com/goliatone/a", StatusApplyFailed)
	assertApplyStatus(t, result, "github.com/goliatone/b", StatusApplyFailed)
}

func TestApplyPlanSequentialTimeoutStopsRemainingUpdatesAndTidy(t *testing.T) {
	ops := &blockingGoOps{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	result, err := ApplyPlan(ctx, applyTestPlan(), ops, ApplyOptions{Tidy: true})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got %v", err)
	}
	if len(ops.getCalls) != 1 {
		t.Fatalf("expected timeout to stop later updates, got %#v", ops.getCalls)
	}
	if len(ops.tidyCalls) != 0 || result.TidyRun {
		t.Fatalf("expected no tidy after timeout, got %#v", result)
	}
	assertApplyStatus(t, result, "github.com/goliatone/a", StatusApplyFailed)
	assertApplyStatus(t, result, "github.com/goliatone/b", StatusApplyFailed)
}

func TestApplyPlanTidyTimeoutIsReportedAsInterruption(t *testing.T) {
	ops := &blockingTidyGoOps{}
	plan := applyTestPlan()
	plan.Items = plan.Items[:1]
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	result, err := ApplyPlan(ctx, plan, ops, ApplyOptions{Tidy: true})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got %v", err)
	}
	if len(ops.getCalls) != 1 || len(ops.tidyCalls) != 1 {
		t.Fatalf("expected one get and one tidy attempt, got gets=%#v tidy=%#v", ops.getCalls, ops.tidyCalls)
	}
	if !result.TidyRun || !result.TidyFailed || !errors.Is(result.Interruption, context.DeadlineExceeded) {
		t.Fatalf("expected interrupted tidy result, got %#v", result)
	}
	if result.TidyError == nil || !strings.Contains(result.TidyError.Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("expected tidy deadline detail, got %v", result.TidyError)
	}
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
