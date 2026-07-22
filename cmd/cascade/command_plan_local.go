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
	GitIgnore       bool
	NoTidy          bool
	NoHooks         bool
}

func newPlanLocalCommand() *cobra.Command {
	opts := localCommandOptions{}
	cmd := &cobra.Command{
		Use:   "local",
		Short: "Preview local sibling dependency updates",
		Long: `Preview local Go dependency updates across every module in the current
repository. Cascade uses go.work when present and discovers repository go.mod
files otherwise, then compares their dependencies with sibling repositories.`,
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
	plan, err := buildLocalRepositoryPlan(cmd, opts)
	if err != nil {
		planning.Fail("Could not prepare local dependency plan")
		return err
	}
	planning.Success(localRepositoryPlanProgressMessage(plan))
	renderLocalRepositoryPlan(cmd.OutOrStdout(), plan, "Local dependency plan")
	return nil
}

func runUpdateLocal(cmd *cobra.Command, opts localCommandOptions) error {
	progress := newLocalProgress(cmd)
	planning := progress.Start("Planning local dependency updates")
	plan, err := buildLocalRepositoryPlan(cmd, opts)
	if err != nil {
		planning.Fail("Could not prepare local dependency plan")
		return err
	}
	planning.Success(localRepositoryPlanProgressMessage(plan))

	localCfg := localCommandConfig()
	dryRun := false
	hookPlan := hooks.LocalUpdatePlan{}
	executorTimeout := time.Duration(0)
	if localCfg != nil {
		dryRun = localCfg.Executor.DryRun
		executorTimeout = localCfg.Executor.Timeout
		hookPlan = hooks.ResolveLocalUpdatePlan(localCfg.Hooks.Update.Local, localRepositoryHookContext(plan, nil, nil), localCfg.ConfigLayers())
	}
	if dryRun {
		renderLocalRepositoryPlan(cmd.OutOrStdout(), plan, "DRY RUN: Local dependency update plan")
		if !hookPlan.Empty() && !opts.NoHooks {
			renderLocalDryRunHookPlan(cmd.OutOrStdout(), hookPlan)
		}
		return nil
	}

	ctx := commandContext(cmd)
	applyCtx, cancelApply := localApplyContext(ctx, executorTimeout)
	goOps := executor.NewGoOperations()
	if localCfg != nil && localCfg.Logging.Verbose {
		goOps = executor.NewGoOperationsWithOutput(cmd.ErrOrStderr(), cmd.ErrOrStderr())
	}
	applyProgress := newLocalApplyProgress(progress, len(plan.Plans) > 1)
	result, applyErr := localupdate.ApplyRepository(applyCtx, plan, goOps, localupdate.ApplyOptions{
		Tidy:   !opts.NoTidy,
		Notify: applyProgress.Notify,
	})
	cancelApply()
	renderLocalRepositoryApplyResult(cmd.OutOrStdout(), result)
	var hookErr error
	if !opts.NoHooks {
		phases := hookPlan.SelectedPhases(applyErr == nil)
		if len(phases) > 0 {
			hookCount := countLocalHooks(phases)
			hookTask := progress.Start(fmt.Sprintf("Running %d local update %s", hookCount, pluralize(hookCount, "hook", "hooks")))
			hookResults, err := hooks.NewRunner(executorTimeout).Run(ctx, phases, localRepositoryHookContext(plan, result, applyErr))
			if err != nil {
				hookTask.Fail("Local update hooks failed")
			} else {
				hookTask.Success(fmt.Sprintf("Completed %d local update %s", hookCount, pluralize(hookCount, "hook", "hooks")))
			}
			renderLocalHookResults(cmd.OutOrStdout(), hookResults)
			hookErr = err
		}
	}
	if applyErr != nil {
		message := localApplyFailureMessage(applyErr, executorTimeout)
		if hookErr != nil {
			return newExecutionError(message+" and failed to run local update hooks", errors.Join(applyErr, hookErr))
		}
		return newExecutionError(message, applyErr)
	}
	if hookErr != nil {
		return newExecutionError("failed to run local update hooks", hookErr)
	}
	return nil
}

func localApplyContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, timeout)
}

