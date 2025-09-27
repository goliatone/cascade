package main

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	execpkg "github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/internal/state"
	"github.com/goliatone/cascade/pkg/config"
	"github.com/goliatone/cascade/pkg/di"
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
func detectModuleInfo() (string, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("determine working directory: %w", err)
	}

	for dir := cwd; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		goModPath := filepath.Join(dir, "go.mod")
		info, err := os.Stat(goModPath)
		if err != nil {
			if os.IsNotExist(err) {
				if parent := filepath.Dir(dir); parent == dir {
					break
				}
				continue
			}
			return "", "", fmt.Errorf("stat go.mod: %w", err)
		}
		if info.IsDir() {
			continue
		}

		content, err := os.ReadFile(goModPath)
		if err != nil {
			return "", "", fmt.Errorf("read go.mod: %w", err)
		}
		modulePath := parseGoModModulePath(string(content))
		if modulePath == "" {
			return "", "", fmt.Errorf("module declaration not found in %s", goModPath)
		}
		return modulePath, dir, nil
	}

	return "", "", fmt.Errorf("go.mod not found in current tree")
}

// detectDefaultVersion inspects common local sources for a module version.
// Priority: `.version` file (or `VERSION`), then latest annotated tag in git.
// Returns any warnings encountered while probing so the CLI can surface them.
func detectDefaultVersion(ctx context.Context, moduleDir string) (string, []string) {
	var warnings []string

	if strings.TrimSpace(moduleDir) == "" {
		return "", warnings
	}

	// Look for marker files first so projects can override without git metadata.
	versionFiles := []string{filepath.Join(moduleDir, ".version"), filepath.Join(moduleDir, "VERSION")}
	for _, candidate := range versionFiles {
		data, err := os.ReadFile(candidate)
		if err != nil {
			if !os.IsNotExist(err) {
				warnings = append(warnings, fmt.Sprintf("failed to read %s: %v", candidate, err))
			}
			continue
		}
		if v := normalizeVersionString(string(data)); v != "" {
			return v, warnings
		}
	}

	// Fallback to git tags if repository information is available.
	if _, err := os.Stat(filepath.Join(moduleDir, ".git")); err == nil {
		cmd := exec.CommandContext(ctx, "git", "-C", moduleDir, "describe", "--tags", "--abbrev=0")
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		output, err := cmd.Output()
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("git tag detection failed: %v", err))
		} else if v := normalizeVersionString(string(output)); v != "" {
			return v, warnings
		}
	} else if !os.IsNotExist(err) {
		warnings = append(warnings, fmt.Sprintf("git metadata unavailable: %v", err))
	}

	return "", warnings
}

// normalizeVersionString trims whitespace and ensures versions have the expected
// leading "v" when the underlying data omits it.
func normalizeVersionString(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "v") {
		return trimmed
	}
	if trimmed[0] >= '0' && trimmed[0] <= '9' {
		return "v" + trimmed
	}
	return trimmed
}

// deriveModuleName extracts the module name from the module path
func deriveModuleName(modulePath string) string {
	if modulePath == "" {
		return ""
	}
	// Extract the last part after the final slash
	parts := strings.Split(modulePath, "/")
	return parts[len(parts)-1]
}

// deriveRepository converts module path to repository format for known hosts
func deriveRepository(modulePath string) string {
	if modulePath == "" {
		return ""
	}

	// For common hosting providers, extract the repository part (owner/repo)
	parts := strings.Split(modulePath, "/")
	if len(parts) >= 3 {
		switch parts[0] {
		case "github.com", "gitlab.com", "bitbucket.org":
			return strings.Join(parts[1:3], "/")
		}
	}

	// For non-hosted URLs or unknown hosts, preserve the original module path
	// This prevents breaking URLs like go.uber.org/zap into invalid repository names
	return modulePath
}

// deriveGitHubOrgFromModule extracts the GitHub organization from a module path when available.
func deriveGitHubOrgFromModule(modulePath string) string {
	parts := strings.Split(modulePath, "/")
	if len(parts) >= 3 && parts[0] == "github.com" {
		return parts[1]
	}
	return ""
}

