package planner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/goliatone/cascade/internal/manifest"
)

// TestDependencyCheckerInterface verifies the DependencyChecker interface contract.
func TestDependencyCheckerInterface(t *testing.T) {
	tests := []struct {
		name      string
		checker   DependencyChecker
		dependent manifest.Dependent
		target    Target
		workspace string
		wantErr   bool
	}{
		{
			name:    "interface is callable",
			checker: &mockChecker{needsUpdate: true, err: nil},
			dependent: manifest.Dependent{
				Repo:   "github.com/example/repo",
				Module: "github.com/example/repo",
			},
			target: Target{
				Module:  "github.com/example/dependency",
				Version: "v1.0.0",
			},
			workspace: "/tmp/workspace",
			wantErr:   false,
		},
		{
			name:    "interface handles errors",
			checker: &mockChecker{needsUpdate: false, err: errors.New("check failed")},
			dependent: manifest.Dependent{
				Repo:   "github.com/example/repo",
				Module: "github.com/example/repo",
			},
			target: Target{
				Module:  "github.com/example/dependency",
				Version: "v1.0.0",
			},
			workspace: "/tmp/workspace",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			needsUpdate, err := tt.checker.NeedsUpdate(ctx, tt.dependent, tt.target, tt.workspace)

			if (err != nil) != tt.wantErr {
				t.Errorf("NeedsUpdate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && needsUpdate != tt.checker.(*mockChecker).needsUpdate {
				t.Errorf("NeedsUpdate() = %v, want %v", needsUpdate, tt.checker.(*mockChecker).needsUpdate)
			}
		})
	}
}

// TestDependencyCheckError verifies the DependencyCheckError type.
func TestDependencyCheckError(t *testing.T) {
	tests := []struct {
		name     string
		err      *DependencyCheckError
		wantMsg  string
		wantWrap error
	}{
		{
			name: "error with context",
			err: &DependencyCheckError{
				Dependent: "github.com/example/repo",
				Target: Target{
					Module:  "github.com/example/dependency",
					Version: "v1.0.0",
				},
				Err: errors.New("go.mod not found"),
			},
			wantMsg:  "dependency check failed for github.com/example/repo (target: github.com/example/dependency@v1.0.0): go.mod not found",
			wantWrap: errors.New("go.mod not found"),
		},
		{
			name: "error with filesystem error",
			err: &DependencyCheckError{
				Dependent: "github.com/example/another",
				Target: Target{
					Module:  "github.com/example/lib",
					Version: "v2.3.4",
				},
				Err: errors.New("permission denied"),
			},
			wantMsg:  "dependency check failed for github.com/example/another (target: github.com/example/lib@v2.3.4): permission denied",
			wantWrap: errors.New("permission denied"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.wantMsg {
				t.Errorf("DependencyCheckError.Error() = %v, want %v", got, tt.wantMsg)
			}

			if unwrapped := tt.err.Unwrap(); unwrapped == nil {
				t.Error("DependencyCheckError.Unwrap() returned nil")
			} else if unwrapped.Error() != tt.wantWrap.Error() {
				t.Errorf("DependencyCheckError.Unwrap() = %v, want %v", unwrapped, tt.wantWrap)
			}
		})
	}
}

// TestCheckResult verifies the CheckResult structure.
func TestCheckResult(t *testing.T) {
	tests := []struct {
		name   string
		result CheckResult
	}{
		{
			name: "needs update result",
			result: CheckResult{
				NeedsUpdate:    true,
				CurrentVersion: "v0.9.0",
				TargetVersion:  "v1.0.0",
				Reason:         "version mismatch",
			},
		},
		{
			name: "up-to-date result",
			result: CheckResult{
				NeedsUpdate:    false,
				CurrentVersion: "v1.0.0",
				TargetVersion:  "v1.0.0",
				Reason:         "already up-to-date",
			},
		},
		{
			name: "dependency not found result",
			result: CheckResult{
				NeedsUpdate:    true,
				CurrentVersion: "",
				TargetVersion:  "v1.0.0",
				Reason:         "dependency not found in go.mod",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify the result has expected fields
			if tt.result.NeedsUpdate && tt.result.Reason == "" {
				t.Error("CheckResult with NeedsUpdate=true should have a Reason")
			}
			if tt.result.TargetVersion == "" {
				t.Error("CheckResult should always have a TargetVersion")
			}
		})
	}
}

// mockChecker is a test implementation of DependencyChecker.
type mockChecker struct {
	needsUpdate bool
	err         error
}

func (m *mockChecker) NeedsUpdate(ctx context.Context, dependent manifest.Dependent, target Target, workspace string) (bool, error) {
	return m.needsUpdate, m.err
}