func localApplyFailureMessage(err error, timeout time.Duration) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded) && timeout > 0:
		return fmt.Sprintf("local dependency update timed out after %s", timeout)
	case errors.Is(err, context.Canceled):
		return "local dependency update canceled"
	default:
		return "failed to apply local dependency updates"
	}
}

func newLocalProgress(cmd *cobra.Command) *cliui.Progress {
	options := cliui.Options{}
	if localCfg := localCommandConfig(); localCfg != nil {
		options.Quiet = localCfg.Logging.Quiet
		options.Verbose = localCfg.Logging.Verbose
	}
	return cliui.NewProgress(cmd.ErrOrStderr(), options)
}

func localRepositoryPlanProgressMessage(plan localupdate.RepositoryPlan) string {
	updates := plan.Updates()
	modules := len(plan.Plans)
	return fmt.Sprintf("Planned %d %s across %d candidates in %d %s", updates, pluralize(updates, "update", "updates"), plan.Candidates(), modules, pluralize(modules, "module", "modules"))
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
	multi    bool
}

func newLocalApplyProgress(progress *cliui.Progress, multi bool) *localApplyProgress {
	return &localApplyProgress{progress: progress, multi: multi}
}

func (p *localApplyProgress) Notify(event localupdate.ApplyEvent) {
	switch event.Kind {
	case localupdate.ApplyBatchStarted:
		p.current = p.progress.Start(p.withModule(event, fmt.Sprintf("Updating %d dependencies with one go get", event.Total)))
	case localupdate.ApplyBatchFinished:
		if event.Err != nil {
			p.current.Warn(p.withModule(event, "Combined dependency update did not complete"))
		} else {
			p.current.Success(p.withModule(event, fmt.Sprintf("Updated %d dependencies", event.Total)))
		}
		p.current = nil
	case localupdate.ApplyBatchFallback:
		p.progress.Warn(p.withModule(event, fmt.Sprintf("Retrying %d dependencies individually", event.Total)))
	case localupdate.ApplyItemStarted:
		p.current = p.progress.Start(p.withModule(event, fmt.Sprintf("Updating %s to %s (%d/%d)", event.Item.Module, event.Item.LocalVersion, event.Index, event.Total)))
	case localupdate.ApplyItemFinished:
		if event.Err != nil {
			p.current.Fail(p.withModule(event, fmt.Sprintf("Failed to update %s", event.Item.Module)))
		} else {
			p.current.Success(p.withModule(event, fmt.Sprintf("Updated %s to %s", event.Item.Module, event.Item.LocalVersion)))
		}
		p.current = nil
	case localupdate.ApplyTidyStarted:
		p.current = p.progress.Start(p.withModule(event, "Running go mod tidy"))
	case localupdate.ApplyTidyFinished:
		if event.Err != nil {
			p.current.Fail(p.withModule(event, "go mod tidy failed"))
		} else {
			p.current.Success(p.withModule(event, "go mod tidy completed"))
		}
		p.current = nil
	}
}

func (p *localApplyProgress) withModule(event localupdate.ApplyEvent, message string) string {
	if !p.multi || strings.TrimSpace(event.Module) == "" {
		return message
	}
	return fmt.Sprintf("%s: %s", event.Module, message)
}

func commandContext(cmd *cobra.Command) context.Context {
	if cmd == nil || cmd.Context() == nil {
		return context.Background()
	}
	return cmd.Context()
}