// deriveLocalModulePath calculates the relative path from repository root to module
func deriveLocalModulePath(modulePath string) string {
	parts := strings.Split(modulePath, "/")
	if len(parts) >= 4 {
		switch parts[0] {
		case "github.com", "gitlab.com", "bitbucket.org":
			// For hosted repos, everything after org/repo is the local path
			return strings.Join(parts[3:], "/")
		}
	}
	// For non-hosted URLs or short paths, default to root
	// This handles cases like go.uber.org/zap where the entire URL is the "repository"
	return "."
}

// buildDependentOptions converts string list to DependentOptions slice
func buildDependentOptions(dependents []string) []manifest.DependentOptions {
	if len(dependents) == 0 {
		return []manifest.DependentOptions{}
	}

	options := make([]manifest.DependentOptions, len(dependents))
	for i, dep := range dependents {
		// Handle format: owner/repo or full module path
		repo := strings.TrimSpace(dep)
		modulePath := ""

		// If it looks like a GitHub repository, convert to module path
		if strings.Count(repo, "/") == 1 && !strings.Contains(repo, ".") {
			modulePath = "github.com/" + repo
		} else {
			modulePath = repo
			repo = deriveRepository(repo)
		}

		options[i] = manifest.DependentOptions{
			Repository:      repo,
			ModulePath:      modulePath,
			LocalModulePath: deriveLocalModulePath(modulePath),
		}
	}

	return options
}

// resolveGenerateOutputPath determines where to write the generated manifest
func resolveGenerateOutputPath(outputPath string, cfg *config.Config) string {
	// Use explicit output path if provided
	if outputPath != "" {
		if !filepath.IsAbs(outputPath) {
			if abs, err := filepath.Abs(outputPath); err == nil {
				return abs
			}
		}
		return outputPath
	}

	// Use config workspace manifest path
	if cfg != nil && cfg.Workspace.ManifestPath != "" {
		return cfg.Workspace.ManifestPath
	}

	// Default to hidden manifest in current directory to avoid clobbering existing files
	if abs, err := filepath.Abs(".cascade.yaml"); err == nil {
		return abs
	}

	return ".cascade.yaml"
}

// resolveManifestPath determines the manifest path respecting CLI input, config defaults, and workspace.
func resolveManifestPath(manifestPath string, cfg *config.Config) string {
	path := strings.TrimSpace(manifestPath)
	if path != "" {
		if !filepath.IsAbs(path) {
			if abs, err := filepath.Abs(path); err == nil {
				return abs
			}
		}
		return path
	}

	// Check for .cascade.yaml in current working directory first
	if abs, err := filepath.Abs(".cascade.yaml"); err == nil {
		return abs
	}

	if cfg != nil {
		if candidate := strings.TrimSpace(cfg.Workspace.ManifestPath); candidate != "" {
			return candidate
		}
		if base := strings.TrimSpace(cfg.Workspace.Path); base != "" {
			return filepath.Join(base, ".cascade.yaml")
		}
	}

	return ""
}

// resolvePlanManifestPath determines manifest path for plan command with flag and positional arg support
func resolvePlanManifestPath(manifestFlag, manifestArg string, cfg *config.Config) string {
	// Use explicit flag if provided
	if manifestFlag != "" {
		return resolveManifestPath(manifestFlag, cfg)
	}

	// Use positional argument if provided
	if manifestArg != "" {
		return resolveManifestPath(manifestArg, cfg)
	}

	// Use default resolution
	return resolveManifestPath("", cfg)
}

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

// extractOrgFromModulePath extracts the organization from a module path
func extractOrgFromModulePath(modulePath string) string {
	parts := strings.Split(modulePath, "/")
	if len(parts) >= 2 {
		switch parts[0] {
		case "github.com", "gitlab.com", "bitbucket.org":
			return parts[1]
		}
	}
	return ""
}

