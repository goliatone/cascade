package planner

import (
	"context"
	"errors"
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
