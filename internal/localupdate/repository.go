package localupdate

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/goliatone/cascade/internal/executor"
)

func PlanRepository(repository Repository, req Request) (RepositoryPlan, error) {
	result := RepositoryPlan{
		Repository: repository,
		Plans:      make([]Plan, 0, len(repository.Modules)),
	}
	resolvedWorkspace := req.Workspace
	if resolvedWorkspace == "" {
		resolvedWorkspace = repositoryDependencyWorkspace(repository)
	}
	for _, target := range repository.Modules {
		moduleRequest := req
		moduleRequest.CurrentModule = target.ModulePath
		moduleRequest.ModuleDir = target.ModuleDir
		moduleRequest.Workspace = resolvedWorkspace
		plan, err := PlanLocal(moduleRequest)
		if err != nil {
			return RepositoryPlan{}, fmt.Errorf("plan local dependencies for %s: %w", target.ModulePath, err)
		}
		if resolvedWorkspace == "" {
			resolvedWorkspace = plan.Workspace
		}
		result.Plans = append(result.Plans, plan)
	}
	return result, nil
}

func repositoryDependencyWorkspace(repository Repository) string {
	repositoryName := filepath.Base(repository.Root)
	for _, target := range repository.Modules {
		parts := strings.Split(strings.Trim(target.ModulePath, "/"), "/")
		moduleRepository := ""
		if len(parts) >= 3 && isKnownVCSHost(parts[0]) {
			moduleRepository = parts[2]
		} else if len(parts) >= 2 && strings.Contains(parts[0], ".") {
			moduleRepository = parts[1]
		}
		if moduleRepository == repositoryName {
			return filepath.Dir(repository.Root)
		}
	}
	return ""
}

func ApplyRepository(ctx context.Context, plan RepositoryPlan, goOps executor.GoOperations, opts ApplyOptions) (*RepositoryApplyResult, error) {
	result := &RepositoryApplyResult{Plan: plan, Results: make([]*ApplyResult, 0, len(plan.Plans))}
	var failures []error

	for index, modulePlan := range plan.Plans {
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
		if ctx.Err() != nil && index+1 < len(plan.Plans) {
			result.Interruption = ctx.Err()
			result.HasFailures = true
			for _, remaining := range plan.Plans[index+1:] {
				result.Results = append(result.Results, interruptedResult(remaining, ctx.Err()))
			}
			failures = append(failures, ctx.Err())
			break
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