// containsMultipleModules checks if a directory contains multiple Go modules
func containsMultipleModules(dir string) bool {
	moduleCount := 0
	maxCheck := 50 // Limit to avoid scanning huge directories

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue on errors
		}

		// Stop if we've checked too many entries
		if moduleCount >= maxCheck {
			return filepath.SkipDir
		}

		// Skip deep nested directories
		if strings.Count(strings.TrimPrefix(path, dir), string(filepath.Separator)) > 3 {
			return filepath.SkipDir
		}

		// Skip common non-module directories
		base := filepath.Base(path)
		if base == ".git" || base == "vendor" || base == "node_modules" || base == ".cache" {
			return filepath.SkipDir
		}

		if info.Name() == "go.mod" {
			moduleCount++
			if moduleCount >= 2 {
				return filepath.SkipAll // Found multiple modules, we can stop
			}
		}

		return nil
	})

	if err != nil {
		return false
	}

	return moduleCount >= 2
}

// discoverWorkspaceDependents uses the workspace discovery to find dependent modules
func discoverWorkspaceDependents(ctx context.Context, targetModule, workspaceDir string, maxDepth int, includePatterns, excludePatterns []string, cfg *config.Config, logger di.Logger) ([]manifest.DependentOptions, error) {
	discovery := manifest.NewWorkspaceDiscovery()

	// Apply config defaults for discovery options
	finalMaxDepth := getDiscoveryMaxDepth(maxDepth, cfg)
	finalIncludePatterns := getDiscoveryIncludePatterns(includePatterns, cfg)
	finalExcludePatterns := getDiscoveryExcludePatterns(excludePatterns, cfg)

	options := manifest.DiscoveryOptions{
		WorkspaceDir:    workspaceDir,
		TargetModule:    targetModule,
		MaxDepth:        finalMaxDepth,
		IncludePatterns: finalIncludePatterns,
		ExcludePatterns: finalExcludePatterns,
	}

	dependents, err := discovery.DiscoverDependents(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("workspace discovery failed: %w", err)
	}

	if logger != nil {
		logger.Debug("Workspace discovery completed",
			"target_module", targetModule,
			"workspace", workspaceDir,
			"found_dependents", len(dependents),
			"max_depth", maxDepth)
	}

	return dependents, nil
}

// dependentsOptionsToStrings converts DependentOptions to string slice for CLI compatibility
func dependentsOptionsToStrings(dependents []manifest.DependentOptions) []string {
	if len(dependents) == 0 {
		return []string{}
	}

	result := make([]string, len(dependents))
	for i, dep := range dependents {
		// Use repository format (owner/repo) for CLI compatibility
		result[i] = dep.Repository
	}

	return result
}

// resolveVersionFromWorkspace resolves the module version using workspace discovery
func resolveVersionFromWorkspace(ctx context.Context, modulePath, version, workspaceDir string, logger di.Logger) (string, []string, error) {
	discovery := manifest.NewWorkspaceDiscovery()

	var strategy manifest.VersionResolutionStrategy
	allowNetwork := true

	if version == "latest" {
		strategy = manifest.VersionResolutionLatest
	} else {
		// Auto strategy: try local first, then network
		strategy = manifest.VersionResolutionAuto
	}

	options := manifest.VersionResolutionOptions{
		WorkspaceDir:       workspaceDir,
		TargetModule:       modulePath,
		Strategy:           strategy,
		AllowNetworkAccess: allowNetwork,
	}

	resolution, err := discovery.ResolveVersion(ctx, options)
	if err != nil {
		return "", nil, err
	}

	// Log resolution source
	if logger != nil {
		logger.Info("Version resolved",
			"module", modulePath,
			"version", resolution.Version,
			"source", string(resolution.Source),
			"source_path", resolution.SourcePath)
	}

	return resolution.Version, resolution.Warnings, nil
}

