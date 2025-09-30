package planner

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goliatone/cascade/internal/manifest"
)

// mockParallelChecker is a mock implementation for parallel testing
type mockParallelChecker struct {
	mu          sync.Mutex
	callCount   int32
	checkDelay  time.Duration
	returnError bool
	results     map[string]bool
}

func (m *mockParallelChecker) NeedsUpdate(
	ctx context.Context,
	dependent manifest.Dependent,
	target Target,
	workspace string,
) (bool, error) {
	atomic.AddInt32(&m.callCount, 1)

	// Simulate work with delay
	if m.checkDelay > 0 {
		select {
		case <-time.After(m.checkDelay):
		case <-ctx.Done():
			return true, ctx.Err()
		}
	}

	if m.returnError {
		return true, errors.New("mock error")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.results != nil {
		if needsUpdate, exists := m.results[dependent.Repo]; exists {
			return needsUpdate, nil
		}
	}

	return false, nil
}

// trackedConcurrentChecker tracks concurrent executions for testing
type trackedConcurrentChecker struct {
	currentConcurrent *int32
	maxConcurrent     *int32
	mu                *sync.Mutex
	checkDelay        time.Duration
}

func (t *trackedConcurrentChecker) NeedsUpdate(ctx context.Context, dependent manifest.Dependent, target Target, workspace string) (bool, error) {
	current := atomic.AddInt32(t.currentConcurrent, 1)
	defer atomic.AddInt32(t.currentConcurrent, -1)

	t.mu.Lock()
	if current > *t.maxConcurrent {
		*t.maxConcurrent = current
	}
	t.mu.Unlock()

	if t.checkDelay > 0 {
		select {
		case <-time.After(t.checkDelay):
		case <-ctx.Done():
			return true, ctx.Err()
		}
	}

	return false, nil
}

func TestParallelDependencyChecker_NeedsUpdate(t *testing.T) {
	logger := &mockLogger{}
	mockChecker := &mockParallelChecker{
		results: map[string]bool{
			"repo1": true,
			"repo2": false,
		},
	}

	parallel := NewParallelDependencyChecker(mockChecker, 2, logger)

	ctx := context.Background()
	dependent := manifest.Dependent{Repo: "repo1"}
	target := Target{Module: "example.com/module", Version: "v1.0.0"}

	needsUpdate, err := parallel.NeedsUpdate(ctx, dependent, target, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !needsUpdate {
		t.Errorf("expected needsUpdate=true, got false")
	}
	if mockChecker.callCount != 1 {
		t.Errorf("expected 1 call, got %d", mockChecker.callCount)
	}
}

func TestParallelDependencyChecker_CheckMany(t *testing.T) {
	logger := &mockLogger{}
	mockChecker := &mockParallelChecker{
		results: map[string]bool{
			"repo1": true,
			"repo2": false,
			"repo3": true,
		},
	}

	parallel := NewParallelDependencyChecker(mockChecker, 2, logger).(*parallelDependencyChecker)

	ctx := context.Background()
	dependents := []manifest.Dependent{
		{Repo: "repo1"},
		{Repo: "repo2"},
		{Repo: "repo3"},
	}
	target := Target{Module: "example.com/module", Version: "v1.0.0"}

	results, err := parallel.CheckMany(ctx, dependents, target, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Verify results
	if result, exists := results["repo1"]; !exists || !result.needsUpdate {
		t.Errorf("repo1: expected needsUpdate=true")
	}
	if result, exists := results["repo2"]; !exists || result.needsUpdate {
		t.Errorf("repo2: expected needsUpdate=false")
	}
	if result, exists := results["repo3"]; !exists || !result.needsUpdate {
		t.Errorf("repo3: expected needsUpdate=true")
	}

	// Verify all checks were called
	if mockChecker.callCount != 3 {
		t.Errorf("expected 3 calls, got %d", mockChecker.callCount)
	}
}

func TestParallelDependencyChecker_Parallelism(t *testing.T) {
	logger := &mockLogger{}

	// Create a checker with a 100ms delay per check
	mockChecker := &mockParallelChecker{
		checkDelay: 100 * time.Millisecond,
	}

	// Create 10 dependents - would take 1s sequentially, but much less in parallel
	dependents := make([]manifest.Dependent, 10)
	for i := 0; i < 10; i++ {
		dependents[i] = manifest.Dependent{Repo: "repo" + string(rune('0'+i))}
	}

	// Use 5 parallel workers
	parallel := NewParallelDependencyChecker(mockChecker, 5, logger).(*parallelDependencyChecker)

	ctx := context.Background()
	target := Target{Module: "example.com/module", Version: "v1.0.0"}

	start := time.Now()
	results, err := parallel.CheckMany(ctx, dependents, target, "")
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 10 {
		t.Fatalf("expected 10 results, got %d", len(results))
	}

	// With 5 workers and 100ms per task, 10 tasks should take ~200ms (2 batches)
	// Allow some overhead, but it should be much less than 1s
	if duration > 500*time.Millisecond {
		t.Errorf("parallel checking too slow: took %v, expected <500ms", duration)
	}

	t.Logf("Parallel check of 10 repos with 5 workers took %v", duration)
}

func TestParallelDependencyChecker_ConcurrencyLimit(t *testing.T) {
	logger := &mockLogger{}

	var currentConcurrent int32
	var maxConcurrent int32
	var mu sync.Mutex

	// Create a custom checker that tracks concurrent executions
	trackedChecker := &trackedConcurrentChecker{
		currentConcurrent: &currentConcurrent,
		maxConcurrent:     &maxConcurrent,
		mu:                &mu,
		checkDelay:        50 * time.Millisecond,
	}

	// Create 20 dependents with concurrency limit of 3
	dependents := make([]manifest.Dependent, 20)
	for i := 0; i < 20; i++ {
		dependents[i] = manifest.Dependent{Repo: "repo" + string(rune('0'+i))}
	}

	parallel := NewParallelDependencyChecker(trackedChecker, 3, logger).(*parallelDependencyChecker)

	ctx := context.Background()
	target := Target{Module: "example.com/module", Version: "v1.0.0"}

	_, err := parallel.CheckMany(ctx, dependents, target, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	max := maxConcurrent
	mu.Unlock()

	if max > 3 {
		t.Errorf("concurrency limit violated: max concurrent=%d, expected <=3", max)
	}

	t.Logf("Max concurrent checks: %d (limit: 3)", max)
}

func TestParallelDependencyChecker_ContextCancellation(t *testing.T) {
	logger := &mockLogger{}

	mockChecker := &mockParallelChecker{
		checkDelay: 1 * time.Second, // Long delay
	}

	dependents := make([]manifest.Dependent, 5)
	for i := 0; i < 5; i++ {
		dependents[i] = manifest.Dependent{Repo: "repo" + string(rune('0'+i))}
	}

	parallel := NewParallelDependencyChecker(mockChecker, 2, logger).(*parallelDependencyChecker)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	target := Target{Module: "example.com/module", Version: "v1.0.0"}

	start := time.Now()
	results, err := parallel.CheckMany(ctx, dependents, target, "")
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should complete quickly due to cancellation
	if duration > 500*time.Millisecond {
		t.Errorf("cancellation took too long: %v", duration)
	}

	// Check that we got results for all dependents
	if len(results) != 5 {
		t.Errorf("expected 5 results, got %d", len(results))
	}

	// Results should have context errors
	for repo, result := range results {
		if result.err == nil || !errors.Is(result.err, context.DeadlineExceeded) {
			t.Errorf("repo %s: expected context deadline error, got %v", repo, result.err)
		}
		// Should fail-open on cancellation
		if !result.needsUpdate {
			t.Errorf("repo %s: expected needsUpdate=true on cancellation", repo)
		}
	}

	t.Logf("Cancelled check completed in %v", duration)
}

func TestParallelDependencyChecker_ErrorHandling(t *testing.T) {
	logger := &mockLogger{}

	mockChecker := &mockParallelChecker{
		returnError: true,
	}

	dependents := []manifest.Dependent{
		{Repo: "repo1"},
		{Repo: "repo2"},
	}

	parallel := NewParallelDependencyChecker(mockChecker, 2, logger).(*parallelDependencyChecker)

	ctx := context.Background()
	target := Target{Module: "example.com/module", Version: "v1.0.0"}

	results, err := parallel.CheckMany(ctx, dependents, target, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All results should have errors
	for repo, result := range results {
		if result.err == nil {
			t.Errorf("repo %s: expected error, got nil", repo)
		}
		// Should fail-open on error
		if !result.needsUpdate {
			t.Errorf("repo %s: expected needsUpdate=true on error (fail-open)", repo)
		}
	}
}

func TestParallelDependencyChecker_DefaultConcurrency(t *testing.T) {
	logger := &mockLogger{}
	mockChecker := &mockParallelChecker{}

	// Test with concurrency <= 0 (should default to runtime.NumCPU())
	parallel := NewParallelDependencyChecker(mockChecker, 0, logger).(*parallelDependencyChecker)

	if parallel.concurrency != runtime.NumCPU() {
		t.Errorf("expected concurrency=%d (NumCPU), got %d", runtime.NumCPU(), parallel.concurrency)
	}

	parallel = NewParallelDependencyChecker(mockChecker, -1, logger).(*parallelDependencyChecker)

	if parallel.concurrency != runtime.NumCPU() {
		t.Errorf("expected concurrency=%d (NumCPU), got %d", runtime.NumCPU(), parallel.concurrency)
	}
}

// Benchmark parallel vs sequential checking
func BenchmarkParallelChecker_Sequential(b *testing.B) {
	logger := &mockLogger{}
	mockChecker := &mockParallelChecker{
		checkDelay: 10 * time.Millisecond,
	}

	dependents := make([]manifest.Dependent, 10)
	for i := 0; i < 10; i++ {
		dependents[i] = manifest.Dependent{Repo: "repo" + string(rune('0'+i))}
	}

	parallel := NewParallelDependencyChecker(mockChecker, 1, logger).(*parallelDependencyChecker) // Sequential
	target := Target{Module: "example.com/module", Version: "v1.0.0"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parallel.CheckMany(context.Background(), dependents, target, "")
	}
}

func BenchmarkParallelChecker_Parallel(b *testing.B) {
	logger := &mockLogger{}
	mockChecker := &mockParallelChecker{
		checkDelay: 10 * time.Millisecond,
	}

	dependents := make([]manifest.Dependent, 10)
	for i := 0; i < 10; i++ {
		dependents[i] = manifest.Dependent{Repo: "repo" + string(rune('0'+i))}
	}

	parallel := NewParallelDependencyChecker(mockChecker, 4, logger).(*parallelDependencyChecker) // Parallel
	target := Target{Module: "example.com/module", Version: "v1.0.0"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parallel.CheckMany(context.Background(), dependents, target, "")
	}
}
