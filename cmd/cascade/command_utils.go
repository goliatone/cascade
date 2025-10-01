package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	execpkg "github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/internal/state"
	"github.com/goliatone/cascade/pkg/config"
)

func ensureWorkspace(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("workspace path is empty")
	}

	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("resolve workspace path: %w", err)
		}
		path = abs
	}

	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create workspace directory: %w", err)
	}

	return nil
}

func resolveModuleVersion(stateID string, cfg *config.Config) (string, string, error) {
	if trimmed := strings.TrimSpace(stateID); trimmed != "" {
		parts := splitModuleVersion(trimmed)
		if parts == nil {
			return "", "", fmt.Errorf("state identifier must be in module@version format: %s", stateID)
		}
		return parts[0], parts[1], nil
	}

	if cfg == nil {
		return "", "", fmt.Errorf("module and version must be provided via flags or state identifier")
	}

	module := strings.TrimSpace(cfg.Module)
	version := strings.TrimSpace(cfg.Version)
	if module == "" || version == "" {
		return "", "", fmt.Errorf("module and version must be provided via --module and --version flags or state identifier")
	}

	return module, version, nil
}

func printResumeSummary(module, version string, itemStates []state.ItemState, plan *planner.Plan) {
	fmt.Printf("DRY RUN: Would resume cascade for %s@%s\n", module, version)
	if plan == nil || len(plan.Items) == 0 {
		fmt.Println("No work items available in regenerated plan")
		return
	}

	if len(plan.Stats.SkippedUpToDateRepos) > 0 {
		fmt.Printf("%d repositories already up-to-date, skipped: %s\n",
			len(plan.Stats.SkippedUpToDateRepos), strings.Join(plan.Stats.SkippedUpToDateRepos, ", "))
	}

	stateByRepo := make(map[string]state.ItemState, len(itemStates))
	for _, st := range itemStates {
		stateByRepo[st.Repo] = st
	}

	fmt.Printf("Plan contains %d work items:\n", len(plan.Items))
	for i, item := range plan.Items {
		status := "pending"
		reason := ""
		if st, ok := stateByRepo[item.Repo]; ok {
			if st.Status != "" {
				status = string(st.Status)
			}
			reason = st.Reason
		}
		fmt.Printf("  %d. %s (%s) -> %s [%s]", i+1, item.Repo, item.Module, item.BranchName, status)
		if strings.TrimSpace(reason) != "" {
			fmt.Printf(" - %s", reason)
		}
		fmt.Println()
	}
}

func runGitCommand(ctx context.Context, runner execpkg.GitCommandRunner, repoPath string, args ...string) error {
	if runner == nil {
		return fmt.Errorf("git command runner not configured")
	}
	if len(args) == 0 {
		return fmt.Errorf("git command requires arguments")
	}
	_, err := runner.Run(ctx, repoPath, args...)
	return err
}

func extractPRNumber(prURL string) (int, error) {
	parsed, err := url.Parse(prURL)
	if err != nil {
		return 0, fmt.Errorf("invalid PR URL: %w", err)
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) == 0 {
		return 0, fmt.Errorf("no path segments in PR URL: %s", prURL)
	}
	num, err := strconv.Atoi(segments[len(segments)-1])
	if err != nil {
		return 0, fmt.Errorf("failed to parse PR number from URL %s: %w", prURL, err)
	}
	return num, nil
}

func appendReason(existing, addition string) string {
	addition = strings.TrimSpace(addition)
	if addition == "" {
		return strings.TrimSpace(existing)
	}
	existing = strings.TrimSpace(existing)
	if existing == "" {
		return addition
	}
	return existing + "; " + addition
}
