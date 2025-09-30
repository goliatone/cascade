package planner

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/goliatone/cascade/internal/manifest"
)

// TestCheckStrategy verifies CheckStrategy enum values and behavior.
func TestCheckStrategy(t *testing.T) {
	tests := []struct {
		name     string
		strategy CheckStrategy
		want     string
	}{
		{
			name:     "local strategy",
			strategy: CheckStrategyLocal,
			want:     "local",
		},
		{
			name:     "remote strategy",
			strategy: CheckStrategyRemote,
			want:     "remote",
		},
		{
			name:     "auto strategy",
			strategy: CheckStrategyAuto,
			want:     "auto",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.strategy) != tt.want {
				t.Errorf("CheckStrategy = %v, want %v", tt.strategy, tt.want)
			}
		})
	}
}

// TestCheckOptions verifies default values and structure.
func TestCheckOptions(t *testing.T) {
	tests := []struct {
		name string
		opts CheckOptions
		want CheckOptions
	}{
		{
			name: "all fields populated",
			opts: CheckOptions{
				Strategy:       CheckStrategyAuto,
				CacheEnabled:   true,
				CacheTTL:       5 * time.Minute,
				ParallelChecks: 4,
				ShallowClone:   true,
				Timeout:        30 * time.Second,
			},
			want: CheckOptions{
				Strategy:       CheckStrategyAuto,
				CacheEnabled:   true,
				CacheTTL:       5 * time.Minute,
				ParallelChecks: 4,
				ShallowClone:   true,
				Timeout:        30 * time.Second,
			},
		},
		{
			name: "zero values",
			opts: CheckOptions{},
			want: CheckOptions{
				Strategy:       "",
				CacheEnabled:   false,
				CacheTTL:       0,
				ParallelChecks: 0,
				ShallowClone:   false,
				Timeout:        0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.opts.Strategy != tt.want.Strategy {
				t.Errorf("Strategy = %v, want %v", tt.opts.Strategy, tt.want.Strategy)
			}
			if tt.opts.CacheEnabled != tt.want.CacheEnabled {
				t.Errorf("CacheEnabled = %v, want %v", tt.opts.CacheEnabled, tt.want.CacheEnabled)
			}
			if tt.opts.CacheTTL != tt.want.CacheTTL {
				t.Errorf("CacheTTL = %v, want %v", tt.opts.CacheTTL, tt.want.CacheTTL)
			}
			if tt.opts.ParallelChecks != tt.want.ParallelChecks {
				t.Errorf("ParallelChecks = %v, want %v", tt.opts.ParallelChecks, tt.want.ParallelChecks)
			}
			if tt.opts.ShallowClone != tt.want.ShallowClone {
				t.Errorf("ShallowClone = %v, want %v", tt.opts.ShallowClone, tt.want.ShallowClone)
			}
			if tt.opts.Timeout != tt.want.Timeout {
				t.Errorf("Timeout = %v, want %v", tt.opts.Timeout, tt.want.Timeout)
			}
		})
	}
}

// TestCacheKey verifies cache key generation and collision avoidance.
func TestCacheKey(t *testing.T) {
	tests := []struct {
		name      string
		key1      cacheKey
		key2      cacheKey
		wantEqual bool
	}{
		{
			name: "identical keys",
			key1: cacheKey{
				cloneURL: "https://github.com/user/repo.git",
				ref:      "main",
			},
			key2: cacheKey{
				cloneURL: "https://github.com/user/repo.git",
				ref:      "main",
			},
			wantEqual: true,
		},
		{
			name: "different URLs",
			key1: cacheKey{
				cloneURL: "https://github.com/user/repo1.git",
				ref:      "main",
			},
			key2: cacheKey{
				cloneURL: "https://github.com/user/repo2.git",
				ref:      "main",
			},
			wantEqual: false,
		},
		{
			name: "different refs",
			key1: cacheKey{
				cloneURL: "https://github.com/user/repo.git",
				ref:      "main",
			},
			key2: cacheKey{
				cloneURL: "https://github.com/user/repo.git",
				ref:      "develop",
			},
			wantEqual: false,
		},
		{
			name: "case sensitive URLs",
			key1: cacheKey{
				cloneURL: "https://github.com/User/Repo.git",
				ref:      "main",
			},
			key2: cacheKey{
				cloneURL: "https://github.com/user/repo.git",
				ref:      "main",
			},
			wantEqual: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			equal := tt.key1 == tt.key2
			if equal != tt.wantEqual {
				t.Errorf("key equality = %v, want %v", equal, tt.wantEqual)
			}
		})
	}
}

