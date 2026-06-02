package localupdate

import (
	"context"
	"fmt"
	"strings"

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

	var failures []string
	successes := 0
	for i, item := range result.Items {
		if !item.NeedsUpdate {
			continue
		}
		if err := goOps.Get(ctx, plan.ModuleDir, item.Module, item.LocalVersion); err != nil {
			result.Items[i].Status = StatusApplyFailed
			result.Items[i].NeedsUpdate = false
			result.Items[i].Reason = err.Error()
			result.HasFailures = true
			failures = append(failures, fmt.Sprintf("%s: %v", item.Module, err))
			continue
		}
		result.Items[i].Status = StatusApplied
		result.Items[i].NeedsUpdate = false
		result.Items[i].Reason = "updated to local sibling version"
		result.GoGetCount++
		successes++
	}

	if successes > 0 && opts.Tidy {
		result.TidyRun = true
		if err := goOps.Tidy(ctx, plan.ModuleDir); err != nil {
			result.TidyFailed = true
			result.TidyError = err
			result.HasFailures = true
			failures = append(failures, fmt.Sprintf("go mod tidy: %v", err))
		}
	}

	if result.HasFailures {
		return result, fmt.Errorf("local update failed: %s", strings.Join(failures, "; "))
	}
	return result, nil
}
