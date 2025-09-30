package planner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/goliatone/cascade/internal/manifest"
)

// mockDependencyChecker implements DependencyChecker for testing
type mockDependencyChecker struct {
	needsUpdateFunc func(ctx context.Context, dependent manifest.Dependent, target Target, workspace string) (bool, error)
	callCount       int
}

func (m *mockDependencyChecker) NeedsUpdate(ctx context.Context, dependent manifest.Dependent, target Target, workspace string) (bool, error) {
	m.callCount++
	if m.needsUpdateFunc != nil {
		return m.needsUpdateFunc(ctx, dependent, target, workspace)
	}
	return false, nil
}

// mockRemoteDependencyChecker implements RemoteDependencyChecker for testing
type mockRemoteDependencyCheckerImpl struct {
	mockDependencyChecker
}

func (m *mockRemoteDependencyCheckerImpl) Warm(ctx context.Context, dependents []manifest.Dependent) error {
	return nil
}

func (m *mockRemoteDependencyCheckerImpl) ClearCache() error {
	return nil
}

func TestHybridDependencyChecker_LocalStrategy(t *testing.T) {
	localChecker := &mockDependencyChecker{
		needsUpdateFunc: func(ctx context.Context, dependent manifest.Dependent, target Target, workspace string) (bool, error) {
			return true, nil
		},
	}

	remoteChecker := &mockRemoteDependencyCheckerImpl{
		mockDependencyChecker: mockDependencyChecker{
			needsUpdateFunc: func(ctx context.Context, dependent manifest.Dependent, target Target, workspace string) (bool, error) {
				t.Error("remote checker should not be called with local strategy")
				return false, nil
			},
		},
	}

	checker := NewHybridDependencyChecker(
		localChecker,
		remoteChecker,
		CheckStrategyLocal,
		"/workspace",
		nil,
	)

	dependent := manifest.Dependent{Repo: "goliatone/test-repo"}
	target := Target{Module: "github.com/goliatone/go-errors", Version: "v0.9.0"}

	needsUpdate, err := checker.NeedsUpdate(context.Background(), dependent, target, "/workspace")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !needsUpdate {
		t.Error("expected needsUpdate=true")
	}
	if localChecker.callCount != 1 {
		t.Errorf("expected local checker called once, got %d", localChecker.callCount)
	}
	if remoteChecker.callCount != 0 {
		t.Errorf("expected remote checker not called, got %d calls", remoteChecker.callCount)
	}
}

func TestHybridDependencyChecker_RemoteStrategy(t *testing.T) {
	localChecker := &mockDependencyChecker{
		needsUpdateFunc: func(ctx context.Context, dependent manifest.Dependent, target Target, workspace string) (bool, error) {
			t.Error("local checker should not be called with remote strategy")
			return false, nil
		},
	}

	remoteChecker := &mockRemoteDependencyCheckerImpl{
		mockDependencyChecker: mockDependencyChecker{
			needsUpdateFunc: func(ctx context.Context, dependent manifest.Dependent, target Target, workspace string) (bool, error) {
				return true, nil
			},
		},
	}

	checker := NewHybridDependencyChecker(
		localChecker,
		remoteChecker,
		CheckStrategyRemote,
		"/workspace",
		nil,
	)

	dependent := manifest.Dependent{Repo: "goliatone/test-repo"}
	target := Target{Module: "github.com/goliatone/go-errors", Version: "v0.9.0"}

	needsUpdate, err := checker.NeedsUpdate(context.Background(), dependent, target, "/workspace")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !needsUpdate {
		t.Error("expected needsUpdate=true")
	}
	if localChecker.callCount != 0 {
		t.Errorf("expected local checker not called, got %d calls", localChecker.callCount)
	}
	if remoteChecker.callCount != 1 {
		t.Errorf("expected remote checker called once, got %d", remoteChecker.callCount)
	}
}