// displayDiscoverySummary shows the discovery results and handles user confirmation
func displayDiscoverySummary(modulePath, version, workspaceDir string, discoveredDependents []manifest.DependentOptions, finalDependents, versionWarnings []string, yes, nonInteractive, dryRun bool) error {
	// Always show summary if discovery was performed or if we have dependents
	shouldShowSummary := workspaceDir != "" || len(finalDependents) > 0

	if !shouldShowSummary {
		return nil
	}

	// Display summary
	fmt.Printf("Generating manifest for %s@%s\n", modulePath, version)

	if workspaceDir != "" {
		fmt.Printf("Discovery workspace: %s\n", workspaceDir)
	}

	if len(discoveredDependents) > 0 {
		fmt.Printf("Discovered %d dependent repositories:\n", len(discoveredDependents))
		for i, dep := range discoveredDependents {
			fmt.Printf("  %d. %s (module: %s)\n", i+1, dep.Repository, dep.ModulePath)
		}
	} else if len(finalDependents) > 0 {
		fmt.Printf("Using %d configured dependent repositories:\n", len(finalDependents))
		for i, dep := range finalDependents {
			fmt.Printf("  %d. %s\n", i+1, dep)
		}
	} else {
		fmt.Println("No dependent repositories found or configured.")
	}

	// Show version warnings if any
	if len(versionWarnings) > 0 {
		fmt.Println("\nVersion Resolution Warnings:")
		for _, warning := range versionWarnings {
			fmt.Printf("  ! %s\n", warning)
		}
	}

	// Default test commands that will be applied
	fmt.Println("\nDefault configurations:")
	fmt.Println("  Branch: main")
	fmt.Println("  Labels: [automation:cascade]")
	fmt.Println("  Test commands: go test ./... -race -count=1")
	fmt.Println("  Commit template: chore(deps): bump {{ .Module }} to {{ .Version }}")
	fmt.Println("  PR title: chore(deps): bump {{ .Module }} to {{ .Version }}")

	// Handle confirmation unless in dry-run mode, yes flag, or non-interactive mode
	if !dryRun && !yes && !nonInteractive {
		fmt.Printf("\nProceed with manifest generation? [Y/n]: ")
		var response string
		fmt.Scanln(&response)

		// Default to yes if empty response, check for explicit no
		if response != "" && (response == "n" || response == "N" || response == "no" || response == "NO") {
			fmt.Println("Manifest generation cancelled.")
			return fmt.Errorf("manifest generation cancelled by user")
		}
	}

	if dryRun {
		fmt.Println("\n--- DRY RUN: Would proceed with manifest generation ---")
	} else if yes || nonInteractive {
		fmt.Println("\n--- Proceeding with manifest generation ---")
	}

	return nil
}

// newGitHubClient constructs a GitHub client using configuration and shared HTTP client settings
func newGitHubClient(ctx context.Context, cfg *config.Config) (*gh.Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration required for GitHub discovery")
	}

	token := strings.TrimSpace(cfg.Integration.GitHub.Token)
	if token == "" {
		token = strings.TrimSpace(os.Getenv(config.EnvGitHubToken))
	}
	if token == "" {
		token = strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	}
	if token == "" {
		token = strings.TrimSpace(os.Getenv("GH_TOKEN"))
	}
	if token == "" {
		return nil, fmt.Errorf("github token required for discovery")
	}

	var baseHTTP *http.Client
	if container != nil {
		baseHTTP = container.HTTPClient()
	}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	oauthClient := oauth2.NewClient(ctx, ts)

	if baseHTTP != nil {
		if transport, ok := oauthClient.Transport.(*oauth2.Transport); ok {
			if baseHTTP.Transport != nil {
				transport.Base = baseHTTP.Transport
			}
		}
		if baseHTTP.Timeout > 0 {
			oauthClient.Timeout = baseHTTP.Timeout
		}
	}

	endpoint := strings.TrimSpace(cfg.Integration.GitHub.Endpoint)
	if endpoint == "" {
		return gh.NewClient(oauthClient), nil
	}

	baseURL, uploadURL := normalizeEnterpriseEndpoints(endpoint)
	client, err := gh.NewEnterpriseClient(baseURL, uploadURL, oauthClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub enterprise client: %w", err)
	}
	return client, nil
}

