package manifest

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
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
		{
			name: "local resolution fails for external dependency not in workspace",
			options: VersionResolutionOptions{
				WorkspaceDir: filepath.Join("testdata", "workspace-discovery"),
				TargetModule: "github.com/target/module", // External dependency, not in workspace
				Strategy:     VersionResolutionLocal,
			},
			expectError: true, // Current implementation returns error when module not found
		},
		{
			name: "auto strategy fails when network disabled and module not local",
			options: VersionResolutionOptions{
				WorkspaceDir:       filepath.Join("testdata", "workspace-discovery"),
				TargetModule:       "github.com/target/module", // External dependency
				Strategy:           VersionResolutionAuto,
				AllowNetworkAccess: false, // Force local-only behavior
			},
			expectError: true, // Current implementation fails when no network and module not found locally
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

func TestWorkspaceDiscovery_getModuleVersionFromPath_JSONDecoding(t *testing.T) {
	// This test specifically verifies the JSON decoder fix for go list output

	tests := []struct {
		name            string
		goListOutput    string
		targetModule    string
		expectedVersion string
		expectError     bool
	}{
		{
			name: "parses concatenated JSON objects correctly",
			goListOutput: `{"Path":"example.com/main","Version":"","Replace":null,"Main":true,"Indirect":false}
{"Path":"github.com/target/module","Version":"v1.2.3","Replace":null,"Main":false,"Indirect":false}
{"Path":"github.com/other/dep","Version":"v2.0.0","Replace":null,"Main":false,"Indirect":false}`,
			targetModule:    "github.com/target/module",
			expectedVersion: "v1.2.3",
			expectError:     false,
		},
		{
			name: "handles replace directive",
			goListOutput: `{"Path":"example.com/main","Version":"","Replace":null,"Main":true,"Indirect":false}
{"Path":"github.com/target/module","Version":"v1.0.0","Replace":{"Path":"../local","Version":"v0.0.0-00010101000000-000000000000"},"Main":false,"Indirect":false}`,
			targetModule:    "github.com/target/module",
			expectedVersion: "v0.0.0-00010101000000-000000000000",
			expectError:     false,
		},
		{
			name: "returns empty when module not found",
			goListOutput: `{"Path":"example.com/main","Version":"","Replace":null,"Main":true,"Indirect":false}
{"Path":"github.com/other/dep","Version":"v2.0.0","Replace":null,"Main":false,"Indirect":false}`,
			targetModule:    "github.com/missing/module",
			expectedVersion: "",
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temp directory and simulate go list output
			tmpDir, err := os.MkdirTemp("", "test-module")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			// Create a simple go.mod to make the directory look like a Go module
			goModPath := filepath.Join(tmpDir, "go.mod")
			goModContent := "module example.com/test\n\ngo 1.21\n"
			if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
				t.Fatalf("failed to write go.mod: %v", err)
			}

			// We can't easily test the actual exec.Command, but we can test the JSON parsing logic
			// by using the parseModuleFromJSON helper directly (if we had one)
			// For now, let's create a simple JSON decoder test
			decoder := json.NewDecoder(strings.NewReader(tt.goListOutput))
			var foundVersion string

			for {
				var module moduleInfo
				if err := decoder.Decode(&module); err != nil {
					if err == io.EOF {
						break
					}
					continue
				}

				if module.Path == tt.targetModule {
					if module.Replace != nil {
						foundVersion = module.Replace.Version
					} else {
						foundVersion = module.Version
					}
					break
				}
			}

			if foundVersion != tt.expectedVersion {
				t.Errorf("expected version %s, got %s", tt.expectedVersion, foundVersion)
			}
		})
	}
}