// TestCacheEntry verifies cache entry structure and TTL support.
func TestCacheEntry(t *testing.T) {
	now := time.Now()
	entry := cacheEntry{
		goModPath: "go.mod",
		dependencies: map[string]string{
			"github.com/user/dep1": "v1.0.0",
			"github.com/user/dep2": "v2.0.0",
		},
		cachedAt: now,
		ttl:      5 * time.Minute,
	}

	if entry.goModPath != "go.mod" {
		t.Errorf("goModPath = %v, want go.mod", entry.goModPath)
	}

	if len(entry.dependencies) != 2 {
		t.Errorf("dependencies count = %v, want 2", len(entry.dependencies))
	}

	if entry.dependencies["github.com/user/dep1"] != "v1.0.0" {
		t.Errorf("dependency version = %v, want v1.0.0", entry.dependencies["github.com/user/dep1"])
	}

	if !entry.cachedAt.Equal(now) {
		t.Errorf("cachedAt = %v, want %v", entry.cachedAt, now)
	}

	if entry.ttl != 5*time.Minute {
		t.Errorf("ttl = %v, want 5m", entry.ttl)
	}
}

// TestRemoteDependencyCheckerInterface verifies the interface is properly defined.
func TestRemoteDependencyCheckerInterface(t *testing.T) {
	// This test ensures the interface compiles and can be used in type assertions
	var _ RemoteDependencyChecker = (*mockRemoteChecker)(nil)
}

// mockRemoteChecker is a test double for RemoteDependencyChecker.
type mockRemoteChecker struct {
	DependencyChecker
}

func (m *mockRemoteChecker) Warm(ctx context.Context, dependents []manifest.Dependent) error {
	return nil
}

func (m *mockRemoteChecker) ClearCache() error {
	return nil
}

// defaultParseCloneURL is the default implementation for parseCloneURL in tests
func defaultParseCloneURL(dependent manifest.Dependent) (string, error) {
	return "https://github.com/" + dependent.Repo + ".git", nil
}

func TestRemoteDependencyChecker_NeedsUpdate_CacheMiss(t *testing.T) {
	// Setup mock git operations
	mockGit := &mockGitOperations{
		parseCloneURLFunc: func(dependent manifest.Dependent) (string, error) {
			return "https://github.com/" + dependent.Repo + ".git", nil
		},
		fetchGoModFunc: func(ctx context.Context, cloneURL, ref string) (string, error) {
			// Return a go.mod with github.com/goliatone/go-errors at v0.8.0
			return `module github.com/goliatone/go-crud

go 1.21

require (
	github.com/goliatone/go-errors v0.8.0
	github.com/stretchr/testify v1.8.0
)
`, nil
		},
	}

	// Create checker with mock
	checker := &remoteDependencyChecker{
		cache:  newDependencyCache(5 * time.Minute),
		gitOps: mockGit,
		logger: &mockLogger{},
		options: CheckOptions{
			CacheEnabled:   true,
			CacheTTL:       5 * time.Minute,
			ParallelChecks: 4,
			Timeout:        30 * time.Second,
		},
	}

	// Test: dependency needs update (v0.8.0 < v0.9.0)
	dependent := manifest.Dependent{
		Repo:   "goliatone/go-crud",
		Branch: "main",
	}
	target := Target{
		Module:  "github.com/goliatone/go-errors",
		Version: "v0.9.0",
	}

	needsUpdate, err := checker.NeedsUpdate(context.Background(), dependent, target, "")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !needsUpdate {
		t.Error("expected needsUpdate=true, got false")
	}

	// Verify cache was populated
	stats := checker.cache.Stats()
	if stats.Size != 1 {
		t.Errorf("expected cache size 1, got %d", stats.Size)
	}
}