// normalizeEnterpriseEndpoints mirrors pkg/di provider logic for GitHub enterprise endpoints
func normalizeEnterpriseEndpoints(endpoint string) (string, string) {
	base := strings.TrimSpace(endpoint)
	if base == "" {
		return "", ""
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}

	trimmed := strings.TrimSuffix(base, "/")
	if strings.HasSuffix(trimmed, "/api/v3") {
		prefix := strings.TrimSuffix(trimmed, "/api/v3")
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		return prefix + "api/v3/", prefix + "api/uploads/"
	}

	return base, base
}

// matchesRepoPatterns evaluates include/exclude patterns against repository names
func matchesRepoPatterns(repo string, includePatterns, excludePatterns []string) bool {
	repoLower := strings.ToLower(repo)
	repoName := repoLower
	if idx := strings.Index(repoLower, "/"); idx >= 0 {
		repoName = repoLower[idx+1:]
	}

	matchesPattern := func(pattern string) bool {
		pattern = strings.ToLower(pattern)
		if ok, _ := path.Match(pattern, repoLower); ok {
			return true
		}
		if ok, _ := path.Match(pattern, repoName); ok {
			return true
		}
		return false
	}

	for _, pattern := range excludePatterns {
		if matchesPattern(pattern) {
			return false
		}
	}

	if len(includePatterns) == 0 {
		return true
	}

	for _, pattern := range includePatterns {
		if matchesPattern(pattern) {
			return true
		}
	}

	return false
}

// fetchModuleInfoFromGitHub downloads go.mod content and extracts module information
func fetchModuleInfoFromGitHub(ctx context.Context, client *gh.Client, repo *gh.Repository, goModPath string) (string, string, error) {
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()

	file, _, resp, err := client.Repositories.GetContents(ctx, owner, name, goModPath, nil)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return "", "", fmt.Errorf("go.mod not found at %s", goModPath)
		}
		return "", "", err
	}

	content, err := file.GetContent()
	if err != nil {
		return "", "", err
	}

	modulePath := parseGoModModulePath(content)
	if modulePath == "" {
		modulePath = fmt.Sprintf("github.com/%s/%s", owner, name)
	}

	localPath := path.Dir(goModPath)
	if localPath == "." || localPath == "/" {
		localPath = "."
	}

	return modulePath, localPath, nil
}

// parseGoModModulePath extracts the module path from go.mod content
func parseGoModModulePath(content string) string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1]
			}
		}
	}
	return ""
}

// resolveGitHubOrg returns the GitHub organization from CLI flag or config
func resolveGitHubOrg(cliOrg string, cfg *config.Config) string {
	// CLI flag takes priority
	if cliOrg != "" {
		return cliOrg
	}

	// Check config for GitHub discovery organization
	if cfg != nil && cfg.ManifestGenerator.Discovery.GitHub.Organization != "" {
		return cfg.ManifestGenerator.Discovery.GitHub.Organization
	}

	// Check config for general GitHub organization (fallback)
	if cfg != nil && cfg.Integration.GitHub.Organization != "" {
		return cfg.Integration.GitHub.Organization
	}

	return ""
}

// discoverGitHubDependents discovers dependent repositories in a GitHub organization
// This is a placeholder implementation for Task 3.2 - actual implementation comes in Task 3.1
func discoverGitHubDependents(ctx context.Context, targetModule, organization string, includePatterns, excludePatterns []string, cfg *config.Config, logger di.Logger) ([]manifest.DependentOptions, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration required for GitHub discovery")
	}

	client, err := newGitHubClient(ctx, cfg)
	if err != nil {
		return nil, err
	}

	finalInclude := includePatterns
	finalExclude := excludePatterns

	if len(finalInclude) == 0 {
		finalInclude = cfg.ManifestGenerator.Discovery.GitHub.IncludePatterns
	}
	if len(finalExclude) == 0 {
		finalExclude = cfg.ManifestGenerator.Discovery.GitHub.ExcludePatterns
	}

	return discoverGitHubDependentsWithClient(ctx, client, targetModule, organization, finalInclude, finalExclude, logger)
}

