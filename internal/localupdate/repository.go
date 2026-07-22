package localupdate

import (
	"context"
	"errors"
	"fmt"

	"github.com/goliatone/cascade/internal/executor"
)

func PlanRepository(repository Repository, req Request) (RepositoryPlan, error) {
	result := RepositoryPlan{
		Repository: repository,
		Plans:      make([]Plan, 0, len(repository.Modules)),
	}
	for _, target := range repository.Modules {
		moduleRequest := req
		moduleRequest.CurrentModule = target.ModulePath
		moduleRequest.ModuleDir = target.ModuleDir
		plan, err := PlanLocal(moduleRequest)
		if err != nil {
			return RepositoryPlan{}, fmt.Errorf("plan local dependencies for %s: %w", target.ModulePath, err)
		}
		result.Plans = append(result.Plans, plan)
	}
	return result, nil
}

func ApplyRepository(ctx context.Context, plan RepositoryPlan, goOps executor.GoOperations, opts ApplyOptions) (*RepositoryApplyResult, error) {
	result := &RepositoryApplyResult{Plan: plan, Results: make([]*ApplyResult, 0, len(plan.Plans))}
	var failures []error

	for index, modulePlan := range plan.Plans {
		if err := ctx.Err(); err != nil {
			result.Interruption = err
			result.HasFailures = true
			for _, remaining := range plan.Plans[index:] {
				result.Results = append(result.Results, interruptedResult(remaining, err))
			}
			failures = append(failures, err)
			break
		}

		moduleOptions := opts
		if opts.Notify != nil {
			moduleOptions.Notify = func(event ApplyEvent) {
				event.Module = modulePlan.CurrentModule
				event.ModuleDir = modulePlan.ModuleDir
				opts.Notify(event)
			}
		}
		moduleResult, err := ApplyPlan(ctx, modulePlan, goOps, moduleOptions)
		result.Results = append(result.Results, moduleResult)
		if err != nil {
			result.HasFailures = true
			failures = append(failures, fmt.Errorf("%s: %w", modulePlan.CurrentModule, err))
			if ctx.Err() != nil {
				result.Interruption = ctx.Err()
			}
		}
	}

	if len(failures) > 0 {
		return result, fmt.Errorf("repository local update failed: %w", errors.Join(failures...))
	}
	return result, nil
}

func interruptedResult(plan Plan, err error) *ApplyResult {
	result := &ApplyResult{Plan: plan, Items: append([]Item(nil), plan.Items...), Interruption: err, HasFailures: true}
	markInterrupted(result, pendingUpdateIndexes(result.Items), err)
	return result
}
