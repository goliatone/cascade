package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/localupdate"
	"github.com/spf13/cobra"
)

type localCommandOptions struct {
	Prefixes        []string
	IncludeIndirect bool
	Only            []string
	Exclude         []string
	NoTidy          bool
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

	dryRun := false
	if container != nil && container.Config() != nil {
		cfg := container.Config()
		dryRun = cfg.Executor.DryRun
	}
	if dryRun {
		renderLocalPlan(cmd.OutOrStdout(), plan, "DRY RUN: Local dependency update plan")
		return nil
	}

	result, applyErr := localupdate.ApplyPlan(context.Background(), plan, executor.NewGoOperations(), localupdate.ApplyOptions{
		Tidy: !opts.NoTidy,
	})
	renderLocalApplyResult(cmd.OutOrStdout(), result)
	if applyErr != nil {
		return newExecutionError("failed to apply local dependency updates", applyErr)
	}
	return nil
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
		path := strings.TrimSpace(container.Config().Workspace.Path)
		if path != "" && !isDefaultLocalCacheWorkspace(path) {
			return path, true
		}
	}
	return "", false
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
