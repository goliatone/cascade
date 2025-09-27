package manifest

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/google/go-github/v66/github"
	"golang.org/x/oauth2"
)

// GitHubDiscovery provides functionality to discover Go modules and their dependencies
// across a GitHub organization using the GitHub API.
type GitHubDiscovery interface {
	// DiscoverDependents finds repositories in the GitHub organization that depend on the target module.
	DiscoverDependents(ctx context.Context, options GitHubDiscoveryOptions) ([]DependentOptions, error)

	// ResolveVersion attempts to resolve the latest version of a module using GitHub API.
	ResolveVersion(ctx context.Context, options GitHubVersionResolutionOptions) (*VersionResolution, error)
}

// GitHubDiscoveryOptions configures GitHub discovery behavior.
type GitHubDiscoveryOptions struct {
	// Organization is the GitHub organization to search within
	Organization string

	// TargetModule is the module path we're looking for dependents of
	TargetModule string

	// IncludePatterns specifies repository name patterns to include (empty = include all)
	IncludePatterns []string

	// ExcludePatterns specifies repository name patterns to exclude
	ExcludePatterns []string

	// MaxResults limits the number of repositories to search (0 = no limit)
	MaxResults int

	// SearchQuery allows custom GitHub search query modifications
	SearchQuery string
}

// GitHubVersionResolutionOptions configures GitHub-based version resolution.
type GitHubVersionResolutionOptions struct {
	// Repository is the GitHub repository (owner/repo format)
	Repository string

	// TargetModule is the module path we're trying to resolve the version for
	TargetModule string

	// Strategy determines how to resolve the version
	Strategy GitHubVersionResolutionStrategy

	// UseProxy indicates whether to try Go module proxy first
	UseProxy bool
}

// GitHubVersionResolutionStrategy defines how to resolve module versions using GitHub.
type GitHubVersionResolutionStrategy string

const (
	// GitHubVersionResolutionTags resolves version from Git tags
	GitHubVersionResolutionTags GitHubVersionResolutionStrategy = "tags"

	// GitHubVersionResolutionProxy tries Go module proxy, then falls back to tags
	GitHubVersionResolutionProxy GitHubVersionResolutionStrategy = "proxy"
)

// GitHubDiscoveredRepository represents a repository found during GitHub discovery.
type GitHubDiscoveredRepository struct {
	Owner         string // Repository owner
	Name          string // Repository name
	FullName      string // Full repository name (owner/repo)
	DefaultBranch string // Default branch name
	ModulePath    string // Inferred Go module path
	Language      string // Primary language
	Private       bool   // Whether the repository is private
}

// NewGitHubDiscovery creates a new GitHub discovery instance.
func NewGitHubDiscovery(client *github.Client) GitHubDiscovery {
	return &gitHubDiscovery{
		client: client,
	}
}

// GitHubAuthConfig holds authentication configuration options for GitHub discovery.
type GitHubAuthConfig struct {
	// Token is the GitHub personal access token or OAuth token
	Token string
	// BaseURL is the GitHub API base URL (for GitHub Enterprise)
	BaseURL string
	// UploadURL is the GitHub upload URL (for GitHub Enterprise)
	UploadURL string
	// InsecureSkipVerify skips TLS verification (for self-signed certificates)
	InsecureSkipVerify bool
}

// NewGitHubDiscoveryFromToken creates a new GitHub discovery instance with authentication.
func NewGitHubDiscoveryFromToken(token string) (GitHubDiscovery, error) {
	if token == "" {
		loadedToken, err := loadGitHubToken()
		if err != nil {
			return nil, fmt.Errorf("failed to load GitHub token: %w", err)
		}
		token = loadedToken
	}

	authConfig := GitHubAuthConfig{
		Token: token,
	}

	client, err := createAuthenticatedClient(authConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create authenticated GitHub client: %w", err)
	}

	return &gitHubDiscovery{
		client: client,
	}, nil
}

// NewGitHubDiscoveryFromConfig creates a new GitHub discovery instance with full auth configuration.
func NewGitHubDiscoveryFromConfig(config GitHubAuthConfig) (GitHubDiscovery, error) {
	client, err := createAuthenticatedClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create authenticated GitHub client: %w", err)
	}

	return &gitHubDiscovery{
		client: client,
	}, nil
}