func TestHybridDependencyChecker_AutoStrategy_LocalSuccess(t *testing.T) {
	localChecker := &mockDependencyChecker{
		needsUpdateFunc: func(ctx context.Context, dependent manifest.Dependent, target Target, workspace string) (bool, error) {
			return true, nil // Local check succeeds
		},
	}

	remoteChecker := &mockRemoteDependencyCheckerImpl{
		mockDependencyChecker: mockDependencyChecker{
			needsUpdateFunc: func(ctx context.Context, dependent manifest.Dependent, target Target, workspace string) (bool, error) {
				t.Error("remote checker should not be called when local succeeds")
				return false, nil
			},
		},
	}

	checker := NewHybridDependencyChecker(
		localChecker,
		remoteChecker,
		CheckStrategyAuto,
		"/workspace",
		nil,
	)

	dependent := manifest.Dependent{Repo: "goliatone/test-repo"}
	target := Target{Module: "github.com/goliatone/go-errors", Version: "v0.9.0"}

	needsUpdate, err := checker.NeedsUpdate(context.Background(), dependent, target, "/workspace")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !needsUpdate {
		t.Error("expected needsUpdate=true")
	}
	if localChecker.callCount != 1 {
		t.Errorf("expected local checker called once, got %d", localChecker.callCount)
	}
	if remoteChecker.callCount != 0 {
		t.Errorf("expected remote checker not called, got %d calls", remoteChecker.callCount)
	}
}

func TestHybridDependencyChecker_AutoStrategy_LocalFallbackToRemote(t *testing.T) {
	localChecker := &mockDependencyChecker{
		needsUpdateFunc: func(ctx context.Context, dependent manifest.Dependent, target Target, workspace string) (bool, error) {
			return false, errors.New("repository not found in workspace")
		},
	}

	remoteChecker := &mockRemoteDependencyCheckerImpl{
		mockDependencyChecker: mockDependencyChecker{
			needsUpdateFunc: func(ctx context.Context, dependent manifest.Dependent, target Target, workspace string) (bool, error) {
				return true, nil // Remote check succeeds
			},
		},
	}

	checker := NewHybridDependencyChecker(
		localChecker,
		remoteChecker,
		CheckStrategyAuto,
		"/workspace",
		nil,
	)

	dependent := manifest.Dependent{Repo: "goliatone/test-repo"}
	target := Target{Module: "github.com/goliatone/go-errors", Version: "v0.9.0"}

	needsUpdate, err := checker.NeedsUpdate(context.Background(), dependent, target, "/workspace")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !needsUpdate {
		t.Error("expected needsUpdate=true from remote fallback")
	}
	if localChecker.callCount != 1 {
		t.Errorf("expected local checker called once, got %d", localChecker.callCount)
	}
	if remoteChecker.callCount != 1 {
		t.Errorf("expected remote checker called once (fallback), got %d", remoteChecker.callCount)
	}
}

func TestHybridDependencyChecker_AutoStrategy_BothFail(t *testing.T) {
	localChecker := &mockDependencyChecker{
		needsUpdateFunc: func(ctx context.Context, dependent manifest.Dependent, target Target, workspace string) (bool, error) {
			return false, errors.New("local error")
		},
	}

	remoteChecker := &mockRemoteDependencyCheckerImpl{
		mockDependencyChecker: mockDependencyChecker{
			needsUpdateFunc: func(ctx context.Context, dependent manifest.Dependent, target Target, workspace string) (bool, error) {
				return true, errors.New("remote error")
			},
		},
	}

	checker := NewHybridDependencyChecker(
		localChecker,
		remoteChecker,
		CheckStrategyAuto,
		"/workspace",
		nil,
	)

	dependent := manifest.Dependent{Repo: "goliatone/test-repo"}
	target := Target{Module: "github.com/goliatone/go-errors", Version: "v0.9.0"}

	needsUpdate, err := checker.NeedsUpdate(context.Background(), dependent, target, "/workspace")

	// Should return remote error (fail-open returns true)
	if err == nil {
		t.Error("expected error from remote fallback")
	}
	if !needsUpdate {
		t.Error("expected needsUpdate=true (fail-open)")
	}
	if localChecker.callCount != 1 {
		t.Errorf("expected local checker called once, got %d", localChecker.callCount)
	}
	if remoteChecker.callCount != 1 {
		t.Errorf("expected remote checker called once (fallback), got %d", remoteChecker.callCount)
	}
}