// mockLogger captures log messages for testing.
type mockLogger struct {
	mu        sync.Mutex
	debugMsgs []string
	infoMsgs  []string
	warnMsgs  []string
	errorMsgs []string
}

func (m *mockLogger) Debug(msg string, args ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.debugMsgs = append(m.debugMsgs, msg)
}

func (m *mockLogger) Info(msg string, args ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.infoMsgs = append(m.infoMsgs, msg)
}

func (m *mockLogger) Warn(msg string, args ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.warnMsgs = append(m.warnMsgs, msg)
}

func (m *mockLogger) Error(msg string, args ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errorMsgs = append(m.errorMsgs, msg)
}

// TestCompareVersionsWrapper tests the version comparison via the public CompareVersions function.
func TestCompareVersionsWrapper(t *testing.T) {
	tests := []struct {
		name        string
		current     string
		target      string
		wantUpdate  bool
		wantErr     bool
		errContains string
	}{
		{
			name:       "current < target",
			current:    "v1.0.0",
			target:     "v1.0.1",
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name:       "current == target",
			current:    "v1.0.1",
			target:     "v1.0.1",
			wantUpdate: false,
			wantErr:    false,
		},
		{
			name:       "current > target",
			current:    "v1.0.2",
			target:     "v1.0.1",
			wantUpdate: false,
			wantErr:    false,
		},
		{
			name:       "pre-release current < release target",
			current:    "v1.0.0-alpha",
			target:     "v1.0.0",
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name:       "build metadata ignored",
			current:    "v1.0.0+build1",
			target:     "v1.0.0+build2",
			wantUpdate: false,
			wantErr:    false,
		},
		{
			name:       "without v prefix",
			current:    "1.0.0",
			target:     "1.0.1",
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name:       "mixed v prefix",
			current:    "v1.0.0",
			target:     "1.0.1",
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name:        "invalid current version",
			current:     "invalid",
			target:      "v1.0.0",
			wantUpdate:  false,
			wantErr:     true,
			errContains: "invalid current version",
		},
		{
			name:        "invalid target version",
			current:     "v1.0.0",
			target:      "invalid",
			wantUpdate:  false,
			wantErr:     true,
			errContains: "invalid target version",
		},
		{
			name:       "major version upgrade",
			current:    "v1.9.9",
			target:     "v2.0.0",
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name:       "patch version only",
			current:    "v1.0.0",
			target:     "v1.0.1",
			wantUpdate: true,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUpdate, err := CompareVersions(tt.current, tt.target)

			if (err != nil) != tt.wantErr {
				t.Errorf("CompareVersions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errContains != "" && err != nil {
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("CompareVersions() error = %v, should contain %q", err, tt.errContains)
				}
			}

			if !tt.wantErr && gotUpdate != tt.wantUpdate {
				t.Errorf("CompareVersions() = %v, want %v", gotUpdate, tt.wantUpdate)
			}
		})
	}
}

