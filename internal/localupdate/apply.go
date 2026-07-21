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
	if err := ctx.Err(); err != nil && len(updateIndexes) > 0 {
		markInterrupted(result, updateIndexes, err)
		return result, interruptedApplyError(err, result.Items)
	}
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
			if ctxErr := ctx.Err(); ctxErr != nil {
				err = ctxErr
			}
			notify(opts, ApplyEvent{Kind: ApplyBatchFinished, Total: len(targets), Err: err, Duration: time.Since(started)})
			if err == nil {
				for _, index := range updateIndexes {
					markApplied(result, index)
				}
				successes = len(updateIndexes)
				updateIndexes = nil
			} else if ctx.Err() != nil {
				markInterrupted(result, updateIndexes, ctx.Err())
				updateIndexes = nil
			} else {
				notify(opts, ApplyEvent{Kind: ApplyBatchFallback, Total: len(targets), Err: err})
			}
		}
	}

	for position, index := range updateIndexes {
		if err := ctx.Err(); err != nil {
			markInterrupted(result, updateIndexes[position:], err)
			break
		}
		item := result.Items[index]
		notify(opts, ApplyEvent{Kind: ApplyItemStarted, Item: item, Index: position + 1, Total: len(updateIndexes)})
		started := time.Now()
		err := goOps.Get(ctx, plan.ModuleDir, item.Module, item.LocalVersion)
		result.GoCommandCount++
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
		}
		notify(opts, ApplyEvent{Kind: ApplyItemFinished, Item: item, Index: position + 1, Total: len(updateIndexes), Err: err, Duration: time.Since(started)})
		if err != nil {
			markApplyFailed(result, index, err)
			failures = append(failures, fmt.Sprintf("%s: %v", item.Module, err))
			if ctx.Err() != nil {
				remaining := updateIndexes[position+1:]
				markInterrupted(result, remaining, ctx.Err())
				result.Interruption = ctx.Err()
				break
			}
			continue
		}
		markApplied(result, index)
		successes++
	}

	if successes > 0 && opts.Tidy && ctx.Err() != nil {
		result.Interruption = ctx.Err()
		result.HasFailures = true
	}

	if successes > 0 && opts.Tidy && result.Interruption == nil {
		result.TidyRun = true
		notify(opts, ApplyEvent{Kind: ApplyTidyStarted})
		started := time.Now()
		err := goOps.Tidy(ctx, plan.ModuleDir)
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
		}
		if err != nil {
			notify(opts, ApplyEvent{Kind: ApplyTidyFinished, Err: err, Duration: time.Since(started)})
			result.TidyFailed = true
			result.TidyError = err
			result.HasFailures = true
			failures = append(failures, fmt.Sprintf("go mod tidy: %v", err))
			if ctx.Err() != nil {
				result.Interruption = ctx.Err()
			}
		} else {
			notify(opts, ApplyEvent{Kind: ApplyTidyFinished, Duration: time.Since(started)})
		}
	}

	if result.Interruption != nil {
		return result, interruptedApplyError(result.Interruption, result.Items)
	}
	if result.HasFailures {
		return result, fmt.Errorf("local update failed: %s", strings.Join(failures, "; "))
	}
	return result, nil
}

func markInterrupted(result *ApplyResult, indexes []int, err error) {
	result.Interruption = err
	result.HasFailures = true
	for _, index := range indexes {
		markApplyFailed(result, index, err)
	}
}

func interruptedApplyError(err error, items []Item) error {
	var failures []string
	for _, item := range items {
		if item.Status == StatusApplyFailed {
			failures = append(failures, fmt.Sprintf("%s: %s", item.Module, item.Reason))
		}
	}
	if len(failures) == 0 {
		return fmt.Errorf("local update interrupted: %w", err)
	}
	return fmt.Errorf("local update interrupted: %s: %w", strings.Join(failures, "; "), err)
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
