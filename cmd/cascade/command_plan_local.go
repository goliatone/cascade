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

	"github.com/goliatone/cascade/internal/cliui"
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
	progress := newLocalProgress(cmd)
	planning := progress.Start("Planning local dependency updates")
	req, err := buildLocalRequest(cmd, opts)
	if err != nil {
		planning.Fail("Could not prepare local dependency plan")
		return err
	}
	plan, err := localupdate.PlanLocal(req)
	if err != nil {
		planning.Fail("Could not resolve local dependencies")
		return newPlanningError("failed to plan local dependency updates", err)
	}
	planning.Success(localPlanProgressMessage(plan))
	renderLocalPlan(cmd.OutOrStdout(), plan, "Local dependency plan")
	return nil
}

func runUpdateLocal(cmd *cobra.Command, opts localCommandOptions) error {
	progress := newLocalProgress(cmd)
	planning := progress.Start("Planning local dependency updates")
	req, err := buildLocalRequest(cmd, opts)
	if err != nil {
		planning.Fail("Could not prepare local dependency plan")
		return err
	}
	plan, err := localupdate.PlanLocal(req)
	if err != nil {
		planning.Fail("Could not resolve local dependencies")
		return newPlanningError("failed to plan local dependency updates", err)
	}
	planning.Success(localPlanProgressMessage(plan))

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
	goOps := executor.NewGoOperations()
	if localCfg != nil && localCfg.Logging.Verbose {
		goOps = executor.NewGoOperationsWithOutput(cmd.ErrOrStderr(), cmd.ErrOrStderr())
	}
	applyProgress := newLocalApplyProgress(progress)
	result, applyErr := localupdate.ApplyPlan(ctx, plan, goOps, localupdate.ApplyOptions{
		Tidy:   !opts.NoTidy,
		Notify: applyProgress.Notify,
	})
	renderLocalApplyResult(cmd.OutOrStdout(), result)
	var hookErr error
	if !opts.NoHooks && !hookPlan.Empty() {
		phases := hookPlan.SelectedPhases(applyErr == nil)
		hookCount := countLocalHooks(phases)
		hookTask := progress.Start(fmt.Sprintf("Running %d local update %s", hookCount, pluralize(hookCount, "hook", "hooks")))
		hookResults, err := hooks.NewRunner(executorTimeout).Run(ctx, phases, localHookContext(plan, result, applyErr))
		if err != nil {
			hookTask.Fail("Local update hooks failed")
		} else {
			hookTask.Success(fmt.Sprintf("Completed %d local update %s", hookCount, pluralize(hookCount, "hook", "hooks")))
		}
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

func newLocalProgress(cmd *cobra.Command) *cliui.Progress {
	options := cliui.Options{}
	if localCfg := localCommandConfig(); localCfg != nil {
		options.Quiet = localCfg.Logging.Quiet
		options.Verbose = localCfg.Logging.Verbose
	}
	return cliui.NewProgress(cmd.ErrOrStderr(), options)
}

func localPlanProgressMessage(plan localupdate.Plan) string {
	updates := len(plan.Updates())
	return fmt.Sprintf("Planned %d %s across %d candidates", updates, pluralize(updates, "update", "updates"), len(plan.Items))
}

func pluralize(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

func countLocalHooks(phases []hooks.PhaseHooks) int {
	count := 0
	for _, phase := range phases {
		count += len(phase.Hooks)
	}
	return count
}

type localApplyProgress struct {
	progress *cliui.Progress
	current  *cliui.Task
}

func newLocalApplyProgress(progress *cliui.Progress) *localApplyProgress {
	return &localApplyProgress{progress: progress}
}

func (p *localApplyProgress) Notify(event localupdate.ApplyEvent) {
	switch event.Kind {
	case localupdate.ApplyBatchStarted:
		p.current = p.progress.Start(fmt.Sprintf("Updating %d dependencies with one go get", event.Total))
	case localupdate.ApplyBatchFinished:
		if event.Err != nil {
			p.current.Warn("Combined dependency update did not complete")
		} else {
			p.current.Success(fmt.Sprintf("Updated %d dependencies", event.Total))
		}
		p.current = nil
	case localupdate.ApplyBatchFallback:
		p.progress.Warn(fmt.Sprintf("Retrying %d dependencies individually", event.Total))
	case localupdate.ApplyItemStarted:
		p.current = p.progress.Start(fmt.Sprintf("Updating %s to %s (%d/%d)", event.Item.Module, event.Item.LocalVersion, event.Index, event.Total))
	case localupdate.ApplyItemFinished:
		if event.Err != nil {
			p.current.Fail(fmt.Sprintf("Failed to update %s", event.Item.Module))
		} else {
			p.current.Success(fmt.Sprintf("Updated %s to %s", event.Item.Module, event.Item.LocalVersion))
		}
		p.current = nil
	case localupdate.ApplyTidyStarted:
		p.current = p.progress.Start("Running go mod tidy")
	case localupdate.ApplyTidyFinished:
		if event.Err != nil {
			p.current.Fail("go mod tidy failed")
		} else {
			p.current.Success("go mod tidy completed")
		}
		p.current = nil
	}
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
	styler := cliui.NewStyler(out)
	fmt.Fprintf(out, "  - %s %s -> %s [%s]", item.Module, current, local, styledLocalStatus(styler, item.Status))
	if strings.TrimSpace(item.Reason) != "" {
		fmt.Fprintf(out, " - %s", item.Reason)
	}
	fmt.Fprintln(out)
}

func renderLocalSummary(out io.Writer, items []localupdate.Item) {
	styler := cliui.NewStyler(out)
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
		localupdate.StatusInvalidLocalModule,
		localupdate.StatusAmbiguousLocalModule,
		localupdate.StatusComparisonFailed,
		localupdate.StatusApplyFailed,
	} {
		if count := counts[status]; count > 0 {
			fmt.Fprintf(out, "  %s: %d\n", styledLocalStatus(styler, status), count)
		}
	}
}

func styledLocalStatus(styler cliui.Styler, status localupdate.Status) string {
	value := string(status)
	switch status {
	case localupdate.StatusApplied, localupdate.StatusCurrent:
		return styler.Success(value)
	case localupdate.StatusUpdate:
		return styler.Info(value)
	case localupdate.StatusApplyFailed, localupdate.StatusComparisonFailed, localupdate.StatusInvalidVersion, localupdate.StatusInvalidLocalModule, localupdate.StatusAmbiguousLocalModule:
		return styler.Error(value)
	case localupdate.StatusSkippedIndirect, localupdate.StatusSkippedFilter, localupdate.StatusSkippedReplace, localupdate.StatusMissingLocalRepo, localupdate.StatusMissingVersionFile:
		return styler.Warning(value)
	default:
		return styler.Muted(value)
	}
}

func valueOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