// discoverGitHubDependentsWithClient executes GitHub discovery using a prepared client (primarily for testing)
func discoverGitHubDependentsWithClient(ctx context.Context, client *gh.Client, targetModule, organization string, includePatterns, excludePatterns []string, logger di.Logger) ([]manifest.DependentOptions, error) {
	if client == nil {
		return nil, fmt.Errorf("github client is required")
	}

	query := fmt.Sprintf("org:%s \"%s\" path:go.mod", organization, targetModule)
	options := &gh.SearchOptions{ListOptions: gh.ListOptions{PerPage: 100}}

	dependents := make([]manifest.DependentOptions, 0)
	fetchedRepos := make(map[string]struct{})

	for {
		results, resp, err := client.Search.Code(ctx, query, options)
		if err != nil {
			return nil, fmt.Errorf("github code search failed: %w", err)
		}

		for _, item := range results.CodeResults {
			repo := item.GetRepository()
			fullName := repo.GetFullName()

			if !matchesRepoPatterns(fullName, includePatterns, excludePatterns) {
				continue
			}

			modulePath, localModulePath, err := fetchModuleInfoFromGitHub(ctx, client, repo, item.GetPath())
			if err != nil {
				if logger != nil {
					logger.Warn("Failed to fetch module info from GitHub",
						"repository", fullName,
						"path", item.GetPath(),
						"error", err)
				}
				continue
			}

			key := fmt.Sprintf("%s|%s|%s", fullName, modulePath, localModulePath)
			if _, exists := fetchedRepos[key]; exists {
				continue
			}
			fetchedRepos[key] = struct{}{}

			dependents = append(dependents, manifest.DependentOptions{
				Repository:      fullName,
				ModulePath:      modulePath,
				LocalModulePath: localModulePath,
				DiscoverySource: "github",
			})
		}

		if resp.NextPage == 0 {
			break
		}
		options.Page = resp.NextPage
	}

	return dependents, nil
}

