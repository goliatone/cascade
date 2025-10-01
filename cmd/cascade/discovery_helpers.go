package main

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/goliatone/cascade/internal/broker"
	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/pkg/config"
	"github.com/goliatone/cascade/pkg/di"
	gh "github.com/google/go-github/v66/github"
	oauth2 "golang.org/x/oauth2"
)

func performMultiSourceDiscovery(ctx context.Context, targetModule, targetVersion, githubOrg, workspace string, maxDepth int,
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

	workspaceDir := workspace
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

func newGitHubClient(ctx context.Context, cfg *config.Config) (*gh.Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration required for GitHub discovery")
	}

	token := strings.TrimSpace(cfg.Integration.GitHub.Token)
	if token == "" {
		envToken, err := broker.LoadGitHubToken()
		if err != nil {
			return nil, fmt.Errorf("failed to load GitHub token: %w", err)
		}
		token = strings.TrimSpace(envToken)
		if token == "" {
			return nil, fmt.Errorf("GitHub token is empty after loading from environment")
		}
	}

	httpClient := oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}))

	endpoint := strings.TrimSpace(cfg.Integration.GitHub.Endpoint)
	if endpoint == "" {
		return gh.NewClient(httpClient), nil
	}

	baseURL, uploadURL := normalizeEnterpriseEndpoints(endpoint)
	client, err := gh.NewEnterpriseClient(baseURL, uploadURL, httpClient)
	if err != nil {
		return nil, fmt.Errorf("create github enterprise client: %w", err)
	}
	return client, nil
}

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

func matchesRepoPatterns(fullName string, includePatterns, excludePatterns []string) bool {
	repoLower := strings.ToLower(fullName)
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

func fetchModuleInfoFromGitHub(ctx context.Context, client *gh.Client, repo *gh.Repository, goModPath string) (string, string, error) {
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()

	file, _, resp, err := client.Repositories.GetContents(ctx, owner, name, goModPath, nil)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
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
