package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/hooks"
	"github.com/goliatone/cascade/internal/localupdate"
	"github.com/goliatone/cascade/pkg/config"
	"github.com/spf13/cobra"
)

type localCommandOptions struct {
	Prefixes        []string
	IncludeIndirect bool
	Only            []string
	Exclude         []string
	NoTidy          bool
	NoHooks         bool
}

func newPlanLocalCommand() *cobra.Command {
	opts := localCommandOptions{}
	cmd := &cobra.Command{
		Use:   "local",
		Short: "Preview local sibling dependency updates",
		Long: `Preview local Go dependency updates by comparing the current module's
direct dependencies against sibling repositories in the local workspace.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPlanLocal(cmd, opts)
		},
	}
	addLocalPlanFlags(cmd, &opts)
	return cmd
}

func runPlanLocal(cmd *cobra.Command, opts localCommandOptions) error {
	req, err := buildLocalRequest(cmd, opts)
	if err != nil {
		return err
	}
	plan, err := localupdate.PlanLocal(req)
	if err != nil {
		return newPlanningError("failed to plan local dependency updates", err)
	}
	renderLocalPlan(cmd.OutOrStdout(), plan, "Local dependency plan")
	return nil
}

func runUpdateLocal(cmd *cobra.Command, opts localCommandOptions) error {
	req, err := buildLocalRequest(cmd, opts)
	if err != nil {
		return err
	}
	plan, err := localupdate.PlanLocal(req)
	if err != nil {
		return newPlanningError("failed to plan local dependency updates", err)
	}

	localCfg := localCommandConfig()
	dryRun := false
	hookPlan := hooks.LocalUpdatePlan{}
	executorTimeout := time.Duration(0)
	if localCfg != nil {
		dryRun = localCfg.Executor.DryRun
		executorTimeout = localCfg.Executor.Timeout
		hookPlan = hooks.ResolveLocalUpdatePlan(localCfg.Hooks.Update.Local, localHookContext(plan, nil, nil), localCfg.ConfigLayers())
	}
	if dryRun {
		renderLocalPlan(cmd.OutOrStdout(), plan, "DRY RUN: Local dependency update plan")
		if !hookPlan.Empty() && !opts.NoHooks {
			renderLocalDryRunHookPlan(cmd.OutOrStdout(), hookPlan)
		}
		return nil
	}

	ctx := commandContext(cmd)
	result, applyErr := localupdate.ApplyPlan(ctx, plan, executor.NewGoOperations(), localupdate.ApplyOptions{
		Tidy: !opts.NoTidy,
	})
	renderLocalApplyResult(cmd.OutOrStdout(), result)
	var hookErr error
	if !opts.NoHooks && !hookPlan.Empty() {
		hookResults, err := hooks.NewRunner(executorTimeout).Run(ctx, hookPlan.SelectedPhases(applyErr == nil), localHookContext(plan, result, applyErr))
		renderLocalHookResults(cmd.OutOrStdout(), hookResults)
		hookErr = err
	}
	if applyErr != nil {
		if hookErr != nil {
			return newExecutionError("failed to apply local dependency updates and run local update hooks", errors.Join(applyErr, hookErr))
		}
		return newExecutionError("failed to apply local dependency updates", applyErr)
	}
	if hookErr != nil {
		return newExecutionError("failed to run local update hooks", hookErr)
	}
	return nil
}

func commandContext(cmd *cobra.Command) context.Context {
	if cmd == nil || cmd.Context() == nil {
		return context.Background()
	}
	return cmd.Context()
}

func localHookContext(plan localupdate.Plan, result *localupdate.ApplyResult, applyErr error) hooks.Context {
	hookCtx := hooks.Context{
		Command:      "update local",
		Module:       plan.CurrentModule,
		ModuleDir:    plan.ModuleDir,
		Workspace:    plan.Workspace,
		UpdateStatus: "success",
	}
	if applyErr != nil {
		hookCtx.UpdateStatus = "failure"
	}
	if result != nil {
		hookCtx.UpdatedCount = result.GoGetCount
		hookCtx.TidyRan = result.TidyRun
		hookCtx.TidyFailed = result.TidyFailed
	}
	return hookCtx
}

func buildLocalRequest(cmd *cobra.Command, opts localCommandOptions) (localupdate.Request, error) {
	modulePath, moduleDir, err := detectModuleInfo()
	if err != nil {
		return localupdate.Request{}, newValidationError("go.mod must be present in the current directory tree", err)
	}

	workspacePath, workspaceExplicit := localWorkspace(cmd)
	return localupdate.Request{
		CurrentModule:     modulePath,
		ModuleDir:         moduleDir,
		Workspace:         workspacePath,
		WorkspaceExplicit: workspaceExplicit,
		Prefixes:          opts.Prefixes,
		IncludeIndirect:   opts.IncludeIndirect,
		Only:              opts.Only,
		Exclude:           opts.Exclude,
		Tidy:              !opts.NoTidy,
	}, nil
}

func localWorkspace(cmd *cobra.Command) (string, bool) {
	if flagChanged(cmd, "workspace") {
		if value, err := cmd.Flags().GetString("workspace"); err == nil && strings.TrimSpace(value) != "" {
			return value, true
		}
	}
	if container != nil && container.Config() != nil {
		if path, explicit := configuredLocalWorkspace(container.Config()); explicit {
			return path, true
		}
	}
	if localCfg := localCommandConfig(); localCfg != nil {
		if path, explicit := configuredLocalWorkspace(localCfg); explicit {
			return path, true
		}
	}
	return "", false
}

func configuredLocalWorkspace(cfg *config.Config) (string, bool) {
	if cfg == nil {
		return "", false
	}
	path := strings.TrimSpace(cfg.Workspace.Path)
	if path == "" {
		return "", false
	}
	if cfg.ExplicitlySetWorkspacePath() {
		return path, true
	}
	if !isDefaultLocalCacheWorkspace(path) {
		return path, true
	}
	return "", false
}

func localCommandConfig() *config.Config {
	if container != nil && container.Config() != nil {
		return container.Config()
	}
	return cfg
}

func flagChanged(cmd *cobra.Command, name string) bool {
	for current := cmd; current != nil; current = current.Parent() {
		if flag := current.Flags().Lookup(name); flag != nil && flag.Changed {
			return true
		}
		if flag := current.PersistentFlags().Lookup(name); flag != nil && flag.Changed {
			return true
		}
	}
	return false
}

func isDefaultLocalCacheWorkspace(path string) bool {
	defaults := []string{}
	if xdg := os.Getenv("XDG_CACHE_HOME"); strings.TrimSpace(xdg) != "" {
		defaults = append(defaults, filepath.Join(xdg, "cascade"))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		defaults = append(defaults, filepath.Join(home, ".cache", "cascade"))
	}
	if len(defaults) == 0 {
		defaults = append(defaults, filepath.Join(os.TempDir(), "cascade"))
	}
	cleanPath := filepath.Clean(path)
	for _, defaultPath := range defaults {
		if cleanPath == filepath.Clean(defaultPath) {
			return true
		}
	}
	return false
}

func addLocalPlanFlags(cmd *cobra.Command, opts *localCommandOptions) {
	cmd.Flags().StringSliceVar(&opts.Prefixes, "prefix", []string{}, "Module prefix to include (repeatable or comma-separated; default: github.com/goliatone/)")
	cmd.Flags().BoolVar(&opts.IncludeIndirect, "include-indirect", false, "Include indirect require entries")
	cmd.Flags().StringSliceVar(&opts.Only, "only", []string{}, "Only include these module paths or basenames (repeatable or comma-separated)")
	cmd.Flags().StringSliceVar(&opts.Exclude, "exclude", []string{}, "Exclude these module paths or basenames (repeatable or comma-separated)")
}

func renderLocalPlan(out io.Writer, plan localupdate.Plan, title string) {
	fmt.Fprintf(out, "%s for %s\n", title, plan.CurrentModule)
	fmt.Fprintf(out, "Module dir: %s\n", plan.ModuleDir)
	fmt.Fprintf(out, "Workspace: %s\n\n", plan.Workspace)
	if len(plan.Items) == 0 {
		fmt.Fprintln(out, "No local dependency candidates found.")
		return
	}

	fmt.Fprintln(out, "Candidates:")
	for _, item := range plan.Items {
		renderLocalItem(out, item)
	}
	renderLocalSummary(out, plan.Items)
}

func renderLocalApplyResult(out io.Writer, result *localupdate.ApplyResult) {
	if result == nil {
		fmt.Fprintln(out, "No local update result available.")
		return
	}
	fmt.Fprintf(out, "Local dependency update for %s\n", result.Plan.CurrentModule)
	fmt.Fprintf(out, "Module dir: %s\n", result.Plan.ModuleDir)
	fmt.Fprintf(out, "Workspace: %s\n\n", result.Plan.Workspace)
	if len(result.Items) == 0 {
		fmt.Fprintln(out, "No local dependency candidates found.")
		return
	}

	fmt.Fprintln(out, "Results:")
	for _, item := range result.Items {
		renderLocalItem(out, item)
	}
	if result.TidyRun {
		if result.TidyFailed {
			fmt.Fprintf(out, "\ngo mod tidy: failed - %v\n", result.TidyError)
		} else {
			fmt.Fprintln(out, "\ngo mod tidy: completed")
		}
	}
	renderLocalSummary(out, result.Items)
}

func renderLocalHookResults(out io.Writer, results hooks.Results) {
	if len(results) == 0 {
		return
	}
	fmt.Fprintln(out, "\nHooks:")
	for _, result := range results {
		status := "ok"
		if result.Timeout {
			status = "timeout"
		} else if result.Failed() {
			status = "failed"
		}
		fmt.Fprintf(out, "  - %s %s [%s] (%s)", result.Phase, result.DisplayName(), status, roundDuration(result.Duration))
		if result.Err != nil {
			fmt.Fprintf(out, " - %v", result.Err)
		}
		fmt.Fprintln(out)
		if result.Failed() && strings.TrimSpace(result.Output) != "" {
			fmt.Fprintln(out, "    output:")
			for _, line := range strings.Split(truncateHookOutput(result.Output), "\n") {
				if strings.TrimSpace(line) != "" {
					fmt.Fprintf(out, "      %s\n", line)
				}
			}
		}
	}
}

func renderLocalDryRunHookPlan(out io.Writer, plan hooks.LocalUpdatePlan) {
	fmt.Fprintln(out, "\nLocal update hooks configured; skipping hook execution during dry-run.")
	if len(plan.MatchedRules) == 0 {
		return
	}
	fmt.Fprintln(out, "Matched hook rules:")
	for _, rule := range plan.MatchedRules {
		fmt.Fprintf(out, "  - %s [%s]\n", rule.Name, hookRuleSourceLabel(rule.Source))
	}
}

func hookRuleSourceLabel(source hooks.RuleSource) string {
	scope := string(source.Scope)
	if strings.TrimSpace(scope) == "" {
		scope = "merged"
	}
	if strings.TrimSpace(source.Path) == "" {
		return scope
	}
	return fmt.Sprintf("%s %s", scope, source.Path)
}

func roundDuration(duration time.Duration) time.Duration {
	if duration <= 0 {
		return 0
	}
	return duration.Round(time.Millisecond)
}

func truncateHookOutput(output string) string {
	const maxOutput = 4096
	output = strings.TrimSpace(output)
	if len(output) <= maxOutput {
		return output
	}
	return output[:maxOutput] + "\n... output truncated ..."
}

func renderLocalItem(out io.Writer, item localupdate.Item) {
	current := valueOrDash(item.CurrentVersion)
	local := valueOrDash(item.LocalVersion)
	fmt.Fprintf(out, "  - %s %s -> %s [%s]", item.Module, current, local, item.Status)
	if strings.TrimSpace(item.Reason) != "" {
		fmt.Fprintf(out, " - %s", item.Reason)
	}
	fmt.Fprintln(out)
}

func renderLocalSummary(out io.Writer, items []localupdate.Item) {
	counts := map[localupdate.Status]int{}
	for _, item := range items {
		counts[item.Status]++
	}
	fmt.Fprintln(out, "\nSummary:")
	for _, status := range []localupdate.Status{
		localupdate.StatusUpdate,
		localupdate.StatusApplied,
		localupdate.StatusCurrent,
		localupdate.StatusSkippedIndirect,
		localupdate.StatusSkippedFilter,
		localupdate.StatusSkippedReplace,
		localupdate.StatusMissingLocalRepo,
		localupdate.StatusMissingVersionFile,
		localupdate.StatusInvalidVersion,
		localupdate.StatusComparisonFailed,
		localupdate.StatusApplyFailed,
	} {
		if count := counts[status]; count > 0 {
			fmt.Fprintf(out, "  %s: %d\n", status, count)
		}
	}
}

func valueOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