// TestLocateRepository tests the repository location logic.
func TestLocateRepository(t *testing.T) {
	// Create temporary test workspace structure
	tmpDir := t.TempDir()

	// Create test repositories
	testRepos := []string{
		"go-crud",                     // Direct in workspace
		"goliatone/go-errors",         // With org
		"github.com/goliatone/go-lib", // Full path
	}

	for _, repo := range testRepos {
		repoPath := tmpDir + "/" + repo
		if err := createTestRepo(repoPath); err != nil {
			t.Fatalf("Failed to create test repo %s: %v", repo, err)
		}
	}

	checker := &dependencyChecker{}

	tests := []struct {
		name      string
		dependent manifest.Dependent
		workspace string
		wantErr   bool
	}{
		{
			name: "direct repo name",
			dependent: manifest.Dependent{
				Repo: "goliatone/go-crud",
			},
			workspace: tmpDir,
			wantErr:   false,
		},
		{
			name: "with org",
			dependent: manifest.Dependent{
				Repo: "goliatone/go-errors",
			},
			workspace: tmpDir,
			wantErr:   false,
		},
		{
			name: "full path",
			dependent: manifest.Dependent{
				Repo: "github.com/goliatone/go-lib",
			},
			workspace: tmpDir,
			wantErr:   false,
		},
		{
			name: "repo not found",
			dependent: manifest.Dependent{
				Repo: "goliatone/nonexistent",
			},
			workspace: tmpDir,
			wantErr:   true,
		},
		{
			name: "empty workspace",
			dependent: manifest.Dependent{
				Repo: "goliatone/go-crud",
			},
			workspace: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := checker.locateRepository(tt.dependent, tt.workspace)
			if (err != nil) != tt.wantErr {
				t.Errorf("locateRepository() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestNewDependencyChecker verifies the constructor.
func TestNewDependencyChecker(t *testing.T) {
	t.Run("with nil logger", func(t *testing.T) {
		checker := NewDependencyChecker(nil)
		if checker == nil {
			t.Error("NewDependencyChecker() returned nil")
		}
	})

	t.Run("with mock logger", func(t *testing.T) {
		logger := &mockLogger{}
		checker := NewDependencyChecker(logger)
		if checker == nil {
			t.Error("NewDependencyChecker() returned nil")
		}
	})
}

// Helper functions
func createTestRepo(path string) error {
	// Create directory structure
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}

	// Create go.mod file
	goModPath := filepath.Join(path, "go.mod")
	goModContent := "module example.com/test\n\ngo 1.21\n"
	return os.WriteFile(goModPath, []byte(goModContent), 0644)
}

// TestNeedsUpdateIntegration tests the full NeedsUpdate flow with real testdata.
func TestNeedsUpdateIntegration(t *testing.T) {
	// Get testdata workspace path
	workspacePath := filepath.Join("testdata", "workspace")

	logger := &mockLogger{}
	checker := NewDependencyChecker(logger).(*dependencyChecker)

	tests := []struct {
		name       string
		dependent  manifest.Dependent
		target     Target
		wantUpdate bool
		wantErr    bool
		wantWarn   bool
		warnMsg    string
	}{
		{
			name: "repo already up-to-date",
			dependent: manifest.Dependent{
				Repo:   "goliatone/repo-up-to-date",
				Module: "github.com/goliatone/repo-up-to-date",
			},
			target: Target{
				Module:  "github.com/goliatone/go-errors",
				Version: "v0.9.0",
			},
			wantUpdate: false,
			wantErr:    false,
		},
		{
			name: "repo needs update",
			dependent: manifest.Dependent{
				Repo:   "goliatone/repo-outdated",
				Module: "github.com/goliatone/repo-outdated",
			},
			target: Target{
				Module:  "github.com/goliatone/go-errors",
				Version: "v0.9.0",
			},
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name: "dependency not in go.mod",
			dependent: manifest.Dependent{
				Repo:   "goliatone/repo-no-dep",
				Module: "github.com/goliatone/repo-no-dep",
			},
			target: Target{
				Module:  "github.com/goliatone/go-errors",
				Version: "v0.9.0",
			},
			wantUpdate: false,
			wantErr:    false,
			wantWarn:   true,
			warnMsg:    "dependency not found in go.mod, skipping update",
		},
		{
			name: "dependency has replace directive",
			dependent: manifest.Dependent{
				Repo:   "goliatone/repo-replaced",
				Module: "github.com/goliatone/repo-replaced",
			},
			target: Target{
				Module:  "github.com/goliatone/go-errors",
				Version: "v0.9.0",
			},
			wantUpdate: true,
			wantErr:    false,
			wantWarn:   true,
			warnMsg:    "dependency has local replace directive",
		},
		{
			name: "go.mod not found",
			dependent: manifest.Dependent{
				Repo:   "goliatone/repo-missing-gomod",
				Module: "github.com/goliatone/repo-missing-gomod",
			},
			target: Target{
				Module:  "github.com/goliatone/go-errors",
				Version: "v0.9.0",
			},
			wantUpdate: false,
			wantErr:    true,
		},
		{
			name: "repository not found",
			dependent: manifest.Dependent{
				Repo:   "goliatone/nonexistent-repo",
				Module: "github.com/goliatone/nonexistent-repo",
			},
			target: Target{
				Module:  "github.com/goliatone/go-errors",
				Version: "v0.9.0",
			},
			wantUpdate: true, // fail-open: assume needs update
			wantErr:    false,
			wantWarn:   true,
			warnMsg:    "repository not found in workspace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset logger
			logger.debugMsgs = nil
			logger.infoMsgs = nil
			logger.warnMsgs = nil
			logger.errorMsgs = nil

			ctx := context.Background()
			gotUpdate, err := checker.NeedsUpdate(ctx, tt.dependent, tt.target, workspacePath)

			if (err != nil) != tt.wantErr {
				t.Errorf("NeedsUpdate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if gotUpdate != tt.wantUpdate {
				t.Errorf("NeedsUpdate() = %v, want %v", gotUpdate, tt.wantUpdate)
			}

			if tt.wantWarn {
				if len(logger.warnMsgs) == 0 {
					t.Errorf("Expected warning message, got none")
				} else if !contains(logger.warnMsgs[0], tt.warnMsg) {
					t.Errorf("Warning message %q should contain %q", logger.warnMsgs[0], tt.warnMsg)
				}
			}
		})
	}
}
