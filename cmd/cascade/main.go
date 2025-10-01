package main

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	execpkg "github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/internal/state"
	"github.com/goliatone/cascade/pkg/config"
	"github.com/goliatone/cascade/pkg/di"
	"github.com/goliatone/cascade/pkg/util/modpath"
	gh "github.com/google/go-github/v66/github"
	oauth2 "golang.org/x/oauth2"
)

// Exit codes for different error types
const (
	ExitSuccess         = 0  // Successful execution
	ExitGenericError    = 1  // Generic error
	ExitConfigError     = 2  // Configuration error
	ExitValidationError = 3  // Input validation error
	ExitNetworkError    = 4  // Network/connectivity error
	ExitFileError       = 5  // File system error
	ExitStateError      = 6  // State management error
	ExitPlanningError   = 7  // Planning phase error
	ExitExecutionError  = 8  // Execution phase error
	ExitInterruptError  = 9  // User interruption (SIGINT, etc.)
	ExitResourceError   = 10 // Resource exhaustion (disk, memory, etc.)
)

// Global variables for CLI state
var (
	container di.Container
	cfg       *config.Config
)

func main() {
	if err := execute(); err != nil {
		handleCLIError(err)
	}
}

// handleCLIError processes and exits with appropriate error codes
func handleCLIError(err error) {
	if err == nil {
		return
	}

	// Handle structured errors with appropriate exit codes
	if cliErr, ok := err.(*CLIError); ok {
		fmt.Fprintf(os.Stderr, "cascade: %s\n", cliErr.Message)
		if cliErr.Cause != nil {
			fmt.Fprintf(os.Stderr, "  Cause: %v\n", cliErr.Cause)
		}
		os.Exit(cliErr.ExitCode())
	}

	// Try to infer error type from error message patterns for better exit codes
	errorMsg := err.Error()

	// Configuration and validation errors
	if strings.Contains(errorMsg, "configuration") || strings.Contains(errorMsg, "config") {
		fmt.Fprintf(os.Stderr, "cascade: configuration error: %v\n", err)
		os.Exit(ExitConfigError)
	}

	// File system errors
	if strings.Contains(errorMsg, "no such file") || strings.Contains(errorMsg, "permission denied") ||
		strings.Contains(errorMsg, "file not found") || strings.Contains(errorMsg, "manifest") {
		fmt.Fprintf(os.Stderr, "cascade: file error: %v\n", err)
		os.Exit(ExitFileError)
	}

	// Validation errors
	if strings.Contains(errorMsg, "must be specified") || strings.Contains(errorMsg, "invalid") ||
		strings.Contains(errorMsg, "validation") || strings.Contains(errorMsg, "required") {
		fmt.Fprintf(os.Stderr, "cascade: validation error: %v\n", err)
		os.Exit(ExitValidationError)
	}

	// Network/connectivity errors
	if strings.Contains(errorMsg, "network") || strings.Contains(errorMsg, "connection") ||
		strings.Contains(errorMsg, "timeout") || strings.Contains(errorMsg, "unreachable") {
		fmt.Fprintf(os.Stderr, "cascade: network error: %v\n", err)
		os.Exit(ExitNetworkError)
	}

	// Generic error fallback
	fmt.Fprintf(os.Stderr, "cascade: %v\n", err)
	os.Exit(ExitGenericError)
}

// execute is the main entry point that sets up and runs the CLI
func execute() error {
	rootCmd := newRootCommand()
	return rootCmd.Execute()
}

// Helper function to split module@version strings
func splitModuleVersion(stateID string) []string {
	parts := strings.Split(stateID, "@")
	if len(parts) != 2 {
		return nil
	}
	return parts
}

// Helper functions for manifest generation

// detectModuleInfo walks up from the current working directory to locate a go.mod file
// and returns the module path with the directory that contains it. It enables
// `cascade manifest generate` to infer sensible defaults without requiring flags.

// detectDefaultVersion inspects common local sources for a module version.
// Priority: `.version` file (or `VERSION`), then latest annotated tag in git.
// Returns any warnings encountered while probing so the CLI can surface them.

// normalizeVersionString trims whitespace and ensures versions have the expected
// leading "v" when the underlying data omits it.

// deriveModuleName extracts the module name from the module path

// deriveRepository converts module path to repository format for known hosts

// deriveLocalModulePath calculates the relative path from repository root to module

// buildDependentOptions converts string list to DependentOptions slice

// buildCloneURL ensures the repo string is a valid cloneable URL.
// This mirrors the logic from internal/executor/git.go to maintain consistency.

// resolveGenerateOutputPath determines where to write the generated manifest

// resolveManifestPath determines the manifest path respecting CLI input, config defaults, and workspace.

// resolvePlanManifestPath determines manifest path for plan command with flag and positional arg support

// ensureWorkspace guarantees the workspace directory exists before execution.
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

// resolveModuleVersion extracts the module/version pair either from CLI state identifier or config.
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

// printResumeSummary reports the work items that would be processed during a dry-run resume.
func printResumeSummary(module, version string, itemStates []state.ItemState, plan *planner.Plan) {
	fmt.Printf("DRY RUN: Would resume cascade for %s@%s\n", module, version)
	if plan == nil || len(plan.Items) == 0 {
		fmt.Println("No work items available in regenerated plan")
		return
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

// runGitCommand executes a git subcommand using the provided runner.
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

// extractPRNumber parses a pull request URL and extracts the numeric identifier.
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

// appendReason concatenates reason strings with a delimiter while avoiding duplicates.
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

// discoverWorkspaceDependents uses the workspace discovery to find dependent modules

// dependentsOptionsToStrings converts DependentOptions to string slice for CLI compatibility

// resolveVersionFromWorkspace resolves the module version using workspace discovery

// displayDiscoverySummary shows the discovery results and handles user confirmation

// newGitHubClient constructs a GitHub client using configuration and shared HTTP client settings

// normalizeEnterpriseEndpoints mirrors pkg/di provider logic for GitHub enterprise endpoints

// matchesRepoPatterns evaluates include/exclude patterns against repository names

// fetchModuleInfoFromGitHub downloads go.mod content and extracts module information

// parseGoModModulePath extracts the module path from go.mod content

// resolveGitHubOrg returns the GitHub organization from CLI flag or config

// discoverGitHubDependents discovers dependent repositories in a GitHub organization
// This is a placeholder implementation for Task 3.2 - actual implementation comes in Task 3.1

// discoverGitHubDependentsWithClient executes GitHub discovery using a prepared client (primarily for testing)

// performMultiSourceDiscovery performs discovery from multiple sources and merges the results.
// This implements Task 3.3: Result Merging & Conflict Resolution.

// mergeDiscoveryResults merges and deduplicates discovery results from multiple sources.
// Deduplication is based on repository name and module path pairs.

// dependentKey creates a unique key for deduplication based on repository and module path.

// mergeConflictingDependents merges two DependentOptions that refer to the same repo/module.
// It prefers more complete information and logs the merge decisions.

// promptForDependentSelection allows users to interactively select which discovered
// dependents to include in the manifest.

// parseSelectionInput parses user input for dependent selection.
// Supports formats like "1,2,3", "1-3,5", etc.
