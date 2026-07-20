package localupdate

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/goliatone/cascade/internal/executor"
)

func ApplyPlan(ctx context.Context, plan Plan, goOps executor.GoOperations, opts ApplyOptions) (*ApplyResult, error) {
	result := &ApplyResult{
		Plan:  plan,
		Items: append([]Item(nil), plan.Items...),
	}

	if opts.DryRun {
		return result, nil
	}
	if goOps == nil {
		return result, fmt.Errorf("go operations are required")
	}

	updateIndexes := pendingUpdateIndexes(result.Items)
	var failures []string
	successes := 0
	if len(updateIndexes) > 1 {
		if batchOps, ok := goOps.(executor.GoBatchOperations); ok {
			targets := make([]executor.ModuleVersion, 0, len(updateIndexes))
			for _, index := range updateIndexes {
				item := result.Items[index]
				targets = append(targets, executor.ModuleVersion{Module: item.Module, Version: item.LocalVersion})
			}
			notify(opts, ApplyEvent{Kind: ApplyBatchStarted, Total: len(targets)})
			started := time.Now()
			err := batchOps.GetBatch(ctx, plan.ModuleDir, targets)
			result.GoCommandCount++
			notify(opts, ApplyEvent{Kind: ApplyBatchFinished, Total: len(targets), Err: err, Duration: time.Since(started)})
			if err == nil {
				for _, index := range updateIndexes {
					markApplied(result, index)
				}
				successes = len(updateIndexes)
				updateIndexes = nil
			} else if ctx.Err() != nil {
				for _, index := range updateIndexes {
					markApplyFailed(result, index, ctx.Err())
					failures = append(failures, fmt.Sprintf("%s: %v", result.Items[index].Module, ctx.Err()))
				}
				updateIndexes = nil
			} else {
				notify(opts, ApplyEvent{Kind: ApplyBatchFallback, Total: len(targets), Err: err})
			}
		}
	}

	for position, index := range updateIndexes {
		item := result.Items[index]
		notify(opts, ApplyEvent{Kind: ApplyItemStarted, Item: item, Index: position + 1, Total: len(updateIndexes)})
		started := time.Now()
		err := goOps.Get(ctx, plan.ModuleDir, item.Module, item.LocalVersion)
		result.GoCommandCount++
		notify(opts, ApplyEvent{Kind: ApplyItemFinished, Item: item, Index: position + 1, Total: len(updateIndexes), Err: err, Duration: time.Since(started)})
		if err != nil {
			markApplyFailed(result, index, err)
			failures = append(failures, fmt.Sprintf("%s: %v", item.Module, err))
			continue
		}
		markApplied(result, index)
		successes++
	}

	if successes > 0 && opts.Tidy {
		result.TidyRun = true
		notify(opts, ApplyEvent{Kind: ApplyTidyStarted})
		started := time.Now()
		if err := goOps.Tidy(ctx, plan.ModuleDir); err != nil {
			notify(opts, ApplyEvent{Kind: ApplyTidyFinished, Err: err, Duration: time.Since(started)})
			result.TidyFailed = true
			result.TidyError = err
			result.HasFailures = true
			failures = append(failures, fmt.Sprintf("go mod tidy: %v", err))
		} else {
			notify(opts, ApplyEvent{Kind: ApplyTidyFinished, Duration: time.Since(started)})
		}
	}

	if result.HasFailures {
		return result, fmt.Errorf("local update failed: %s", strings.Join(failures, "; "))
	}
	return result, nil
}

func pendingUpdateIndexes(items []Item) []int {
	indexes := make([]int, 0)
	for index, item := range items {
		if item.NeedsUpdate {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

func markApplied(result *ApplyResult, index int) {
	result.Items[index].Status = StatusApplied
	result.Items[index].NeedsUpdate = false
	result.Items[index].Reason = "updated to local sibling version"
	result.GoGetCount++
}

func markApplyFailed(result *ApplyResult, index int, err error) {
	result.Items[index].Status = StatusApplyFailed
	result.Items[index].NeedsUpdate = false
	result.Items[index].Reason = err.Error()
	result.HasFailures = true
}

func notify(opts ApplyOptions, event ApplyEvent) {
	if opts.Notify != nil {
		opts.Notify(event)
	}
}
