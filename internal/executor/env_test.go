package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPrepareEnvMerging(t *testing.T) {
	tests := []struct {
		name   string
		base   map[string]string
		custom map[string]string
		want   map[string]string
	}{
		{
			name:   "empty environments",
			base:   map[string]string{},
			custom: map[string]string{},
			want:   map[string]string{},
		},
		{
			name:   "base only",
			base:   map[string]string{"PATH": "/usr/bin", "HOME": "/home/user"},
			custom: map[string]string{},
			want:   map[string]string{"PATH": "/usr/bin", "HOME": "/home/user"},
		},
		{
			name:   "custom only",
			base:   map[string]string{},
			custom: map[string]string{"DEBUG": "true", "LOG_LEVEL": "info"},
			want:   map[string]string{"DEBUG": "true", "LOG_LEVEL": "info"},
		},
		{
			name:   "merge without conflicts",
			base:   map[string]string{"PATH": "/usr/bin", "HOME": "/home/user"},
			custom: map[string]string{"DEBUG": "true", "LOG_LEVEL": "info"},
			want:   map[string]string{"PATH": "/usr/bin", "HOME": "/home/user", "DEBUG": "true", "LOG_LEVEL": "info"},
		},
		{
			name:   "custom overrides base",
			base:   map[string]string{"PATH": "/usr/bin", "DEBUG": "false"},
			custom: map[string]string{"DEBUG": "true", "LOG_LEVEL": "info"},
			want:   map[string]string{"PATH": "/usr/bin", "DEBUG": "true", "LOG_LEVEL": "info"},
		},
		{
			name:   "nil base",
			base:   nil,
			custom: map[string]string{"DEBUG": "true"},
			want:   map[string]string{"DEBUG": "true"},
		},
		{
			name:   "nil custom",
			base:   map[string]string{"PATH": "/usr/bin"},
			custom: nil,
			want:   map[string]string{"PATH": "/usr/bin"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PrepareEnv(tt.base, tt.custom)

			// Convert slice back to map for easier comparison
			gotMap := make(map[string]string)
			for _, env := range got {
				parts := strings.SplitN(env, "=", 2)
				if len(parts) == 2 {
					gotMap[parts[0]] = parts[1]
				}
			}

			if len(gotMap) != len(tt.want) {
				t.Errorf("PrepareEnv() returned %d items, want %d", len(gotMap), len(tt.want))
			}

			for k, v := range tt.want {
				if gotMap[k] != v {
					t.Errorf("PrepareEnv()[%q] = %q, want %q", k, gotMap[k], v)
				}
			}
		})
	}
}

func TestValidateTimeout(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
		want    time.Duration
	}{
		{
			name:    "zero timeout returns default",
			timeout: 0,
			want:    15 * time.Minute,
		},
		{
			name:    "negative timeout returns default",
			timeout: -1 * time.Second,
			want:    15 * time.Minute,
		},
		{
			name:    "positive timeout unchanged",
			timeout: 5 * time.Minute,
			want:    5 * time.Minute,
		},
		{
			name:    "very small timeout unchanged",
			timeout: 1 * time.Nanosecond,
			want:    1 * time.Nanosecond,
		},
		{
			name:    "large timeout unchanged",
			timeout: 2 * time.Hour,
			want:    2 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateTimeout(tt.timeout)
			if got != tt.want {
				t.Errorf("ValidateTimeout(%v) = %v, want %v", tt.timeout, got, tt.want)
			}
		})
	}
}