// loadGitHubToken loads a GitHub token from environment variables.
// It checks multiple environment variables in order of precedence:
// 1. GITHUB_TOKEN
// 2. GITHUB_ACCESS_TOKEN
// 3. GH_TOKEN
func loadGitHubToken() (string, error) {
	envVars := []string{"GITHUB_TOKEN", "GITHUB_ACCESS_TOKEN", "GH_TOKEN"}

	for _, envVar := range envVars {
		if token := os.Getenv(envVar); token != "" {
			return strings.TrimSpace(token), nil
		}
	}

	return "", fmt.Errorf("GitHub token not found: set one of %v environment variables", envVars)
}

// createAuthenticatedClient creates a GitHub client with the given token and configuration.
func createAuthenticatedClient(config GitHubAuthConfig) (*github.Client, error) {
	if config.Token == "" {
		return nil, fmt.Errorf("GitHub token is required")
	}

	// Create OAuth2 token source
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: config.Token},
	)

	// Create HTTP client with OAuth2 transport
	httpClient := oauth2.NewClient(context.Background(), ts)

	// Configure TLS settings if needed
	if config.InsecureSkipVerify {
		transport := httpClient.Transport.(*oauth2.Transport)
		if transport.Base == nil {
			transport.Base = http.DefaultTransport
		}

		if baseTransport, ok := transport.Base.(*http.Transport); ok {
			baseTransport.TLSClientConfig = &tls.Config{
				InsecureSkipVerify: true,
			}
		}
	}

	var client *github.Client

	// Create GitHub client with custom base URL if specified (GitHub Enterprise)
	if config.BaseURL != "" {
		var err error
		client, err = github.NewClient(httpClient).WithEnterpriseURLs(config.BaseURL, config.UploadURL)
		if err != nil {
			return nil, fmt.Errorf("failed to create GitHub Enterprise client: %w", err)
		}
	} else {
		client = github.NewClient(httpClient)
	}

	return client, nil
}

type gitHubDiscovery struct {
	client *github.Client
}

// ValidateAuthentication validates that the GitHub client can authenticate successfully.
func (g *gitHubDiscovery) ValidateAuthentication(ctx context.Context) error {
	if g.client == nil {
		return fmt.Errorf("GitHub client is nil")
	}

	// Test authentication by getting the authenticated user
	user, resp, err := g.client.Users.Get(ctx, "")
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("GitHub authentication failed: invalid or expired token")
		}
		return fmt.Errorf("GitHub authentication validation failed: %w", err)
	}

	if user == nil || user.Login == nil {
		return fmt.Errorf("GitHub authentication succeeded but user information is unavailable")
	}

	return nil
}

// CheckRateLimit checks the current GitHub API rate limit and returns a warning if it's low.
func (g *gitHubDiscovery) CheckRateLimit(ctx context.Context) error {
	if g.client == nil {
		return fmt.Errorf("GitHub client is nil")
	}

	rateLimits, _, err := g.client.RateLimits(ctx)
	if err != nil {
		return fmt.Errorf("failed to get GitHub API rate limits: %w", err)
	}

	if rateLimits.Core != nil && isRateLimitCritical(rateLimits.Core) {
		return fmt.Errorf("GitHub API rate limit critically low: %d/%d remaining (resets at %v)",
			rateLimits.Core.Remaining,
			rateLimits.Core.Limit,
			rateLimits.Core.Reset.Time)
	}

	return nil
}

// isRateLimitCritical checks if the rate limit is critically low (< 10% remaining).
func isRateLimitCritical(rate *github.Rate) bool {
	if rate == nil {
		return false
	}

	threshold := float64(rate.Limit) * 0.10 // 10% threshold
	return float64(rate.Remaining) < threshold
}

// DiscoverDependents searches GitHub for repositories that depend on the target module.
func (g *gitHubDiscovery) DiscoverDependents(ctx context.Context, options GitHubDiscoveryOptions) ([]DependentOptions, error) {
	if options.Organization == "" {
		return nil, fmt.Errorf("GitHub organization is required")
	}
	if options.TargetModule == "" {
		return nil, fmt.Errorf("target module is required")
	}

	repos, err := g.searchRepositories(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("failed to search repositories: %w", err)
	}

	var dependents []DependentOptions

	for _, repo := range repos {
		// Check if the repository actually uses Go and has go.mod files
		hasDependency, err := g.repositoryHasDependency(ctx, repo, options.TargetModule)
		if err != nil {
			// Log warning but continue with other repositories
			continue
		}

		if hasDependency {
			dependent := DependentOptions{
				Repository:      repo.FullName,
				ModulePath:      repo.ModulePath,
				LocalModulePath: g.inferLocalModulePath(repo.ModulePath),
			}
			dependents = append(dependents, dependent)
		}
	}

	return dependents, nil
}