// performMultiSourceDiscovery performs discovery from multiple sources and merges the results.
// This implements Task 3.3: Result Merging & Conflict Resolution.
func performMultiSourceDiscovery(ctx context.Context, targetModule, githubOrg, workspace string, maxDepth int,
	includePatterns, excludePatterns, githubIncludePatterns, githubExcludePatterns []string,
	cfg *config.Config, logger di.Logger) ([]manifest.DependentOptions, error) {

	var githubDependents []manifest.DependentOptions
	var workspaceDependents []manifest.DependentOptions
	var discoveryErrors []error

	// Step 1: Attempt GitHub discovery if organization is specified
	finalGitHubOrg := resolveGitHubOrg(githubOrg, cfg)
	shouldRunGitHub := finalGitHubOrg != ""
	if githubOrg == "" && cfg != nil && !cfg.ManifestGenerator.Discovery.GitHub.Enabled {
		shouldRunGitHub = false
	}
	if shouldRunGitHub {
		finalGitHubInclude := getGitHubIncludePatterns(githubIncludePatterns, cfg)
		finalGitHubExclude := getGitHubExcludePatterns(githubExcludePatterns, cfg)

		if logger != nil {
			logger.Info("Attempting GitHub discovery", "organization", finalGitHubOrg)
		}

		ghDeps, err := discoverGitHubDependents(ctx, targetModule, finalGitHubOrg,
			finalGitHubInclude, finalGitHubExclude, cfg, logger)
		if err != nil {
			discoveryErrors = append(discoveryErrors, fmt.Errorf("GitHub discovery failed: %w", err))
			if logger != nil {
				logger.Warn("GitHub discovery failed", "error", err)
			}
		} else {
			githubDependents = ghDeps
			if logger != nil && len(githubDependents) > 0 {
				logger.Info("GitHub discovery completed",
					"organization", finalGitHubOrg,
					"found_dependents", len(githubDependents))
			}
		}
	}

	// Step 2: Attempt workspace discovery
	workspaceDir := resolveWorkspaceDir(workspace, cfg)
	if workspaceDir != "" {
		if logger != nil {
			logger.Info("Attempting workspace discovery", "workspace", workspaceDir)
		}

		wsDeps, err := discoverWorkspaceDependents(ctx, targetModule, workspaceDir, maxDepth,
			includePatterns, excludePatterns, cfg, logger)
		if err != nil {
			discoveryErrors = append(discoveryErrors, fmt.Errorf("workspace discovery failed: %w", err))
			if logger != nil {
				logger.Warn("Workspace discovery failed", "error", err)
			}
		} else {
			workspaceDependents = wsDeps
			if logger != nil && len(workspaceDependents) > 0 {
				logger.Info("Workspace discovery completed",
					"workspace", workspaceDir,
					"found_dependents", len(workspaceDependents))
			}
		}
	}

	// Step 3: Merge and deduplicate results
	mergedDependents := mergeDiscoveryResults(githubDependents, workspaceDependents, logger)

	if len(mergedDependents) == 0 {
		if len(discoveryErrors) > 0 {
			// Return the first error if no results were found and errors occurred
			return nil, discoveryErrors[0]
		}
		if logger != nil {
			logger.Info("No dependent repositories discovered")
		}
	} else if logger != nil {
		logger.Info("Discovery results merged",
			"github_dependents", len(githubDependents),
			"workspace_dependents", len(workspaceDependents),
			"merged_total", len(mergedDependents))
	}

	return mergedDependents, nil
}

// mergeDiscoveryResults merges and deduplicates discovery results from multiple sources.
// Deduplication is based on repository name and module path pairs.
func mergeDiscoveryResults(githubDependents, workspaceDependents []manifest.DependentOptions, logger di.Logger) []manifest.DependentOptions {
	// Use a map to deduplicate based on repo+module pair
	dependentMap := make(map[string]manifest.DependentOptions)

	// Add workspace dependents first (they may have more accurate local paths)
	for _, dep := range workspaceDependents {
		key := dependentKey(dep.Repository, dep.ModulePath)
		dep.DiscoverySource = "workspace"
		dependentMap[key] = dep
	}

	// Add GitHub dependents, potentially overriding workspace entries
	conflictCount := 0
	for _, dep := range githubDependents {
		key := dependentKey(dep.Repository, dep.ModulePath)
		if existing, exists := dependentMap[key]; exists {
			// Conflict detected - merge the entries, preferring more complete information
			merged := mergeConflictingDependents(existing, dep, logger)
			dependentMap[key] = merged
			conflictCount++
		} else {
			dep.DiscoverySource = "github"
			dependentMap[key] = dep
		}
	}

	// Convert map back to slice
	result := make([]manifest.DependentOptions, 0, len(dependentMap))
	for _, dep := range dependentMap {
		result = append(result, dep)
	}

	if logger != nil && conflictCount > 0 {
		logger.Info("Resolved discovery conflicts",
			"conflicts", conflictCount,
			"final_count", len(result))
	}

	return result
}

// dependentKey creates a unique key for deduplication based on repository and module path.
func dependentKey(repository, modulePath string) string {
	return fmt.Sprintf("%s|%s", repository, modulePath)
}

