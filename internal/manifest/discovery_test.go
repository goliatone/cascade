package manifest

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceDiscovery_DiscoverDependents(t *testing.T) {
	discovery := NewWorkspaceDiscovery()
	testdataDir := filepath.Join("testdata", "workspace-discovery")

	tests := []struct {
		name          string
		options       DiscoveryOptions
		expectedCount int
		expectedRepos []string
		expectError   bool
	}{
		{
			name: "finds modules that depend on target",
			options: DiscoveryOptions{
				WorkspaceDir: testdataDir,
				TargetModule: "github.com/target/module",
			},
			expectedCount: 3, // module-a, module-b, and module-d depend on target/module
			expectedRepos: []string{"example/module-a", "example/module-b", "example/module-d"},
			expectError:   false,
		},
		{
			name: "excludes directories matching patterns",
			options: DiscoveryOptions{
				WorkspaceDir:    testdataDir,
				TargetModule:    "github.com/target/module",
				ExcludePatterns: []string{"excluded"},
			},
			expectedCount: 2, // module-d is excluded
			expectedRepos: []string{"example/module-a", "example/module-b"},
			expectError:   false,
		},
		{
			name: "includes only directories matching patterns",
			options: DiscoveryOptions{
				WorkspaceDir:    testdataDir,
				TargetModule:    "github.com/target/module",
				IncludePatterns: []string{"module-a"},
			},
			expectedCount: 1, // only module-a matches include pattern
			expectedRepos: []string{"example/module-a"},
			expectError:   false,
		},
		{
			name: "returns empty when no dependents found",
			options: DiscoveryOptions{
				WorkspaceDir: testdataDir,
				TargetModule: "github.com/nonexistent/module",
			},
			expectedCount: 0,
			expectedRepos: []string{},
			expectError:   false,
		},
		{
			name: "respects max depth",
			options: DiscoveryOptions{
				WorkspaceDir: testdataDir,
				TargetModule: "github.com/target/module",
				MaxDepth:     1,
			},
			expectedCount: 2, // excludes module-d which is at depth 2
			expectedRepos: []string{"example/module-a", "example/module-b"},
			expectError:   false,
		},
		{
			name: "returns error for missing workspace",
			options: DiscoveryOptions{
				WorkspaceDir: "",
				TargetModule: "github.com/target/module",
			},
			expectError: true,
		},
		{
			name: "returns error for missing target module",
			options: DiscoveryOptions{
				WorkspaceDir: testdataDir,
				TargetModule: "",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			dependents, err := discovery.DiscoverDependents(ctx, tt.options)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(dependents) != tt.expectedCount {
				t.Errorf("expected %d dependents, got %d", tt.expectedCount, len(dependents))
			}

			// Check that expected repositories are present
			foundRepos := make(map[string]bool)
			for _, dep := range dependents {
				foundRepos[dep.Repository] = true
			}

			for _, expectedRepo := range tt.expectedRepos {
				if !foundRepos[expectedRepo] {
					t.Errorf("expected repository %s not found", expectedRepo)
				}
			}
		})
	}
}

func TestWorkspaceDiscovery_findGoModules(t *testing.T) {
	wd := &workspaceDiscovery{}
	testdataDir := filepath.Join("testdata", "workspace-discovery")

	tests := []struct {
		name          string
		options       DiscoveryOptions
		expectedCount int
		expectedPaths []string
	}{
		{
			name: "finds all go modules",
			options: DiscoveryOptions{
				WorkspaceDir: testdataDir,
			},
			expectedCount: 4, // module-a, module-b, module-c, module-d
			expectedPaths: []string{
				"github.com/example/module-a",
				"github.com/example/module-b",
				"github.com/example/module-c",
				"github.com/example/module-d",
			},
		},
		{
			name: "respects exclude patterns",
			options: DiscoveryOptions{
				WorkspaceDir:    testdataDir,
				ExcludePatterns: []string{"excluded"},
			},
			expectedCount: 3, // excludes module-d
			expectedPaths: []string{
				"github.com/example/module-a",
				"github.com/example/module-b",
				"github.com/example/module-c",
			},
		},
		{
			name: "respects include patterns",
			options: DiscoveryOptions{
				WorkspaceDir:    testdataDir,
				IncludePatterns: []string{"module-a", "module-b"},
			},
			expectedCount: 2, // only module-a and module-b
			expectedPaths: []string{
				"github.com/example/module-a",
				"github.com/example/module-b",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			modules, err := wd.findGoModules(ctx, tt.options)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(modules) != tt.expectedCount {
				t.Errorf("expected %d modules, got %d", tt.expectedCount, len(modules))
			}

			// Check that expected module paths are present
			foundPaths := make(map[string]bool)
			for _, module := range modules {
				foundPaths[module.ModulePath] = true
			}

			for _, expectedPath := range tt.expectedPaths {
				if !foundPaths[expectedPath] {
					t.Errorf("expected module path %s not found", expectedPath)
				}
			}
		})
	}
}