// ResolveVersion attempts to resolve the latest version of a module using GitHub API.
func (g *gitHubDiscovery) ResolveVersion(ctx context.Context, options GitHubVersionResolutionOptions) (*VersionResolution, error) {
	if options.Repository == "" {
		return nil, fmt.Errorf("repository is required")
	}
	if options.TargetModule == "" {
		return nil, fmt.Errorf("target module is required")
	}

	resolution := &VersionResolution{
		Warnings: []string{},
	}

	switch options.Strategy {
	case GitHubVersionResolutionTags:
		return g.resolveVersionFromTags(ctx, options.Repository, resolution)
	case GitHubVersionResolutionProxy:
		// Try proxy first if requested
		if options.UseProxy {
			// This would delegate to the existing workspace discovery proxy resolution
			// For now, fall back to tags
			resolution.Warnings = append(resolution.Warnings, "Proxy resolution not implemented, falling back to Git tags")
		}
		return g.resolveVersionFromTags(ctx, options.Repository, resolution)
	default:
		return nil, fmt.Errorf("unsupported GitHub version resolution strategy: %s", options.Strategy)
	}
}

// searchRepositories searches for repositories in the GitHub organization.
func (g *gitHubDiscovery) searchRepositories(ctx context.Context, options GitHubDiscoveryOptions) ([]GitHubDiscoveredRepository, error) {
	var allRepos []GitHubDiscoveredRepository

	// Build search query
	query := g.buildSearchQuery(options)

	// Search for repositories using GitHub search API
	searchOpts := &github.SearchOptions{
		Sort:  "updated",
		Order: "desc",
		ListOptions: github.ListOptions{
			PerPage: 100, // Maximum per page
		},
	}

	for {
		result, resp, err := g.client.Search.Repositories(ctx, query, searchOpts)
		if err != nil {
			return nil, fmt.Errorf("GitHub repository search failed: %w", err)
		}

		for _, repo := range result.Repositories {
			if g.shouldIncludeRepository(repo, options) {
				discoveredRepo := GitHubDiscoveredRepository{
					Owner:         repo.GetOwner().GetLogin(),
					Name:          repo.GetName(),
					FullName:      repo.GetFullName(),
					DefaultBranch: repo.GetDefaultBranch(),
					ModulePath:    g.inferModulePath(repo.GetFullName()),
					Language:      repo.GetLanguage(),
					Private:       repo.GetPrivate(),
				}
				allRepos = append(allRepos, discoveredRepo)

				// Check max results limit
				if options.MaxResults > 0 && len(allRepos) >= options.MaxResults {
					return allRepos[:options.MaxResults], nil
				}
			}
		}

		// Check if there are more pages
		if resp.NextPage == 0 {
			break
		}
		searchOpts.Page = resp.NextPage
	}

	return allRepos, nil
}

// buildSearchQuery constructs a GitHub search query based on the options.
func (g *gitHubDiscovery) buildSearchQuery(options GitHubDiscoveryOptions) string {
	query := []string{}

	// If a custom search query is provided, use it as the base
	if options.SearchQuery != "" {
		query = append(query, options.SearchQuery)
	} else {
		// Default search for Go repositories that might contain go.mod files
		query = append(query, "language:go")
		query = append(query, "filename:go.mod")
	}

	// Add organization filter
	query = append(query, "org:"+options.Organization)

	return strings.Join(query, " ")
}

// shouldIncludeRepository checks if a repository should be included based on patterns.
func (g *gitHubDiscovery) shouldIncludeRepository(repo *github.Repository, options GitHubDiscoveryOptions) bool {
	repoName := repo.GetName()

	// Check exclude patterns first
	for _, pattern := range options.ExcludePatterns {
		if g.matchPattern(pattern, repoName) {
			return false
		}
	}

	// If no include patterns specified, include by default
	if len(options.IncludePatterns) == 0 {
		return true
	}

	// Check include patterns
	for _, pattern := range options.IncludePatterns {
		if g.matchPattern(pattern, repoName) {
			return true
		}
	}

	return false
}