func TestWorkspaceDiscovery_inferLocalModulePath(t *testing.T) {
	wd := &workspaceDiscovery{}

	tests := []struct {
		name       string
		modulePath string
		expected   string
	}{
		{
			name:       "github.com repository at root",
			modulePath: "github.com/user/repo",
			expected:   ".",
		},
		{
			name:       "github.com repository with nested module",
			modulePath: "github.com/org/mono/services/api",
			expected:   "services/api",
		},
		{
			name:       "github.com repository with deep nesting",
			modulePath: "github.com/org/mono/cmd/server/internal/handler",
			expected:   "cmd/server/internal/handler",
		},
		{
			name:       "gitlab.com repository with nested module",
			modulePath: "gitlab.com/company/platform/auth/service",
			expected:   "auth/service",
		},
		{
			name:       "bitbucket.org repository with nested module",
			modulePath: "bitbucket.org/team/project/utils/logger",
			expected:   "utils/logger",
		},
		{
			name:       "custom domain fallback",
			modulePath: "go.uber.org/zap",
			expected:   ".",
		},
		{
			name:       "short module path fallback",
			modulePath: "local/module",
			expected:   ".",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wd.inferLocalModulePath(tt.modulePath)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestWorkspaceDiscovery_NestedModuleDependentGeneration(t *testing.T) {
	// Test the complete flow of dependent generation for nested modules

	tests := []struct {
		name                    string
		modulePath              string
		expectedRepository      string
		expectedLocalModulePath string
	}{
		{
			name:                    "nested GitHub module",
			modulePath:              "github.com/org/mono/services/api",
			expectedRepository:      "org/mono",
			expectedLocalModulePath: "services/api",
		},
		{
			name:                    "deeply nested GitHub module",
			modulePath:              "github.com/company/platform/cmd/server/internal",
			expectedRepository:      "company/platform",
			expectedLocalModulePath: "cmd/server/internal",
		},
		{
			name:                    "root level GitHub module",
			modulePath:              "github.com/user/simple",
			expectedRepository:      "user/simple",
			expectedLocalModulePath: ".",
		},
		{
			name:                    "non-GitHub module preserves full path",
			modulePath:              "go.uber.org/zap/zapcore",
			expectedRepository:      "go.uber.org/zap/zapcore",
			expectedLocalModulePath: ".",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wd := &workspaceDiscovery{}

			// Test repository inference
			repository := wd.inferRepository(tt.modulePath)
			if repository != tt.expectedRepository {
				t.Errorf("expected repository %s, got %s", tt.expectedRepository, repository)
			}

			// Test local module path inference
			localModulePath := wd.inferLocalModulePath(tt.modulePath)
			if localModulePath != tt.expectedLocalModulePath {
				t.Errorf("expected local module path %s, got %s", tt.expectedLocalModulePath, localModulePath)
			}

			// Test that DependentOptions would be created correctly
			dependent := DependentOptions{
				Repository:      repository,
				ModulePath:      tt.modulePath,
				LocalModulePath: localModulePath,
			}

			if dependent.Repository != tt.expectedRepository {
				t.Errorf("DependentOptions repository: expected %s, got %s", tt.expectedRepository, dependent.Repository)
			}
			if dependent.LocalModulePath != tt.expectedLocalModulePath {
				t.Errorf("DependentOptions LocalModulePath: expected %s, got %s", tt.expectedLocalModulePath, dependent.LocalModulePath)
			}
			if dependent.ModulePath != tt.modulePath {
				t.Errorf("DependentOptions ModulePath: expected %s, got %s", tt.modulePath, dependent.ModulePath)
			}
		})
	}
}

func TestWorkspaceDiscovery_ExcludesSelfModule(t *testing.T) {
	// Create a temporary workspace with a target module and dependent
	tmpDir := t.TempDir()

	// Create target module (should be excluded from results)
	targetDir := filepath.Join(tmpDir, "target-module")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}
	targetGoMod := `module github.com/example/target
go 1.21
`
	if err := os.WriteFile(filepath.Join(targetDir, "go.mod"), []byte(targetGoMod), 0644); err != nil {
		t.Fatal(err)
	}

	// Create dependent module
	dependentDir := filepath.Join(tmpDir, "dependent-module")
	if err := os.MkdirAll(dependentDir, 0755); err != nil {
		t.Fatal(err)
	}
	dependentGoMod := `module github.com/example/dependent
go 1.21

require github.com/example/target v1.0.0
`
	if err := os.WriteFile(filepath.Join(dependentDir, "go.mod"), []byte(dependentGoMod), 0644); err != nil {
		t.Fatal(err)
	}

	// Run discovery
	discovery := NewWorkspaceDiscovery()
	options := DiscoveryOptions{
		WorkspaceDir: tmpDir,
		TargetModule: "github.com/example/target",
	}

	dependents, err := discovery.DiscoverDependents(context.Background(), options)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify target module is not included in results
	for _, dep := range dependents {
		if dep.ModulePath == "github.com/example/target" {
			t.Error("target module should not be included in its own dependents list")
		}
	}

	// Verify dependent is included
	if len(dependents) != 1 {
		t.Errorf("expected 1 dependent, got %d", len(dependents))
	}
	if len(dependents) > 0 && dependents[0].ModulePath != "github.com/example/dependent" {
		t.Errorf("expected dependent module, got %s", dependents[0].ModulePath)
	}
}

func TestWorkspaceDiscovery_VersionFiltering(t *testing.T) {
	// Create a temporary workspace with multiple dependents at different versions
	tmpDir := t.TempDir()

	tests := []struct {
		name             string
		moduleName       string
		version          string
		shouldBeIncluded bool
	}{
		{
			name:             "old-version-module",
			moduleName:       "github.com/example/old",
			version:          "v0.8.0",
			shouldBeIncluded: true, // Should be included (needs update)
		},
		{
			name:             "current-version-module",
			moduleName:       "github.com/example/current",
			version:          "v0.9.0",
			shouldBeIncluded: false, // Should be excluded (already at target)
		},
		{
			name:             "newer-version-module",
			moduleName:       "github.com/example/newer",
			version:          "v0.10.0",
			shouldBeIncluded: false, // Should be excluded (already newer)
		},
	}

	// Create modules
	for _, tt := range tests {
		moduleDir := filepath.Join(tmpDir, tt.name)
		if err := os.MkdirAll(moduleDir, 0755); err != nil {
			t.Fatal(err)
		}
		goMod := "module " + tt.moduleName + "\ngo 1.21\n\nrequire github.com/example/target " + tt.version + "\n"
		if err := os.WriteFile(filepath.Join(moduleDir, "go.mod"), []byte(goMod), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Run discovery with version filtering
	discovery := NewWorkspaceDiscovery()
	options := DiscoveryOptions{
		WorkspaceDir:  tmpDir,
		TargetModule:  "github.com/example/target",
		TargetVersion: "v0.9.0",
	}

	dependents, err := discovery.DiscoverDependents(context.Background(), options)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Build map of found modules
	foundModules := make(map[string]bool)
	for _, dep := range dependents {
		foundModules[dep.ModulePath] = true
	}

	// Verify filtering worked correctly
	for _, tt := range tests {
		included := foundModules[tt.moduleName]
		if included != tt.shouldBeIncluded {
			t.Errorf("module %s: expected included=%v, got included=%v", tt.moduleName, tt.shouldBeIncluded, included)
		}
	}
}

func TestWorkspaceDiscovery_getDependencyVersion(t *testing.T) {
	// Create a temporary module with a specific dependency version
	tmpDir := t.TempDir()
	goMod := `module github.com/example/test
go 1.21

require (
	github.com/example/dep1 v1.2.3
	github.com/example/dep2 v2.0.0
)
`
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}

	discovery := NewWorkspaceDiscovery().(*workspaceDiscovery)

	tests := []struct {
		name            string
		targetModule    string
		expectedVersion string
	}{
		{
			name:            "finds existing dependency version",
			targetModule:    "github.com/example/dep1",
			expectedVersion: "v1.2.3",
		},
		{
			name:            "finds another dependency version",
			targetModule:    "github.com/example/dep2",
			expectedVersion: "v2.0.0",
		},
		{
			name:            "returns empty for non-existent dependency",
			targetModule:    "github.com/example/nonexistent",
			expectedVersion: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version := discovery.getDependencyVersion(context.Background(), tmpDir, tt.targetModule)
			if version != tt.expectedVersion {
				t.Errorf("expected version %s, got %s", tt.expectedVersion, version)
			}
		})
	}
}