func TestRemoteDependencyChecker_NeedsUpdate_CacheHit(t *testing.T) {
	fetchCount := 0
	mockGit := &mockGitOperations{
		parseCloneURLFunc: defaultParseCloneURL,
		fetchGoModFunc: func(ctx context.Context, cloneURL, ref string) (string, error) {
			fetchCount++
			return `module github.com/goliatone/go-crud

go 1.21

require (
	github.com/goliatone/go-errors v0.8.0
)
`, nil
		},
	}

	checker := &remoteDependencyChecker{
		cache:  newDependencyCache(5 * time.Minute),
		gitOps: mockGit,
		logger: &mockLogger{},
		options: CheckOptions{
			CacheEnabled:   true,
			CacheTTL:       5 * time.Minute,
			ParallelChecks: 4,
			Timeout:        30 * time.Second,
		},
	}

	dependent := manifest.Dependent{
		Repo:   "goliatone/go-crud",
		Branch: "main",
	}
	target := Target{
		Module:  "github.com/goliatone/go-errors",
		Version: "v0.9.0",
	}

	// First call - cache miss
	_, err := checker.NeedsUpdate(context.Background(), dependent, target, "")
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if fetchCount != 1 {
		t.Fatalf("expected 1 fetch, got %d", fetchCount)
	}

	// Second call - cache hit (should not fetch again)
	needsUpdate, err := checker.NeedsUpdate(context.Background(), dependent, target, "")
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if !needsUpdate {
		t.Error("expected needsUpdate=true")
	}
	if fetchCount != 1 {
		t.Errorf("expected 1 fetch total, got %d (cache not working)", fetchCount)
	}

	// Verify cache stats
	stats := checker.cache.Stats()
	if stats.Hits != 1 {
		t.Errorf("expected 1 cache hit, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("expected 1 cache miss, got %d", stats.Misses)
	}
}

func TestRemoteDependencyChecker_NeedsUpdate_DependencyNotPresent(t *testing.T) {
	mockGit := &mockGitOperations{
		parseCloneURLFunc: defaultParseCloneURL,
		fetchGoModFunc: func(ctx context.Context, cloneURL, ref string) (string, error) {
			// Return go.mod without the target dependency
			return `module github.com/goliatone/go-crud

go 1.21

require (
	github.com/stretchr/testify v1.8.0
)
`, nil
		},
	}

	checker := &remoteDependencyChecker{
		cache:  newDependencyCache(5 * time.Minute),
		gitOps: mockGit,
		logger: &mockLogger{},
		options: CheckOptions{
			CacheEnabled:   true,
			CacheTTL:       5 * time.Minute,
			ParallelChecks: 4,
			Timeout:        30 * time.Second,
		},
	}

	dependent := manifest.Dependent{
		Repo:   "goliatone/go-crud",
		Branch: "main",
	}
	target := Target{
		Module:  "github.com/goliatone/go-errors",
		Version: "v0.9.0",
	}

	needsUpdate, err := checker.NeedsUpdate(context.Background(), dependent, target, "")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if needsUpdate {
		t.Error("expected needsUpdate=false when dependency not present")
	}
}

func TestRemoteDependencyChecker_NeedsUpdate_AlreadyUpToDate(t *testing.T) {
	mockGit := &mockGitOperations{
		parseCloneURLFunc: defaultParseCloneURL,
		fetchGoModFunc: func(ctx context.Context, cloneURL, ref string) (string, error) {
			return `module github.com/goliatone/go-crud

go 1.21

require (
	github.com/goliatone/go-errors v0.9.0
)
`, nil
		},
	}

	checker := &remoteDependencyChecker{
		cache:  newDependencyCache(5 * time.Minute),
		gitOps: mockGit,
		logger: &mockLogger{},
		options: CheckOptions{
			CacheEnabled:   true,
			CacheTTL:       5 * time.Minute,
			ParallelChecks: 4,
			Timeout:        30 * time.Second,
		},
	}

	dependent := manifest.Dependent{
		Repo:   "goliatone/go-crud",
		Branch: "main",
	}
	target := Target{
		Module:  "github.com/goliatone/go-errors",
		Version: "v0.9.0",
	}

	needsUpdate, err := checker.NeedsUpdate(context.Background(), dependent, target, "")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if needsUpdate {
		t.Error("expected needsUpdate=false when already up-to-date")
	}
}

func TestRemoteDependencyChecker_NeedsUpdate_FetchError_FailOpen(t *testing.T) {
	mockGit := &mockGitOperations{
		parseCloneURLFunc: defaultParseCloneURL,
		fetchGoModFunc: func(ctx context.Context, cloneURL, ref string) (string, error) {
			return "", fmt.Errorf("network timeout")
		},
	}

	checker := &remoteDependencyChecker{
		cache:  newDependencyCache(5 * time.Minute),
		gitOps: mockGit,
		logger: &mockLogger{},
		options: CheckOptions{
			CacheEnabled:   true,
			CacheTTL:       5 * time.Minute,
			ParallelChecks: 4,
			Timeout:        30 * time.Second,
		},
	}

	dependent := manifest.Dependent{
		Repo:   "goliatone/go-crud",
		Branch: "main",
	}
	target := Target{
		Module:  "github.com/goliatone/go-errors",
		Version: "v0.9.0",
	}

	needsUpdate, err := checker.NeedsUpdate(context.Background(), dependent, target, "")

	// Fail-open behavior: error should result in needsUpdate=true
	if err == nil {
		t.Error("expected error, got nil")
	}
	if !needsUpdate {
		t.Error("expected needsUpdate=true (fail-open) on fetch error")
	}
}

func TestRemoteDependencyChecker_NeedsUpdate_ParseError_FailOpen(t *testing.T) {
	mockGit := &mockGitOperations{
		parseCloneURLFunc: defaultParseCloneURL,
		fetchGoModFunc: func(ctx context.Context, cloneURL, ref string) (string, error) {
			// Return invalid go.mod content
			return "invalid go.mod content!", nil
		},
	}

	checker := &remoteDependencyChecker{
		cache:  newDependencyCache(5 * time.Minute),
		gitOps: mockGit,
		logger: &mockLogger{},
		options: CheckOptions{
			CacheEnabled:   true,
			CacheTTL:       5 * time.Minute,
			ParallelChecks: 4,
			Timeout:        30 * time.Second,
		},
	}

	dependent := manifest.Dependent{
		Repo:   "goliatone/go-crud",
		Branch: "main",
	}
	target := Target{
		Module:  "github.com/goliatone/go-errors",
		Version: "v0.9.0",
	}

	needsUpdate, err := checker.NeedsUpdate(context.Background(), dependent, target, "")

	// Fail-open behavior: parse error should result in needsUpdate=true
	if err == nil {
		t.Error("expected error, got nil")
	}
	if !needsUpdate {
		t.Error("expected needsUpdate=true (fail-open) on parse error")
	}
}

func TestRemoteDependencyChecker_NeedsUpdate_WithReplaceDirective(t *testing.T) {
	mockGit := &mockGitOperations{
		parseCloneURLFunc: defaultParseCloneURL,
		fetchGoModFunc: func(ctx context.Context, cloneURL, ref string) (string, error) {
			return `module github.com/goliatone/go-crud

go 1.21

require (
	github.com/goliatone/go-errors v0.8.0
)

replace github.com/goliatone/go-errors => github.com/goliatone/go-errors v0.7.0
`, nil
		},
	}

	checker := &remoteDependencyChecker{
		cache:  newDependencyCache(5 * time.Minute),
		gitOps: mockGit,
		logger: &mockLogger{},
		options: CheckOptions{
			CacheEnabled:   true,
			CacheTTL:       5 * time.Minute,
			ParallelChecks: 4,
			Timeout:        30 * time.Second,
		},
	}

	dependent := manifest.Dependent{
		Repo:   "goliatone/go-crud",
		Branch: "main",
	}
	target := Target{
		Module:  "github.com/goliatone/go-errors",
		Version: "v0.9.0",
	}

	needsUpdate, err := checker.NeedsUpdate(context.Background(), dependent, target, "")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	// Replace directive should use v0.7.0, which is less than v0.9.0
	if !needsUpdate {
		t.Error("expected needsUpdate=true with replace directive")
	}
}

func TestRemoteDependencyChecker_NeedsUpdate_DefaultBranch(t *testing.T) {
	var capturedRef string
	mockGit := &mockGitOperations{
		parseCloneURLFunc: defaultParseCloneURL,
		fetchGoModFunc: func(ctx context.Context, cloneURL, ref string) (string, error) {
			capturedRef = ref
			return `module github.com/goliatone/go-crud

go 1.21

require (
	github.com/goliatone/go-errors v0.8.0
)
`, nil
		},
	}

	checker := &remoteDependencyChecker{
		cache:  newDependencyCache(5 * time.Minute),
		gitOps: mockGit,
		logger: &mockLogger{},
		options: CheckOptions{
			CacheEnabled:   true,
			CacheTTL:       5 * time.Minute,
			ParallelChecks: 4,
			Timeout:        30 * time.Second,
		},
	}

	// Don't specify branch - should default to "main"
	dependent := manifest.Dependent{
		Repo: "goliatone/go-crud",
	}
	target := Target{
		Module:  "github.com/goliatone/go-errors",
		Version: "v0.9.0",
	}

	_, err := checker.NeedsUpdate(context.Background(), dependent, target, "")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if capturedRef != "main" {
		t.Errorf("expected ref 'main', got '%s'", capturedRef)
	}
}

func TestRemoteDependencyChecker_Warm_Success(t *testing.T) {
	fetchedRepos := make(map[string]bool)
	mockGit := &mockGitOperations{
		parseCloneURLFunc: defaultParseCloneURL,
		fetchGoModFunc: func(ctx context.Context, cloneURL, ref string) (string, error) {
			fetchedRepos[cloneURL] = true
			return `module ` + cloneURL + `

go 1.21

require (
	github.com/goliatone/go-errors v0.8.0
)
`, nil
		},
	}

	checker := &remoteDependencyChecker{
		cache:  newDependencyCache(5 * time.Minute),
		gitOps: mockGit,
		logger: &mockLogger{},
		options: CheckOptions{
			CacheEnabled:   true,
			CacheTTL:       5 * time.Minute,
			ParallelChecks: 4,
			Timeout:        30 * time.Second,
		},
	}

	dependents := []manifest.Dependent{
		{Repo: "goliatone/go-crud", Branch: "main"},
		{Repo: "goliatone/go-auth", Branch: "main"},
		{Repo: "goliatone/go-config", Branch: "main"},
	}

	err := checker.Warm(context.Background(), dependents)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify all repos were fetched
	if len(fetchedRepos) != 3 {
		t.Errorf("expected 3 repos fetched, got %d", len(fetchedRepos))
	}

	// Verify cache was populated
	stats := checker.cache.Stats()
	if stats.Size != 3 {
		t.Errorf("expected cache size 3, got %d", stats.Size)
	}
}

func TestRemoteDependencyChecker_Warm_PartialFailure(t *testing.T) {
	mockGit := &mockGitOperations{
		parseCloneURLFunc: defaultParseCloneURL,
		fetchGoModFunc: func(ctx context.Context, cloneURL, ref string) (string, error) {
			// Simulate failure for one repo
			if cloneURL == "https://github.com/goliatone/go-auth.git" {
				return "", fmt.Errorf("repository not found")
			}
			return `module test

go 1.21

require (
	github.com/goliatone/go-errors v0.8.0
)
`, nil
		},
	}

	checker := &remoteDependencyChecker{
		cache:  newDependencyCache(5 * time.Minute),
		gitOps: mockGit,
		logger: &mockLogger{},
		options: CheckOptions{
			CacheEnabled:   true,
			CacheTTL:       5 * time.Minute,
			ParallelChecks: 4,
			Timeout:        30 * time.Second,
		},
	}

	dependents := []manifest.Dependent{
		{Repo: "goliatone/go-crud", Branch: "main"},
		{Repo: "goliatone/go-auth", Branch: "main"},
		{Repo: "goliatone/go-config", Branch: "main"},
	}

	err := checker.Warm(context.Background(), dependents)

	// Warm should succeed even with partial failures
	if err != nil {
		t.Logf("warm returned error (expected): %v", err)
	}

	// Verify cache has successful entries (2 out of 3)
	stats := checker.cache.Stats()
	if stats.Size != 2 {
		t.Errorf("expected cache size 2 (partial success), got %d", stats.Size)
	}
}

func TestRemoteDependencyChecker_ClearCache(t *testing.T) {
	checker := &remoteDependencyChecker{
		cache:  newDependencyCache(5 * time.Minute),
		gitOps: &mockGitOperations{},
		logger: &mockLogger{},
		options: CheckOptions{
			CacheEnabled: true,
		},
	}

	// Populate cache
	checker.cache.Set("https://github.com/test/repo.git", "main", map[string]string{
		"github.com/foo/bar": "v1.0.0",
	})

	stats := checker.cache.Stats()
	if stats.Size != 1 {
		t.Fatalf("expected cache size 1, got %d", stats.Size)
	}

	// Clear cache
	err := checker.ClearCache()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify cache is empty
	stats = checker.cache.Stats()
	if stats.Size != 0 {
		t.Errorf("expected cache size 0 after clear, got %d", stats.Size)
	}
}

func TestParseGoModContentAndExtractDeps_Success(t *testing.T) {
	content := `module github.com/goliatone/go-crud

go 1.21

require (
	github.com/goliatone/go-errors v0.8.0
	github.com/stretchr/testify v1.8.0
	golang.org/x/mod v0.12.0
)
`

	deps, err := parseGoModContentAndExtractDeps(content)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(deps) != 3 {
		t.Errorf("expected 3 dependencies, got %d", len(deps))
	}
	if deps["github.com/goliatone/go-errors"] != "v0.8.0" {
		t.Errorf("expected go-errors v0.8.0, got %s", deps["github.com/goliatone/go-errors"])
	}
	if deps["github.com/stretchr/testify"] != "v1.8.0" {
		t.Errorf("expected testify v1.8.0, got %s", deps["github.com/stretchr/testify"])
	}
	if deps["golang.org/x/mod"] != "v0.12.0" {
		t.Errorf("expected mod v0.12.0, got %s", deps["golang.org/x/mod"])
	}
}

func TestParseGoModContentAndExtractDeps_WithReplace(t *testing.T) {
	content := `module github.com/goliatone/go-crud

go 1.21

require (
	github.com/goliatone/go-errors v0.8.0
	github.com/foo/bar v1.0.0
)

replace github.com/goliatone/go-errors => github.com/goliatone/go-errors v0.9.0
`

	deps, err := parseGoModContentAndExtractDeps(content)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	// Replace should override the require version
	if deps["github.com/goliatone/go-errors"] != "v0.9.0" {
		t.Errorf("expected go-errors v0.9.0 (from replace), got %s", deps["github.com/goliatone/go-errors"])
	}
}

func TestParseGoModContentAndExtractDeps_LocalReplace(t *testing.T) {
	content := `module github.com/goliatone/go-crud

go 1.21

require (
	github.com/goliatone/go-errors v0.8.0
	github.com/foo/bar v1.0.0
)

replace github.com/goliatone/go-errors => ../go-errors
`

	deps, err := parseGoModContentAndExtractDeps(content)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	// Local path replace should remove from deps (can't compare versions)
	if _, exists := deps["github.com/goliatone/go-errors"]; exists {
		t.Error("expected go-errors to be removed due to local path replace")
	}
	if deps["github.com/foo/bar"] != "v1.0.0" {
		t.Errorf("expected bar v1.0.0, got %s", deps["github.com/foo/bar"])
	}
}

func TestParseGoModContentAndExtractDeps_InvalidContent(t *testing.T) {
	content := "this is not valid go.mod content"

	_, err := parseGoModContentAndExtractDeps(content)

	if err == nil {
		t.Error("expected error for invalid content, got nil")
	}
}

func TestParseGoModContentAndExtractDeps_MissingModuleDirective(t *testing.T) {
	content := `go 1.21

require (
	github.com/goliatone/go-errors v0.8.0
)
`

	_, err := parseGoModContentAndExtractDeps(content)

	if err == nil {
		t.Error("expected error for missing module directive, got nil")
	}
}

func TestNewRemoteDependencyChecker_DefaultOptions(t *testing.T) {
	checker := NewRemoteDependencyChecker(CheckOptions{}, nil)

	rc, ok := checker.(*remoteDependencyChecker)
	if !ok {
		t.Fatal("expected *remoteDependencyChecker")
	}

	if rc.options.ParallelChecks != 4 {
		t.Errorf("expected default ParallelChecks=4, got %d", rc.options.ParallelChecks)
	}
	if rc.options.CacheTTL == 0 {
		t.Error("expected default CacheTTL to be set")
	}
	if rc.options.Timeout == 0 {
		t.Error("expected default Timeout to be set")
	}
}