func localRepositoryHookContext(plan localupdate.RepositoryPlan, result *localupdate.RepositoryApplyResult, applyErr error) hooks.Context {
	modules := make([]string, 0, len(plan.Plans))
	moduleDirs := make([]string, 0, len(plan.Plans))
	module := ""
	for _, modulePlan := range plan.Plans {
		modules = append(modules, modulePlan.CurrentModule)
		moduleDirs = append(moduleDirs, modulePlan.ModuleDir)
		if modulePlan.ModuleDir == plan.Repository.Root || len(plan.Plans) == 1 {
			module = modulePlan.CurrentModule
		}
	}
	hookCtx := hooks.Context{
		Command:      "update local",
		Module:       module,
		Modules:      modules,
		ModuleDir:    plan.Repository.Root,
		ModuleDirs:   moduleDirs,
		ModuleCount:  len(plan.Plans),
		Workspace:    plan.Workspace(),
		UpdateStatus: "success",
	}
	if applyErr != nil {
		hookCtx.UpdateStatus = "failure"
	}
	if result != nil {
		hookCtx.UpdatedCount = result.UpdatedCount()
		hookCtx.TidyCount = result.TidyCount()
		hookCtx.TidyRan = hookCtx.TidyCount > 0
		hookCtx.TidyFailed = result.TidyFailed()
	}
	return hookCtx
}

func buildLocalRepositoryPlan(cmd *cobra.Command, opts localCommandOptions) (localupdate.RepositoryPlan, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return localupdate.RepositoryPlan{}, newValidationError("could not determine the current directory", err)
	}
	repository, err := localupdate.DiscoverRepository(cwd, localupdate.DiscoveryOptions{
		RespectGitIgnore: opts.GitIgnore,
	})
	if err != nil {
		return localupdate.RepositoryPlan{}, newValidationError("could not discover repository Go modules", err)
	}

	workspacePath, workspaceExplicit := localWorkspace(cmd)
	plan, err := localupdate.PlanRepository(repository, localupdate.Request{
		Workspace:         workspacePath,
		WorkspaceExplicit: workspaceExplicit,
		Prefixes:          opts.Prefixes,
		IncludeIndirect:   opts.IncludeIndirect,
		Only:              opts.Only,
		Exclude:           opts.Exclude,
		Tidy:              !opts.NoTidy,
	})
	if err != nil {
		return localupdate.RepositoryPlan{}, newPlanningError("failed to plan local dependency updates", err)
	}
	return plan, nil
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
	cmd.Flags().BoolVar(&opts.GitIgnore, "gitignore", false, "Exclude files and directories matched by repository .gitignore files")
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

func renderLocalRepositoryPlan(out io.Writer, plan localupdate.RepositoryPlan, title string) {
	if len(plan.Plans) == 1 {
		renderLocalPlan(out, plan.Plans[0], title)
		renderExternalWorkUses(out, plan.Repository.ExternalUses)
		return
	}
	fmt.Fprintf(out, "%s for repository %s\n", title, plan.Repository.Root)
	fmt.Fprintf(out, "Modules: %d\n", len(plan.Plans))
	if plan.Repository.WorkFile != "" {
		fmt.Fprintf(out, "Go workspace: %s\n", plan.Repository.WorkFile)
	}
	fmt.Fprintf(out, "Dependency workspace: %s\n", plan.Workspace())
	renderExternalWorkUses(out, plan.Repository.ExternalUses)
	for _, modulePlan := range plan.Plans {
		fmt.Fprintf(out, "\nModule: %s\n", modulePlan.CurrentModule)
		fmt.Fprintf(out, "Module dir: %s\n", modulePlan.ModuleDir)
		if len(modulePlan.Items) == 0 {
			fmt.Fprintln(out, "No local dependency candidates found.")
			continue
		}
		fmt.Fprintln(out, "Candidates:")
		for _, item := range modulePlan.Items {
			renderLocalItem(out, item)
		}
		renderLocalSummary(out, modulePlan.Items)
	}
	renderRepositorySummary(out, len(plan.Plans), aggregatePlanItems(plan), plan.Updates(), 0, 0)
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
	if result.Interruption != nil {
		fmt.Fprintf(out, "\nlocal update interrupted: %v\n", result.Interruption)
	}
	renderLocalSummary(out, result.Items)
}