func TestWorkspaceDiscovery_extractModulePath(t *testing.T) {
	wd := &workspaceDiscovery{}

	tests := []struct {
		name         string
		goModContent string
		expected     string
		expectError  bool
	}{
		{
			name:         "extracts simple module path",
			goModContent: "module github.com/example/test\n\ngo 1.21\n",
			expected:     "github.com/example/test",
			expectError:  false,
		},
		{
			name:         "extracts module path with extra content",
			goModContent: "module github.com/example/test\n\ngo 1.21\n\nrequire (\n\tgithub.com/other v1.0.0\n)\n",
			expected:     "github.com/example/test",
			expectError:  false,
		},
		{
			name:         "handles module path with comments",
			goModContent: "// This is a comment\nmodule github.com/example/test // inline comment\n\ngo 1.21\n",
			expected:     "github.com/example/test",
			expectError:  false,
		},
		{
			name:         "returns error for missing module declaration",
			goModContent: "go 1.21\n\nrequire (\n\tgithub.com/other v1.0.0\n)\n",
			expected:     "",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary file
			tmpFile, err := os.CreateTemp("", "go.mod")
			if err != nil {
				t.Fatalf("failed to create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())

			// Write content
			if _, err := tmpFile.WriteString(tt.goModContent); err != nil {
				t.Fatalf("failed to write temp file: %v", err)
			}
			tmpFile.Close()

			// Test extraction
			result, err := wd.extractModulePath(tmpFile.Name())

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestWorkspaceDiscovery_inferRepository(t *testing.T) {
	wd := &workspaceDiscovery{}

	tests := []struct {
		name       string
		modulePath string
		expected   string
	}{
		{
			name:       "github.com repository",
			modulePath: "github.com/user/repo",
			expected:   "user/repo",
		},
		{
			name:       "github.com repository with subpath",
			modulePath: "github.com/user/repo/v2",
			expected:   "user/repo",
		},
		{
			name:       "gitlab.com repository",
			modulePath: "gitlab.com/user/repo",
			expected:   "user/repo",
		},
		{
			name:       "bitbucket.org repository",
			modulePath: "bitbucket.org/user/repo",
			expected:   "user/repo",
		},
		{
			name:       "custom domain fallback",
			modulePath: "example.com/user/repo",
			expected:   "example.com/user/repo",
		},
		{
			name:       "short module path fallback",
			modulePath: "local/module",
			expected:   "local/module",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wd.inferRepository(tt.modulePath)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestWorkspaceDiscovery_parseGoModForDependency(t *testing.T) {
	wd := &workspaceDiscovery{}

	tests := []struct {
		name         string
		goModContent string
		targetModule string
		expected     bool
		expectError  bool
	}{
		{
			name: "finds single-line require dependency",
			goModContent: `module test

go 1.21

require github.com/target/module v1.0.0`,
			targetModule: "github.com/target/module",
			expected:     true,
			expectError:  false,
		},
		{
			name: "finds multi-line require dependency",
			goModContent: `module test

go 1.21

require (
	github.com/target/module v1.0.0
	github.com/other/dep v2.0.0
)`,
			targetModule: "github.com/target/module",
			expected:     true,
			expectError:  false,
		},
		{
			name: "does not find missing dependency",
			goModContent: `module test

go 1.21

require github.com/other/module v1.0.0`,
			targetModule: "github.com/target/module",
			expected:     false,
			expectError:  false,
		},
		{
			name: "handles empty require block",
			goModContent: `module test

go 1.21

require (
)`,
			targetModule: "github.com/target/module",
			expected:     false,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory and go.mod file
			tmpDir, err := os.MkdirTemp("", "test-module")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			goModPath := filepath.Join(tmpDir, "go.mod")
			if err := os.WriteFile(goModPath, []byte(tt.goModContent), 0644); err != nil {
				t.Fatalf("failed to write go.mod: %v", err)
			}

			// Test parseGoModForDependency
			result, err := wd.parseGoModForDependency(tmpDir, tt.targetModule)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestWorkspaceDiscovery_shouldIncludeDirectory(t *testing.T) {
	wd := &workspaceDiscovery{}
	workspaceDir := "/tmp/workspace"

	tests := []struct {
		name     string
		dirPath  string
		options  DiscoveryOptions
		expected bool
	}{
		{
			name:    "includes all by default",
			dirPath: "/tmp/workspace/module-a",
			options: DiscoveryOptions{
				WorkspaceDir: workspaceDir,
			},
			expected: true,
		},
		{
			name:    "excludes matching exclude pattern",
			dirPath: "/tmp/workspace/excluded/module",
			options: DiscoveryOptions{
				WorkspaceDir:    workspaceDir,
				ExcludePatterns: []string{"excluded"},
			},
			expected: false,
		},
		{
			name:    "includes matching include pattern",
			dirPath: "/tmp/workspace/module-a",
			options: DiscoveryOptions{
				WorkspaceDir:    workspaceDir,
				IncludePatterns: []string{"module-*"},
			},
			expected: true,
		},
		{
			name:    "excludes non-matching include pattern",
			dirPath: "/tmp/workspace/other",
			options: DiscoveryOptions{
				WorkspaceDir:    workspaceDir,
				IncludePatterns: []string{"module-*"},
			},
			expected: false,
		},
		{
			name:    "exclude takes precedence over include",
			dirPath: "/tmp/workspace/module-excluded",
			options: DiscoveryOptions{
				WorkspaceDir:    workspaceDir,
				IncludePatterns: []string{"module-*"},
				ExcludePatterns: []string{"*excluded*"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wd.shouldIncludeDirectory(tt.dirPath, tt.options)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestWorkspaceDiscovery_ResolveVersion(t *testing.T) {
	discovery := NewWorkspaceDiscovery()
	ctx := context.Background()

	tests := []struct {
		name                string
		options             VersionResolutionOptions
		expectError         bool
		expectedSource      VersionResolutionSource
		skipTest            bool // Skip tests that require network access
		validateVersionFunc func(string) bool
	}{
		{
			name: "requires target module",
			options: VersionResolutionOptions{
				WorkspaceDir: "/tmp",
				Strategy:     VersionResolutionLocal,
			},
			expectError: true,
		},
		{
			name: "requires workspace directory",
			options: VersionResolutionOptions{
				TargetModule: "github.com/example/module",
				Strategy:     VersionResolutionLocal,
			},
			expectError: true,
		},
		{
			name: "latest strategy requires network access",
			options: VersionResolutionOptions{
				WorkspaceDir:       "/tmp",
				TargetModule:       "github.com/example/module",
				Strategy:           VersionResolutionLatest,
				AllowNetworkAccess: false,
			},
			expectError: true,
		},
		{
			name: "unsupported strategy fails",
			options: VersionResolutionOptions{
				WorkspaceDir: "/tmp",
				TargetModule: "github.com/example/module",
				Strategy:     "unsupported",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipTest {
				t.Skip("Skipping test that requires network access or external dependencies")
			}

			result, err := discovery.ResolveVersion(ctx, tt.options)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Errorf("expected result but got nil")
				return
			}

			if tt.expectedSource != "" && result.Source != tt.expectedSource {
				t.Errorf("expected source %s, got %s", tt.expectedSource, result.Source)
			}

			if tt.validateVersionFunc != nil && !tt.validateVersionFunc(result.Version) {
				t.Errorf("version validation failed for: %s", result.Version)
			}
		})
	}
}

func TestVersionResolutionStrategy_Values(t *testing.T) {
	// Test that the constants are properly defined
	strategies := []VersionResolutionStrategy{
		VersionResolutionLocal,
		VersionResolutionLatest,
		VersionResolutionAuto,
	}

	expectedValues := []string{"local", "latest", "auto"}

	for i, strategy := range strategies {
		if string(strategy) != expectedValues[i] {
			t.Errorf("expected strategy %d to be %s, got %s", i, expectedValues[i], string(strategy))
		}
	}
}

func TestVersionResolutionSource_Values(t *testing.T) {
	// Test that the constants are properly defined
	sources := []VersionResolutionSource{
		VersionSourceLocal,
		VersionSourceNetwork,
		VersionSourceFallback,
	}

	expectedValues := []string{"local", "network", "fallback"}

	for i, source := range sources {
		if string(source) != expectedValues[i] {
			t.Errorf("expected source %d to be %s, got %s", i, expectedValues[i], string(source))
		}
	}
}
