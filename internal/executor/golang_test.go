package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupFakeGoBinary(t *testing.T) func() {
	t.Helper()
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "go")
	script := `#!/bin/sh
set -eu
cmd="$1"
shift || true
case "$cmd" in
    get)
        module="${1:-}"
        if echo "$module" | grep -q "invalid/nonexistent/module"; then
            echo "go: module $module: not found" >&2
            exit 1
        fi
        exit 0
        ;;
    mod)
        sub="$1"
        shift || true
        case "$sub" in
            tidy)
                if [ ! -f go.mod ]; then
                    echo "go: go.mod file not found in current directory" >&2
                    exit 1
                fi
                exit 0
                ;;
        esac
        ;;
esac
exit 0
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write fake go binary: %v", err)
	}

	origPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", dir+string(os.PathListSeparator)+origPath); err != nil {
		t.Fatalf("failed to update PATH: %v", err)
	}

	return func() {
		os.Setenv("PATH", origPath)
	}
}

func TestGoOperations_Get(t *testing.T) {
	cleanup := setupFakeGoBinary(t)
	defer cleanup()

	tests := []struct {
		name        string
		module      string
		version     string
		wantErr     bool
		errContains string
	}{
		{
			name:    "successful get with version",
			module:  "github.com/stretchr/testify",
			version: "v1.8.0",
			wantErr: false,
		},
		{
			name:    "successful get latest version",
			module:  "github.com/stretchr/testify",
			version: "latest",
			wantErr: false,
		},
		{
			name:    "successful get without version",
			module:  "github.com/stretchr/testify",
			version: "",
			wantErr: false,
		},
		{
			name:        "invalid module",
			module:      "invalid/nonexistent/module/that/should/not/exist",
			version:     "v1.0.0",
			wantErr:     true,
			errContains: "go get failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory with a valid go module
			tempDir := t.TempDir()
			createTestModule(t, tempDir, "test-module", "v1.0.0")

			goOps := NewGoOperations()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			err := goOps.Get(ctx, tempDir, tt.module, tt.version)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Get() expected error but got nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Get() error = %v, expected to contain %v", err, tt.errContains)
				}

				// Verify it's a GoOperationError
				if !IsGoError(err) {
					t.Errorf("Get() expected GoOperationError, got %T", err)
				}
			} else {
				if err != nil {
					t.Errorf("Get() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestGoOperations_Tidy(t *testing.T) {
	cleanup := setupFakeGoBinary(t)
	defer cleanup()

	tests := []struct {
		name        string
		setupModule bool
		wantErr     bool
		errContains string
	}{
		{
			name:        "successful tidy",
			setupModule: true,
			wantErr:     false,
		},
		{
			name:        "tidy without go.mod",
			setupModule: false,
			wantErr:     true,
			errContains: "go mod tidy failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()

			if tt.setupModule {
				createTestModule(t, tempDir, "test-module", "v1.0.0")
			}

			goOps := NewGoOperations()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			err := goOps.Tidy(ctx, tempDir)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Tidy() expected error but got nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Tidy() error = %v, expected to contain %v", err, tt.errContains)
				}

				// Verify it's a GoOperationError
				if !IsGoError(err) {
					t.Errorf("Tidy() expected GoOperationError, got %T", err)
				}
			} else {
				if err != nil {
					t.Errorf("Tidy() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestGoOperations_GetWithContext(t *testing.T) {
	cleanup := setupFakeGoBinary(t)
	defer cleanup()

	tempDir := t.TempDir()
	createTestModule(t, tempDir, "test-module", "v1.0.0")

	goOps := NewGoOperations()

	// Test context cancellation
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := goOps.Get(ctx, tempDir, "github.com/stretchr/testify", "v1.8.0")
	if err == nil {
		t.Error("Get() expected error due to cancelled context")
	}

	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("Get() expected context cancellation error, got %v", err)
	}
}

func TestGoOperations_TidyWithContext(t *testing.T) {
	cleanup := setupFakeGoBinary(t)
	defer cleanup()

	tempDir := t.TempDir()
	createTestModule(t, tempDir, "test-module", "v1.0.0")

	goOps := NewGoOperations()

	// Test context cancellation
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := goOps.Tidy(ctx, tempDir)
	if err == nil {
		t.Error("Tidy() expected error due to cancelled context")
	}

	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("Tidy() expected context cancellation error, got %v", err)
	}
}

// createTestModule creates a minimal go.mod file for testing
func createTestModule(t *testing.T, dir, moduleName, _ string) {
	t.Helper()

	goModContent := fmt.Sprintf(`module %s

go 1.21

require (
    github.com/pkg/errors v0.9.1
)
`, moduleName)

	goModPath := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
		t.Fatalf("failed to create go.mod: %v", err)
	}

	// Create a simple Go file to make it a valid module
	mainGoContent := `package main

import (
    "fmt"
    "github.com/pkg/errors"
)

func main() {
    err := errors.New("test error")
    fmt.Println(err)
}
`

	mainGoPath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainGoPath, []byte(mainGoContent), 0644); err != nil {
		t.Fatalf("failed to create main.go: %v", err)
	}
}
