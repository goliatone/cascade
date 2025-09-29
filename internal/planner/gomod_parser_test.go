package planner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseGoMod(t *testing.T) {
	tests := []struct {
		name        string
		fixture     string
		wantModule  string
		wantErr     bool
		errContains string
	}{
		{
			name:       "simple go.mod",
			fixture:    "simple.mod",
			wantModule: "github.com/example/simple",
			wantErr:    false,
		},
		{
			name:       "multiline requires",
			fixture:    "multiline.mod",
			wantModule: "github.com/example/multiline",
			wantErr:    false,
		},
		{
			name:       "go.mod with replace",
			fixture:    "replace.mod",
			wantModule: "github.com/example/replace",
			wantErr:    false,
		},
		{
			name:       "go.mod with indirect",
			fixture:    "indirect.mod",
			wantModule: "github.com/example/indirect",
			wantErr:    false,
		},
		{
			name:        "non-existent file",
			fixture:     "nonexistent.mod",
			wantErr:     true,
			errContains: "failed to read go.mod file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join("testdata", "gomod_samples", tt.fixture)

			modInfo, err := ParseGoMod(path)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseGoMod() expected error, got nil")
				} else if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("ParseGoMod() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseGoMod() unexpected error = %v", err)
			}

			if modInfo.Module != tt.wantModule {
				t.Errorf("ParseGoMod() module = %v, want %v", modInfo.Module, tt.wantModule)
			}

			if modInfo.File == nil {
				t.Error("ParseGoMod() File is nil")
			}

			if modInfo.FilePath != path {
				t.Errorf("ParseGoMod() FilePath = %v, want %v", modInfo.FilePath, path)
			}
		})
	}
}

func TestExtractDependency(t *testing.T) {
	tests := []struct {
		name        string
		fixture     string
		modulePath  string
		wantVersion string
		wantErr     bool
		errContains string
	}{
		{
			name:        "simple direct dependency",
			fixture:     "simple.mod",
			modulePath:  "github.com/goliatone/go-errors",
			wantVersion: "v0.8.0",
			wantErr:     false,
		},
		{
			name:        "dependency in multiline block",
			fixture:     "multiline.mod",
			modulePath:  "github.com/goliatone/go-errors",
			wantVersion: "v0.8.0",
			wantErr:     false,
		},
		{
			name:        "second dependency in multiline block",
			fixture:     "multiline.mod",
			modulePath:  "github.com/another/dependency",
			wantVersion: "v1.2.3",
			wantErr:     false,
		},
		{
			name:        "dependency with replace directive",
			fixture:     "replace.mod",
			modulePath:  "github.com/replaced/module",
			wantVersion: "v1.5.0",
			wantErr:     false,
		},
		{
			name:        "indirect dependency",
			fixture:     "indirect.mod",
			modulePath:  "github.com/indirect/dependency",
			wantVersion: "v0.5.0",
			wantErr:     false,
		},
		{
			name:        "missing dependency",
			fixture:     "missing_dep.mod",
			modulePath:  "github.com/goliatone/go-errors",
			wantErr:     true,
			errContains: "not found in go.mod",
		},
		{
			name:        "local replace directive",
			fixture:     "local_replace.mod",
			modulePath:  "github.com/goliatone/go-errors",
			wantErr:     true,
			errContains: "local replace directive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join("testdata", "gomod_samples", tt.fixture)
			modInfo, err := ParseGoMod(path)
			if err != nil {
				t.Fatalf("ParseGoMod() failed: %v", err)
			}

			version, err := ExtractDependency(modInfo, tt.modulePath)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ExtractDependency() expected error, got nil")
				} else if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("ExtractDependency() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("ExtractDependency() unexpected error = %v", err)
			}

			if version != tt.wantVersion {
				t.Errorf("ExtractDependency() version = %v, want %v", version, tt.wantVersion)
			}
		})
	}
}

func TestExtractDependency_InvalidInput(t *testing.T) {
	tests := []struct {
		name        string
		modInfo     *ModuleInfo
		modulePath  string
		errContains string
	}{
		{
			name:        "nil module info",
			modInfo:     nil,
			modulePath:  "github.com/example/test",
			errContains: "invalid module info",
		},
		{
			name: "module info with nil file",
			modInfo: &ModuleInfo{
				Module: "test",
				File:   nil,
			},
			modulePath:  "github.com/example/test",
			errContains: "invalid module info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ExtractDependency(tt.modInfo, tt.modulePath)
			if err == nil {
				t.Error("ExtractDependency() expected error, got nil")
			} else if !containsString(err.Error(), tt.errContains) {
				t.Errorf("ExtractDependency() error = %v, want error containing %q", err, tt.errContains)
			}
		})
	}
}

func TestFindGoModFile(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create a test go.mod file
	goModPath := filepath.Join(tmpDir, "go.mod")
	if err := os.WriteFile(goModPath, []byte("module test\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatalf("Failed to create test go.mod: %v", err)
	}

	tests := []struct {
		name        string
		repoPath    string
		wantPath    string
		wantErr     bool
		errContains string
	}{
		{
			name:     "go.mod exists",
			repoPath: tmpDir,
			wantPath: goModPath,
			wantErr:  false,
		},
		{
			name:        "go.mod does not exist",
			repoPath:    filepath.Join(tmpDir, "nonexistent"),
			wantErr:     true,
			errContains: "go.mod not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath, err := findGoModFile(tt.repoPath)

			if tt.wantErr {
				if err == nil {
					t.Errorf("findGoModFile() expected error, got nil")
				} else if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("findGoModFile() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("findGoModFile() unexpected error = %v", err)
			}

			if gotPath != tt.wantPath {
				t.Errorf("findGoModFile() path = %v, want %v", gotPath, tt.wantPath)
			}
		})
	}
}

func TestFindGoModFile_DirectoryNamedGoMod(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	// Create a directory named "go.mod" (instead of a file)
	goModDirPath := filepath.Join(tmpDir, "go.mod")
	if err := os.Mkdir(goModDirPath, 0755); err != nil {
		t.Fatalf("Failed to create go.mod directory: %v", err)
	}

	_, err := findGoModFile(tmpDir)
	if err == nil {
		t.Error("findGoModFile() expected error for directory named go.mod, got nil")
	} else if !containsString(err.Error(), "is a directory") {
		t.Errorf("findGoModFile() error = %v, want error containing 'is a directory'", err)
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