// mergeConflictingDependents merges two DependentOptions that refer to the same repo/module.
// It prefers more complete information and logs the merge decisions.
func mergeConflictingDependents(existing, new manifest.DependentOptions, logger di.Logger) manifest.DependentOptions {
	merged := existing

	// Prefer non-empty local module paths (workspace discovery usually provides these)
	if merged.LocalModulePath == "." && new.LocalModulePath != "." && new.LocalModulePath != "" {
		merged.LocalModulePath = new.LocalModulePath
	}

	// Track both discovery sources
	if existing.DiscoverySource != "" && new.DiscoverySource != "" {
		merged.DiscoverySource = fmt.Sprintf("%s+%s", existing.DiscoverySource, new.DiscoverySource)
	} else if new.DiscoverySource != "" {
		merged.DiscoverySource = new.DiscoverySource
	}

	if logger != nil {
		logger.Debug("Merged conflicting dependents",
			"repository", merged.Repository,
			"module_path", merged.ModulePath,
			"sources", merged.DiscoverySource,
			"local_module_path", merged.LocalModulePath)
	}

	return merged
}

// promptForDependentSelection allows users to interactively select which discovered
// dependents to include in the manifest.
func promptForDependentSelection(dependents []manifest.DependentOptions) ([]manifest.DependentOptions, error) {
	if len(dependents) == 0 {
		return dependents, nil
	}

	fmt.Printf("\nDiscovered %d dependent repositories:\n\n", len(dependents))

	// Display the discovered dependents with indices
	for i, dep := range dependents {
		source := dep.DiscoverySource
		if source == "" {
			source = "unknown"
		}
		fmt.Printf("  %d. %s (module: %s, source: %s)\n", i+1, dep.Repository, dep.ModulePath, source)
		if dep.LocalModulePath != "." && dep.LocalModulePath != "" {
			fmt.Printf("     Local path: %s\n", dep.LocalModulePath)
		}
	}

	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  a - include all")
	fmt.Println("  n - include none")
	fmt.Println("  1,2,3 - include specific repositories by number")
	fmt.Println("  1-3,5 - include ranges and specific repositories")
	fmt.Print("\nSelect dependents to include [a]: ")

	var input string
	fmt.Scanln(&input)

	// Default to "all" if no input provided
	if input == "" || input == "a" || input == "all" {
		return dependents, nil
	}

	// Handle "none" case
	if input == "n" || input == "none" {
		return []manifest.DependentOptions{}, nil
	}

	// Parse selection indices
	selectedIndices, err := parseSelectionInput(input, len(dependents))
	if err != nil {
		return nil, fmt.Errorf("invalid selection: %w", err)
	}

	// Build result with selected dependents
	result := make([]manifest.DependentOptions, 0, len(selectedIndices))
	for _, index := range selectedIndices {
		result = append(result, dependents[index])
	}

	fmt.Printf("Selected %d dependents for inclusion.\n", len(result))
	return result, nil
}

// parseSelectionInput parses user input for dependent selection.
// Supports formats like "1,2,3", "1-3,5", etc.
func parseSelectionInput(input string, maxIndex int) ([]int, error) {
	if input == "" {
		return nil, fmt.Errorf("empty input")
	}

	var indices []int
	parts := strings.Split(input, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Handle range (e.g., "1-3")
		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				return nil, fmt.Errorf("invalid range format: %s", part)
			}

			start, err := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
			if err != nil {
				return nil, fmt.Errorf("invalid start number in range %s: %w", part, err)
			}

			end, err := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid end number in range %s: %w", part, err)
			}

			if start < 1 || end > maxIndex || start > end {
				return nil, fmt.Errorf("invalid range %s: must be between 1 and %d", part, maxIndex)
			}

			for i := start; i <= end; i++ {
				indices = append(indices, i-1) // Convert to 0-based indexing
			}
		} else {
			// Handle single number
			num, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid number: %s", part)
			}

			if num < 1 || num > maxIndex {
				return nil, fmt.Errorf("number %d out of range: must be between 1 and %d", num, maxIndex)
			}

			indices = append(indices, num-1) // Convert to 0-based indexing
		}
	}

	// Remove duplicates
	uniqueIndices := make([]int, 0, len(indices))
	seen := make(map[int]bool)
	for _, index := range indices {
		if !seen[index] {
			uniqueIndices = append(uniqueIndices, index)
			seen[index] = true
		}
	}

	return uniqueIndices, nil
}