func renderLocalRepositoryApplyResult(out io.Writer, result *localupdate.RepositoryApplyResult) {
	if result == nil {
		fmt.Fprintln(out, "No local update result available.")
		return
	}
	if len(result.Results) == 1 {
		renderLocalApplyResult(out, result.Results[0])
		renderExternalWorkUses(out, result.Plan.Repository.ExternalUses)
		return
	}
	fmt.Fprintf(out, "Local dependency update for repository %s\n", result.Plan.Repository.Root)
	fmt.Fprintf(out, "Modules: %d\n", len(result.Plan.Plans))
	if result.Plan.Repository.WorkFile != "" {
		fmt.Fprintf(out, "Go workspace: %s\n", result.Plan.Repository.WorkFile)
	}
	fmt.Fprintf(out, "Dependency workspace: %s\n", result.Plan.Workspace())
	renderExternalWorkUses(out, result.Plan.Repository.ExternalUses)
	items := make([]localupdate.Item, 0)
	for _, moduleResult := range result.Results {
		if moduleResult == nil {
			continue
		}
		fmt.Fprintf(out, "\nModule: %s\n", moduleResult.Plan.CurrentModule)
		fmt.Fprintf(out, "Module dir: %s\n", moduleResult.Plan.ModuleDir)
		if len(moduleResult.Items) == 0 {
			fmt.Fprintln(out, "No local dependency candidates found.")
			continue
		}
		fmt.Fprintln(out, "Results:")
		for _, item := range moduleResult.Items {
			renderLocalItem(out, item)
		}
		items = append(items, moduleResult.Items...)
		if moduleResult.TidyRun {
			if moduleResult.TidyFailed {
				fmt.Fprintf(out, "go mod tidy: failed - %v\n", moduleResult.TidyError)
			} else {
				fmt.Fprintln(out, "go mod tidy: completed")
			}
		}
		if moduleResult.Interruption != nil {
			fmt.Fprintf(out, "local update interrupted: %v\n", moduleResult.Interruption)
		}
		renderLocalSummary(out, moduleResult.Items)
	}
	renderRepositorySummary(out, len(result.Plan.Plans), items, result.UpdatedCount(), result.TidyCount(), repositoryFailureCount(result))
}

func aggregatePlanItems(plan localupdate.RepositoryPlan) []localupdate.Item {
	items := make([]localupdate.Item, 0, plan.Candidates())
	for _, modulePlan := range plan.Plans {
		items = append(items, modulePlan.Items...)
	}
	return items
}

func renderRepositorySummary(out io.Writer, modules int, items []localupdate.Item, updates, tidies, failures int) {
	fmt.Fprintln(out, "\nRepository summary:")
	fmt.Fprintf(out, "  modules: %d\n", modules)
	fmt.Fprintf(out, "  candidates: %d\n", len(items))
	fmt.Fprintf(out, "  updates: %d\n", updates)
	fmt.Fprintf(out, "  tidied-modules: %d\n", tidies)
	fmt.Fprintf(out, "  failures: %d\n", failures)
}

func repositoryFailureCount(result *localupdate.RepositoryApplyResult) int {
	if result == nil {
		return 0
	}
	count := 0
	for _, moduleResult := range result.Results {
		if moduleResult == nil {
			continue
		}
		moduleFailures := 0
		for _, item := range moduleResult.Items {
			if item.Status == localupdate.StatusApplyFailed {
				moduleFailures++
			}
		}
		if moduleResult.TidyFailed {
			moduleFailures++
		}
		if moduleResult.HasFailures && moduleFailures == 0 {
			moduleFailures = 1
		}
		count += moduleFailures
	}
	return count
}

func renderExternalWorkUses(out io.Writer, paths []string) {
	if len(paths) == 0 {
		return
	}
	fmt.Fprintln(out, "External go.work modules skipped:")
	for _, path := range paths {
		fmt.Fprintf(out, "  - %s\n", path)
	}
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