func TestHybridDependencyChecker_UnknownStrategy(t *testing.T) {
	localChecker := &mockDependencyChecker{}
	remoteChecker := &mockRemoteDependencyCheckerImpl{}

	checker := NewHybridDependencyChecker(
		localChecker,
		remoteChecker,
		"invalid",
		"/workspace",
		nil,
	)

	dependent := manifest.Dependent{Repo: "goliatone/test-repo"}
	target := Target{Module: "github.com/goliatone/go-errors", Version: "v0.9.0"}

	needsUpdate, err := checker.NeedsUpdate(context.Background(), dependent, target, "/workspace")

	if err == nil {
		t.Error("expected error for unknown strategy")
	}
	if !needsUpdate {
		t.Error("expected needsUpdate=true (fail-open)")
	}
	if !errors.Is(err, errors.New("unknown check strategy: invalid")) && err.Error() != "unknown check strategy: invalid" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestHybridDependencyChecker_WithLogger(t *testing.T) {
	logger := &mockLogger{}

	localChecker := &mockDependencyChecker{
		needsUpdateFunc: func(ctx context.Context, dependent manifest.Dependent, target Target, workspace string) (bool, error) {
			return false, errors.New("local error")
		},
	}

	remoteChecker := &mockRemoteDependencyCheckerImpl{
		mockDependencyChecker: mockDependencyChecker{
			needsUpdateFunc: func(ctx context.Context, dependent manifest.Dependent, target Target, workspace string) (bool, error) {
				return true, nil
			},
		},
	}

	checker := NewHybridDependencyChecker(
		localChecker,
		remoteChecker,
		CheckStrategyAuto,
		"/workspace",
		logger,
	)

	dependent := manifest.Dependent{Repo: "goliatone/test-repo"}
	target := Target{Module: "github.com/goliatone/go-errors", Version: "v0.9.0"}

	_, _ = checker.NeedsUpdate(context.Background(), dependent, target, "/workspace")

	// Verify logging occurred
	if len(logger.debugMsgs) == 0 {
		t.Error("expected debug logs to be recorded")
	}
}

func TestDetectCheckStrategy_ExplicitLocal(t *testing.T) {
	opts := CheckOptions{Strategy: CheckStrategyLocal}
	strategy := detectCheckStrategy("/some/workspace", opts)

	if strategy != CheckStrategyLocal {
		t.Errorf("expected local strategy, got %s", strategy)
	}
}

func TestDetectCheckStrategy_ExplicitRemote(t *testing.T) {
	opts := CheckOptions{Strategy: CheckStrategyRemote}
	strategy := detectCheckStrategy("/some/workspace", opts)

	if strategy != CheckStrategyRemote {
		t.Errorf("expected remote strategy, got %s", strategy)
	}
}

func TestDetectCheckStrategy_AutoWithValidWorkspace(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "cascade-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	opts := CheckOptions{Strategy: CheckStrategyAuto}
	strategy := detectCheckStrategy(tmpDir, opts)

	if strategy != CheckStrategyLocal {
		t.Errorf("expected local strategy for valid workspace, got %s", strategy)
	}
}

func TestDetectCheckStrategy_AutoWithInvalidWorkspace(t *testing.T) {
	opts := CheckOptions{Strategy: CheckStrategyAuto}
	strategy := detectCheckStrategy("/nonexistent/workspace", opts)

	if strategy != CheckStrategyRemote {
		t.Errorf("expected remote strategy for invalid workspace, got %s", strategy)
	}
}

func TestDetectCheckStrategy_AutoWithEmptyWorkspace(t *testing.T) {
	opts := CheckOptions{Strategy: CheckStrategyAuto}
	strategy := detectCheckStrategy("", opts)

	if strategy != CheckStrategyRemote {
		t.Errorf("expected remote strategy for empty workspace, got %s", strategy)
	}
}

func TestDetectCheckStrategy_EmptyStrategyDefaultsToAuto(t *testing.T) {
	opts := CheckOptions{Strategy: ""}
	strategy := detectCheckStrategy("", opts)

	if strategy != CheckStrategyRemote {
		t.Errorf("expected remote strategy (auto with no workspace), got %s", strategy)
	}
}

func TestDetectCheckStrategy_AutoWithFileInsteadOfDirectory(t *testing.T) {
	// Create a temporary file (not directory)
	tmpFile, err := os.CreateTemp("", "cascade-test-file-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	opts := CheckOptions{Strategy: CheckStrategyAuto}
	strategy := detectCheckStrategy(tmpFile.Name(), opts)

	// Should fallback to remote since it's a file, not a directory
	if strategy != CheckStrategyRemote {
		t.Errorf("expected remote strategy for file path, got %s", strategy)
	}
}

func TestDetectCheckStrategy_PackageLevel(t *testing.T) {
	// Test the exported DetectCheckStrategy function
	tmpDir, err := os.MkdirTemp("", "cascade-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	opts := CheckOptions{Strategy: CheckStrategyAuto}
	strategy := DetectCheckStrategy(tmpDir, opts)

	if strategy != CheckStrategyLocal {
		t.Errorf("expected local strategy, got %s", strategy)
	}
}

func TestHybridDependencyChecker_Integration_LocalToRemoteFallback(t *testing.T) {
	// Create a temporary workspace
	tmpWorkspace, err := os.MkdirTemp("", "cascade-workspace-*")
	if err != nil {
		t.Fatalf("failed to create temp workspace: %v", err)
	}
	defer os.RemoveAll(tmpWorkspace)

	// Create a temporary workspace with a repository that has a malformed go.mod
	// This will cause a hard error from local checker, triggering fallback
	repoDir := filepath.Join(tmpWorkspace, "test-repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	// Create an invalid go.mod (this will cause local checker to return error)
	goModPath := filepath.Join(repoDir, "go.mod")
	if err := os.WriteFile(goModPath, []byte("invalid go.mod"), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	// Create a real local checker
	localChecker := NewDependencyChecker(nil)

	// Create a mock remote checker
	remoteChecker := &mockRemoteDependencyCheckerImpl{
		mockDependencyChecker: mockDependencyChecker{
			needsUpdateFunc: func(ctx context.Context, dependent manifest.Dependent, target Target, workspace string) (bool, error) {
				return true, nil
			},
		},
	}

	// Create hybrid checker with auto strategy
	checker := NewHybridDependencyChecker(
		localChecker,
		remoteChecker,
		CheckStrategyAuto,
		tmpWorkspace,
		nil,
	)

	// Test with a repository that has malformed go.mod (triggers fallback)
	dependent := manifest.Dependent{
		Repo:   "goliatone/test-repo",
		Branch: "main",
	}
	target := Target{
		Module:  "github.com/goliatone/go-errors",
		Version: "v0.9.0",
	}

	needsUpdate, err := checker.NeedsUpdate(context.Background(), dependent, target, tmpWorkspace)

	// Should fallback to remote checker (which returns true)
	if err != nil {
		t.Fatalf("expected no error from remote fallback, got: %v", err)
	}
	if !needsUpdate {
		t.Error("expected needsUpdate=true from remote fallback")
	}

	// Verify remote checker was called (fallback occurred)
	if remoteChecker.callCount != 1 {
		t.Errorf("expected remote checker called once (fallback), got %d", remoteChecker.callCount)
	}
}

func TestHybridDependencyChecker_Integration_LocalSuccess(t *testing.T) {
	// Create a temporary workspace with a test repository
	tmpWorkspace, err := os.MkdirTemp("", "cascade-workspace-*")
	if err != nil {
		t.Fatalf("failed to create temp workspace: %v", err)
	}
	defer os.RemoveAll(tmpWorkspace)

	// Create a mock repository directory with go.mod
	repoDir := filepath.Join(tmpWorkspace, "test-repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	goModContent := `module github.com/goliatone/test-repo

go 1.21

require (
	github.com/goliatone/go-errors v0.8.0
)
`
	goModPath := filepath.Join(repoDir, "go.mod")
	if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	// Create a real local checker
	localChecker := NewDependencyChecker(nil)

	// Create a mock remote checker that should NOT be called
	remoteChecker := &mockRemoteDependencyCheckerImpl{
		mockDependencyChecker: mockDependencyChecker{
			needsUpdateFunc: func(ctx context.Context, dependent manifest.Dependent, target Target, workspace string) (bool, error) {
				t.Error("remote checker should not be called when local succeeds")
				return false, nil
			},
		},
	}

	// Create hybrid checker with auto strategy
	checker := NewHybridDependencyChecker(
		localChecker,
		remoteChecker,
		CheckStrategyAuto,
		tmpWorkspace,
		nil,
	)

	// Test with the repository that exists in workspace
	dependent := manifest.Dependent{
		Repo:   "goliatone/test-repo",
		Branch: "main",
	}
	target := Target{
		Module:  "github.com/goliatone/go-errors",
		Version: "v0.9.0",
	}

	needsUpdate, err := checker.NeedsUpdate(context.Background(), dependent, target, tmpWorkspace)

	// Should succeed with local checker
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !needsUpdate {
		t.Error("expected needsUpdate=true (v0.8.0 < v0.9.0)")
	}

	// Verify remote checker was NOT called
	if remoteChecker.callCount != 0 {
		t.Errorf("expected remote checker not called, got %d calls", remoteChecker.callCount)
	}
}