func TestCreateTempWorkspace(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		item     string
		wantErr  bool
		checkDir bool
	}{
		{
			name:     "empty base uses temp dir",
			base:     "",
			item:     "test-workspace",
			wantErr:  false,
			checkDir: true,
		},
		{
			name:     "explicit base directory",
			base:     t.TempDir(),
			item:     "test-workspace",
			wantErr:  false,
			checkDir: true,
		},
		{
			name:     "nested item path",
			base:     t.TempDir(),
			item:     "nested/workspace/path",
			wantErr:  false,
			checkDir: true,
		},
		{
			name:     "empty item name",
			base:     t.TempDir(),
			item:     "",
			wantErr:  false,
			checkDir: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CreateTempWorkspace(tt.base, tt.item)

			if (err != nil) != tt.wantErr {
				t.Errorf("CreateTempWorkspace() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil && tt.checkDir {
				// Verify directory was created
				if _, err := os.Stat(got); os.IsNotExist(err) {
					t.Errorf("CreateTempWorkspace() created path %q does not exist", got)
				}

				// Verify it's the expected path
				expectedBase := tt.base
				if expectedBase == "" {
					expectedBase = os.TempDir()
				}
				expectedPath := filepath.Join(expectedBase, tt.item)
				if got != expectedPath {
					t.Errorf("CreateTempWorkspace() = %q, want %q", got, expectedPath)
				}

				// Clean up
				os.RemoveAll(got)
			}
		})
	}
}

func TestCleanupWorkspace(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() string
		wantErr bool
	}{
		{
			name: "cleanup existing directory",
			setup: func() string {
				dir := filepath.Join(t.TempDir(), "test-cleanup")
				os.MkdirAll(dir, 0755)
				// Create a test file
				os.WriteFile(filepath.Join(dir, "test.txt"), []byte("test"), 0644)
				return dir
			},
			wantErr: false,
		},
		{
			name: "cleanup non-existent directory",
			setup: func() string {
				return filepath.Join(t.TempDir(), "non-existent")
			},
			wantErr: false,
		},
		{
			name: "cleanup empty path",
			setup: func() string {
				return ""
			},
			wantErr: false,
		},
		{
			name: "cleanup nested directories",
			setup: func() string {
				base := t.TempDir()
				nested := filepath.Join(base, "a", "b", "c")
				os.MkdirAll(nested, 0755)
				os.WriteFile(filepath.Join(nested, "file.txt"), []byte("content"), 0644)
				return base
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup()

			// Verify setup worked if path is not empty
			if path != "" && !strings.Contains(tt.name, "non-existent") {
				if _, err := os.Stat(path); os.IsNotExist(err) {
					t.Fatalf("Setup failed: path %q should exist", path)
				}
			}

			err := CleanupWorkspace(path)
			if (err != nil) != tt.wantErr {
				t.Errorf("CleanupWorkspace() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify cleanup worked if no error and path not empty
			if err == nil && path != "" {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("CleanupWorkspace() failed to remove path %q", path)
				}
			}
		})
	}
}

func TestWorkspaceIsolation(t *testing.T) {
	// Test that multiple workspaces don't interfere with each other
	base := t.TempDir()

	workspace1, err := CreateTempWorkspace(base, "workspace1")
	if err != nil {
		t.Fatalf("CreateTempWorkspace failed: %v", err)
	}

	workspace2, err := CreateTempWorkspace(base, "workspace2")
	if err != nil {
		t.Fatalf("CreateTempWorkspace failed: %v", err)
	}

	// Create files in each workspace
	file1 := filepath.Join(workspace1, "test1.txt")
	file2 := filepath.Join(workspace2, "test2.txt")

	if err := os.WriteFile(file1, []byte("content1"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if err := os.WriteFile(file2, []byte("content2"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Verify both files exist
	if _, err := os.Stat(file1); err != nil {
		t.Errorf("File in workspace1 should exist: %v", err)
	}
	if _, err := os.Stat(file2); err != nil {
		t.Errorf("File in workspace2 should exist: %v", err)
	}

	// Cleanup workspace1
	if err := CleanupWorkspace(workspace1); err != nil {
		t.Fatalf("CleanupWorkspace failed: %v", err)
	}

	// Verify workspace1 is gone but workspace2 still exists
	if _, err := os.Stat(workspace1); !os.IsNotExist(err) {
		t.Errorf("Workspace1 should be removed")
	}
	if _, err := os.Stat(file2); err != nil {
		t.Errorf("File in workspace2 should still exist: %v", err)
	}

	// Cleanup workspace2
	if err := CleanupWorkspace(workspace2); err != nil {
		t.Fatalf("CleanupWorkspace failed: %v", err)
	}
}