// matchPattern performs simple pattern matching (supports * wildcard).
func (g *gitHubDiscovery) matchPattern(pattern, text string) bool {
	// Simple wildcard matching - could be enhanced with proper glob matching
	if pattern == "*" {
		return true
	}
	if strings.Contains(pattern, "*") {
		// Basic wildcard support
		parts := strings.Split(pattern, "*")
		if len(parts) == 2 {
			return strings.HasPrefix(text, parts[0]) && strings.HasSuffix(text, parts[1])
		}
	}
	return pattern == text
}

// repositoryHasDependency checks if a GitHub repository depends on the target module.
func (g *gitHubDiscovery) repositoryHasDependency(ctx context.Context, repo GitHubDiscoveredRepository, targetModule string) (bool, error) {
	// Search for go.mod files in the repository
	query := fmt.Sprintf("filename:go.mod repo:%s", repo.FullName)

	searchOpts := &github.SearchOptions{
		ListOptions: github.ListOptions{
			PerPage: 10, // Limit to first 10 go.mod files
		},
	}

	result, _, err := g.client.Search.Code(ctx, query, searchOpts)
	if err != nil {
		return false, fmt.Errorf("failed to search for go.mod files in %s: %w", repo.FullName, err)
	}

	// Check each go.mod file for the target dependency
	for _, codeResult := range result.CodeResults {
		hasDep, err := g.checkGoModFileForDependency(ctx, repo, codeResult, targetModule)
		if err != nil {
			continue // Skip files we can't read
		}
		if hasDep {
			return true, nil
		}
	}

	return false, nil
}

// checkGoModFileForDependency checks a specific go.mod file for the target dependency.
func (g *gitHubDiscovery) checkGoModFileForDependency(ctx context.Context, repo GitHubDiscoveredRepository, codeResult *github.CodeResult, targetModule string) (bool, error) {
	// Get the content of the go.mod file
	content, _, _, err := g.client.Repositories.GetContents(ctx, repo.Owner, repo.Name, codeResult.GetPath(), &github.RepositoryContentGetOptions{
		Ref: repo.DefaultBranch,
	})
	if err != nil {
		return false, fmt.Errorf("failed to get go.mod content: %w", err)
	}

	if content == nil {
		return false, fmt.Errorf("go.mod file content is nil")
	}

	// Decode the content
	fileContent, err := content.GetContent()
	if err != nil {
		return false, fmt.Errorf("failed to decode go.mod content: %w", err)
	}

	// Simple text search for the target module
	return strings.Contains(fileContent, targetModule), nil
}

// inferModulePath attempts to infer the Go module path from a GitHub repository full name.
func (g *gitHubDiscovery) inferModulePath(repoFullName string) string {
	// For GitHub repositories, the module path is typically github.com/owner/repo
	return "github.com/" + repoFullName
}

// inferLocalModulePath calculates the relative path from repository root to module.
func (g *gitHubDiscovery) inferLocalModulePath(modulePath string) string {
	// For GitHub repos discovered via API, assume the module is at the root
	// This could be enhanced to detect submodules in future versions
	return "."
}

// resolveVersionFromTags resolves the latest version by examining Git tags.
func (g *gitHubDiscovery) resolveVersionFromTags(ctx context.Context, repository string, resolution *VersionResolution) (*VersionResolution, error) {
	parts := strings.Split(repository, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repository format: expected owner/repo, got %s", repository)
	}
	owner, repo := parts[0], parts[1]

	// List tags for the repository
	tags, _, err := g.client.Repositories.ListTags(ctx, owner, repo, &github.ListOptions{
		PerPage: 100, // Get up to 100 tags
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list tags for %s: %w", repository, err)
	}

	if len(tags) == 0 {
		return nil, fmt.Errorf("no tags found for repository %s", repository)
	}

	// Find the latest semantic version tag
	var latestVersion string
	for _, tag := range tags {
		tagName := tag.GetName()
		// Simple semantic version detection (starts with 'v' followed by digits)
		if strings.HasPrefix(tagName, "v") && len(tagName) > 1 {
			if latestVersion == "" || g.isNewerVersion(tagName, latestVersion) {
				latestVersion = tagName
			}
		}
	}

	if latestVersion == "" {
		return nil, fmt.Errorf("no semantic version tags found for repository %s", repository)
	}

	resolution.Version = latestVersion
	resolution.Source = VersionSourceNetwork
	return resolution, nil
}

// isNewerVersion performs a simple version comparison.
// This is a basic implementation - could be enhanced with proper semver comparison.
func (g *gitHubDiscovery) isNewerVersion(version1, version2 string) bool {
	// Simple string comparison for now
	// In a production implementation, this should use proper semver comparison
	return version1 > version2
}
