package planner

import (
	"context"
	"runtime"
	"sync"

	"github.com/goliatone/cascade/internal/manifest"
)

// parallelDependencyChecker wraps a DependencyChecker to perform checks in parallel
type parallelDependencyChecker struct {
	checker     DependencyChecker
	concurrency int
	logger      Logger
}

// NewParallelDependencyChecker creates a checker that performs dependency checks in parallel
func NewParallelDependencyChecker(
	checker DependencyChecker,
	concurrency int,
	logger Logger,
) DependencyChecker {
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}
	return &parallelDependencyChecker{
		checker:     checker,
		concurrency: concurrency,
		logger:      logger,
	}
}

// checkTask represents a single dependency check task
type checkTask struct {
	dependent manifest.Dependent
	target    Target
	workspace string
}

// checkResult represents the result of a dependency check
type checkResult struct {
	dependent   manifest.Dependent
	needsUpdate bool
	err         error
}

// NeedsUpdate delegates to the wrapped checker (single check, not parallel)
func (p *parallelDependencyChecker) NeedsUpdate(
	ctx context.Context,
	dependent manifest.Dependent,
	target Target,
	workspace string,
) (bool, error) {
	return p.checker.NeedsUpdate(ctx, dependent, target, workspace)
}

// CheckMany performs dependency checks in parallel for multiple dependents
func (p *parallelDependencyChecker) CheckMany(
	ctx context.Context,
	dependents []manifest.Dependent,
	target Target,
	workspace string,
) (map[string]checkResult, error) {
	results := make(map[string]checkResult)
	var mu sync.Mutex

	tasks := make([]checkTask, len(dependents))
	for i, dep := range dependents {
		tasks[i] = checkTask{
			dependent: dep,
			target:    target,
			workspace: workspace,
		}
	}

	// Process tasks in parallel with controlled concurrency
	var wg sync.WaitGroup
	sem := make(chan struct{}, p.concurrency)

	for _, task := range tasks {
		wg.Add(1)
		go func(t checkTask) {
			defer wg.Done()
			sem <- struct{}{}        // Acquire semaphore
			defer func() { <-sem }() // Release semaphore

			// Check for context cancellation
			select {
			case <-ctx.Done():
				mu.Lock()
				results[t.dependent.Repo] = checkResult{
					dependent:   t.dependent,
					needsUpdate: true, // Fail-open on cancellation
					err:         ctx.Err(),
				}
				mu.Unlock()
				return
			default:
			}

			needsUpdate, err := p.checker.NeedsUpdate(
				ctx,
				t.dependent,
				t.target,
				t.workspace,
			)

			mu.Lock()
			results[t.dependent.Repo] = checkResult{
				dependent:   t.dependent,
				needsUpdate: needsUpdate,
				err:         err,
			}
			mu.Unlock()
		}(task)
	}

	wg.Wait()
	return results, nil
}
