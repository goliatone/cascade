package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	execpkg "github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/internal/state"
	"github.com/goliatone/cascade/pkg/config"
	"github.com/goliatone/cascade/pkg/di"
	"github.com/goliatone/cascade/pkg/util/modpath"
	workspacepkg "github.com/goliatone/cascade/pkg/workspace"
	gh "github.com/google/go-github/v66/github"
	oauth2 "golang.org/x/oauth2"
)

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

func detectDefaultVersion(ctx context.Context, moduleDir string) (string, []string) {
	var warnings []string

	if strings.TrimSpace(moduleDir) == "" {
		return "", warnings
	}

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

func deriveModuleName(modulePath string) string {
	if modulePath == "" {
		return ""
	}
	parts := strings.Split(modulePath, "/")
	return parts[len(parts)-1]
}

func deriveRepository(modulePath string) string { return modpath.DeriveRepository(modulePath) }

func deriveGitHubOrgFromModule(modulePath string) string {
	parts := strings.Split(modulePath, "/")
	if len(parts) >= 3 && parts[0] == "github.com" {
		return parts[1]
	}
	return ""
}

func deriveLocalModulePath(modulePath string) string {
	return modpath.DeriveLocalModulePath(modulePath)
}

func buildCloneURL(repo string) string { return modpath.BuildCloneURL(repo) }

func resolveVersionFromWorkspace(ctx context.Context, modulePath, version, workspaceDir string, logger di.Logger) (string, []string, error) {
	discovery := manifest.NewWorkspaceDiscovery()

	var strategy manifest.VersionResolutionStrategy
	allowNetwork := true

	if version == "latest" {
		strategy = manifest.VersionResolutionLatest
	} else {
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

	if logger != nil {
		logger.Info("Version resolved",
			"module", modulePath,
			"version", resolution.Version,
			"source", string(resolution.Source),
			"source_path", resolution.SourcePath)
	}

	return resolution.Version, resolution.Warnings, nil
}

func performMultiSourceDiscovery(ctx context.Context, targetModule, targetVersion, githubOrg, workDir string, maxDepth int,
	includePatterns, excludePatterns, githubIncludePatterns, githubExcludePatterns []string,
	cfg *config.Config, logger di.Logger) ([]manifest.DependentOptions, error) {

	var githubDependents []manifest.DependentOptions
	var workspaceDependents []manifest.DependentOptions
	var discoveryErrors []error

	finalGitHubOrg := resolveGitHubOrg(githubOrg, cfg)
	shouldRunGitHub := finalGitHubOrg != ""
	if githubOrg == "" && cfg != nil && !cfg.ManifestGenerator.Discovery.GitHub.Enabled {
		shouldRunGitHub = false
	}
	if shouldRunGitHub {
		finalGitHubInclude := githubIncludePatterns
		if len(finalGitHubInclude) == 0 {
			if patterns := config.GitHubDiscoveryIncludePatterns(cfg); len(patterns) > 0 {
				finalGitHubInclude = patterns
			}
		}
		finalGitHubExclude := githubExcludePatterns
		if len(finalGitHubExclude) == 0 {
			if patterns := config.GitHubDiscoveryExcludePatterns(cfg); len(patterns) > 0 {
				finalGitHubExclude = patterns
			}
		}

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

	workspaceDir := workDir
	if workspaceDir != "" {
		if logger != nil {
			logger.Info("Attempting workspace discovery", "workspace", workspaceDir)
		}

		wsDeps, err := discoverWorkspaceDependents(ctx, targetModule, targetVersion, workspaceDir, maxDepth,
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

	mergedDependents := mergeDiscoveryResults(githubDependents, workspaceDependents, logger)

	if len(mergedDependents) == 0 {
		if len(discoveryErrors) > 0 {
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

func discoverWorkspaceDependents(ctx context.Context, targetModule, targetVersion, workspaceDir string, maxDepth int,
	includePatterns, excludePatterns []string, cfg *config.Config, logger di.Logger) ([]manifest.DependentOptions, error) {
	discovery := manifest.NewWorkspaceDiscovery()

	finalMaxDepth := maxDepth
	if finalMaxDepth <= 0 {
		finalMaxDepth = config.DiscoveryMaxDepth(cfg)
	}
	finalIncludePatterns := includePatterns
	if len(finalIncludePatterns) == 0 {
		if patterns := config.DiscoveryIncludePatterns(cfg); len(patterns) > 0 {
			finalIncludePatterns = patterns
		}
	}
	finalExcludePatterns := excludePatterns
	if len(finalExcludePatterns) == 0 {
		finalExcludePatterns = config.DiscoveryExcludePatterns(cfg)
	}

	options := manifest.DiscoveryOptions{
		WorkspaceDir:    workspaceDir,
		TargetModule:    targetModule,
		TargetVersion:   targetVersion,
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

func mergeDiscoveryResults(githubDependents, workspaceDependents []manifest.DependentOptions, logger di.Logger) []manifest.DependentOptions {
	dependentMap := make(map[string]manifest.DependentOptions)

	for _, dep := range workspaceDependents {
		key := dependentKey(dep.Repository, dep.ModulePath)
		dep.DiscoverySource = "workspace"
		dependentMap[key] = dep
	}

	conflictCount := 0
	for _, dep := range githubDependents {
		key := dependentKey(dep.Repository, dep.ModulePath)
		if existing, exists := dependentMap[key]; exists {
			merged := mergeConflictingDependents(existing, dep, logger)
			dependentMap[key] = merged
			conflictCount++
		} else {
			dep.DiscoverySource = "github"
			dependentMap[key] = dep
		}
	}

	if logger != nil && conflictCount > 0 {
		logger.Debug("Resolved discovery conflicts", "count", conflictCount)
	}

	merged := make([]manifest.DependentOptions, 0, len(dependentMap))
	for _, dep := range dependentMap {
		merged = append(merged, dep)
	}

	return merged
}

func mergeConflictingDependents(existing, incoming manifest.DependentOptions, logger di.Logger) manifest.DependentOptions {
	merged := existing

	if incoming.CloneURL != "" && merged.CloneURL == "" {
		merged.CloneURL = incoming.CloneURL
	}

	if incoming.LocalModulePath != "" && incoming.LocalModulePath != "." && merged.LocalModulePath == "." {
		merged.LocalModulePath = incoming.LocalModulePath
	}

	if incoming.DiscoverySource != "" {
		merged.DiscoverySource = incoming.DiscoverySource
	}

	return merged
}

func dependentKey(repo, module string) string {
	return repo + "|" + module
}

func matchesRepoPatterns(fullName string, includePatterns, excludePatterns []string) bool {
	for _, pattern := range excludePatterns {
		if pattern != "" && strings.Contains(fullName, pattern) {
			return false
		}
	}

	if len(includePatterns) == 0 {
		return true
	}

	for _, pattern := range includePatterns {
		if pattern != "" && strings.Contains(fullName, pattern) {
			return true
		}
	}

	return false
}

func resolveGitHubOrg(githubOrg string, cfg *config.Config) string {
	if strings.TrimSpace(githubOrg) != "" {
		return strings.TrimSpace(githubOrg)
	}

	if cfg != nil && cfg.ManifestGenerator.Discovery.GitHub.Organization != "" {
		return cfg.ManifestGenerator.Discovery.GitHub.Organization
	}

	if cfg != nil && cfg.Integration.GitHub.Organization != "" {
		return cfg.Integration.GitHub.Organization
	}

	return ""
}

func discoverGitHubDependentsForOrg(ctx context.Context, targetModule, organization string, includePatterns, excludePatterns []string, cfg *config.Config, logger di.Logger) ([]manifest.DependentOptions, error) {
	return discoverGitHubDependents(ctx, targetModule, organization, includePatterns, excludePatterns, cfg, logger)
}

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
		return nil, err
	}

	return client, nil
}

func promptForDependentSelection(dependents []manifest.DependentOptions) ([]manifest.DependentOptions, error) {
	if len(dependents) == 0 {
		return dependents, nil
	}

	fmt.Printf("\nDiscovered %d dependent repositories:\n\n", len(dependents))

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

	if input == "" || input == "a" || input == "all" {
		return dependents, nil
	}

	if input == "n" || input == "none" {
		return []manifest.DependentOptions{}, nil
	}

	selectedIndices, err := parseSelectionInput(input, len(dependents))
	if err != nil {
		return nil, fmt.Errorf("invalid selection: %w", err)
	}

	result := make([]manifest.DependentOptions, 0, len(selectedIndices))
	for _, index := range selectedIndices {
		result = append(result, dependents[index])
	}

	fmt.Printf("Selected %d dependents for inclusion.\n", len(result))
	return result, nil
}

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
				indices = append(indices, i-1)
			}
		} else {
			num, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid number: %s", part)
			}

			if num < 1 || num > maxIndex {
				return nil, fmt.Errorf("number %d out of range: must be between 1 and %d", num, maxIndex)
			}

			indices = append(indices, num-1)
		}
	}

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
