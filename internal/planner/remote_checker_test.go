package planner

import (
	"context"
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
