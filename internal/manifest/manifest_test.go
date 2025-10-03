package manifest_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/pkg/testsupport"
	"github.com/google/go-cmp/cmp"
	"gopkg.in/yaml.v3"
)

func TestLoader_Load_GeneratesExpectedManifest(t *testing.T) {

	loader := manifest.NewLoader()
	manifestPath := filepath.Join("testdata", "basic.yaml")
	got, err := loader.Load(manifestPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	var want manifest.Manifest
	if err := testsupport.LoadGolden(filepath.Join("testdata", "basic_manifest.json"), &want); err != nil {
		t.Fatalf("load golden: %v", err)
	}

	if diff := cmp.Diff(&want, got); diff != "" {
		t.Fatalf("manifest mismatch (-want +got):\n%s", diff)
	}
}

func TestLoader_Load_ModuleAndDependentOverrides(t *testing.T) {
	loader := manifest.NewLoader()
	manifestPath := filepath.Join("testdata", "basic.yaml")
	m, err := loader.Load(manifestPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if m.Module == nil {
		t.Fatalf("expected module config to be populated")
	}

	if m.Module.Module != "github.com/goliatone/cascade" {
		t.Fatalf("unexpected module path: %s", m.Module.Module)
	}

	if len(m.Module.Tests) == 0 {
		t.Fatalf("expected module tests to be normalized")
	}

	depOverride, ok := m.Dependents["github.com/goliatone/go-errors"]
	if !ok {
		t.Fatalf("expected dependent override for github.com/goliatone/go-errors")
	}

	if len(depOverride.Tests) != 1 || depOverride.Tests[0].Cmd[0] != "task" {
		t.Fatalf("unexpected dependent override tests: %#v", depOverride.Tests)
	}

	if depOverride.Timeout == 0 {
		t.Fatalf("expected dependent override timeout to be set")
	}
}

func TestValidate_BasicManifest(t *testing.T) {

	var m manifest.Manifest
	if err := testsupport.LoadGolden(filepath.Join("testdata", "basic_manifest.json"), &m); err != nil {
		t.Fatalf("load golden: %v", err)
	}

	if err := manifest.Validate(&m); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestLoader_Load_ErrorCases(t *testing.T) {
	loader := manifest.NewLoader()

	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{
			name:    "missing file",
			path:    "testdata/nonexistent.yaml",
			wantErr: "failed to load",
		},
		{
			name:    "invalid yaml",
			path:    "testdata/invalid.yaml",
			wantErr: "failed to parse YAML",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loader.Load(tt.path)
			if err == nil {
				t.Fatalf("Load() expected error but got none")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Load() error = %v, want to contain %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoader_Generate_NotSupported(t *testing.T) {
	loader := manifest.NewLoader()
	_, err := loader.Generate("/tmp")
	if err == nil {
		t.Fatalf("Generate() expected error but got none")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("Generate() error = %v, want to contain 'not supported'", err)
	}
}

func TestFindModule_ReturnsMatch(t *testing.T) {

	var m manifest.Manifest
	if err := testsupport.LoadGolden(filepath.Join("testdata", "basic_manifest.json"), &m); err != nil {
		t.Fatalf("load golden: %v", err)
	}

	module, err := manifest.FindModule(&m, "go-errors")
	if err != nil {
		t.Fatalf("FindModule error: %v", err)
	}

	if module.Module != "github.com/goliatone/go-errors" {
		t.Fatalf("unexpected module path: %s", module.Module)
	}
}

func TestFindModule_NotFound(t *testing.T) {
	var m manifest.Manifest
	if err := testsupport.LoadGolden(filepath.Join("testdata", "basic_manifest.json"), &m); err != nil {
		t.Fatalf("load golden: %v", err)
	}

	_, err := manifest.FindModule(&m, "nonexistent-module")
	if err == nil {
		t.Fatalf("FindModule expected error but got none")
	}

	if !strings.Contains(err.Error(), "module not found") {
		t.Fatalf("FindModule error = %v, want to contain 'module not found'", err)
	}
}

func TestFindModuleConfig_ReturnsMatch(t *testing.T) {
	var m manifest.Manifest
	if err := testsupport.LoadGolden(filepath.Join("testdata", "basic_manifest.json"), &m); err != nil {
		t.Fatalf("load golden: %v", err)
	}

	config, ok := manifest.FindModuleConfig(&m, "github.com/goliatone/cascade")
	if !ok {
		t.Fatalf("FindModuleConfig expected to find module config")
	}

	if config.Branch != "main" {
		t.Fatalf("unexpected module branch: %s", config.Branch)
	}
}

func TestFindModuleConfig_NotFound(t *testing.T) {
	var m manifest.Manifest
	if err := testsupport.LoadGolden(filepath.Join("testdata", "basic_manifest.json"), &m); err != nil {
		t.Fatalf("load golden: %v", err)
	}

	config, ok := manifest.FindModuleConfig(&m, "github.com/example/missing")
	if ok {
		t.Fatalf("FindModuleConfig should not find missing module, config=%#v", config)
	}

	if config != nil {
		t.Fatalf("FindModuleConfig should return nil config when not found")
	}

	if cfg, ok := manifest.FindModuleConfig(nil, "github.com/example/missing"); ok || cfg != nil {
		t.Fatalf("FindModuleConfig with nil manifest should return nil, false")
	}
}

func TestLoadDependentOverrides(t *testing.T) {
	tests := []struct {
		name       string
		repoDir    string
		modulePath string
		wantNil    bool
		wantErr    string
		assert     func(t *testing.T, cfg *manifest.DependentConfig)
	}{
		{
			name:       "override present",
			repoDir:    filepath.Join("testdata", "dependents", "with_override"),
			modulePath: "github.com/goliatone/go-errors",
			assert: func(t *testing.T, cfg *manifest.DependentConfig) {
				if cfg.Branch != "release" {
					t.Fatalf("expected branch 'release', got %s", cfg.Branch)
				}
				if len(cfg.Tests) != 1 || len(cfg.Tests[0].Cmd) == 0 || cfg.Tests[0].Cmd[0] != "task" {
					t.Fatalf("unexpected tests: %#v", cfg.Tests)
				}
			},
		},
		{
			name:       "missing module entry",
			repoDir:    filepath.Join("testdata", "dependents", "missing_entry"),
			modulePath: "github.com/goliatone/go-errors",
			wantNil:    true,
		},
		{
			name: "no manifest",
			repoDir: func() string {
				dir := t.TempDir()
				return dir
			}(),
			modulePath: "github.com/goliatone/go-errors",
			wantNil:    true,
		},
		{
			name:       "invalid manifest",
			repoDir:    filepath.Join("testdata", "dependents", "invalid"),
			modulePath: "github.com/goliatone/go-errors",
			wantErr:    "failed to parse YAML",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := manifest.LoadDependentOverrides(context.Background(), tt.repoDir, tt.modulePath)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %v does not contain %q", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantNil {
				if cfg != nil {
					t.Fatalf("expected nil override, got %#v", cfg)
				}
				return
			}

			if cfg == nil {
				t.Fatalf("expected override to be returned")
			}

			if tt.assert != nil {
				ttAssert := tt.assert
				ttAssert(t, cfg)
			}
		})
	}
}

func TestLoadDependentManifest(t *testing.T) {
	missingDir := t.TempDir()

	tests := []struct {
		name    string
		repoDir string
		exists  bool
		wantErr string
	}{
		{
			name:    "manifest present",
			repoDir: filepath.Join("testdata", "dependents", "with_override"),
			exists:  true,
		},
		{
			name:    "manifest missing",
			repoDir: missingDir,
			exists:  false,
		},
		{
			name:    "invalid manifest",
			repoDir: filepath.Join("testdata", "dependents", "invalid"),
			wantErr: "failed to parse YAML",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := manifest.LoadDependentManifest(context.Background(), tt.repoDir)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %v does not contain %q", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.exists && m == nil {
				t.Fatalf("expected manifest, got nil")
			}
			if !tt.exists && m != nil {
				t.Fatalf("expected nil manifest, got %#v", m)
			}
		})
	}
}

func TestExpandDefaults_EmptyDependent(t *testing.T) {
	defaults := manifest.Defaults{
		Branch: "main",
		Tests: []manifest.Command{
			{Cmd: []string{"go", "test", "./..."}, Dir: ""},
		},
		Labels:         []string{"automation:cascade"},
		CommitTemplate: "chore(deps): bump {{ module }} to {{ version }}",
		Notifications: manifest.Notifications{
			SlackChannel: "#releases",
		},
		PR: manifest.PRConfig{
			TitleTemplate: "chore: update dependencies",
			Reviewers:     []string{"octocat"},
		},
	}

	dependent := manifest.Dependent{
		Repo:       "test/repo",
		Module:     "test/module",
		ModulePath: ".",
	}

	result := manifest.ExpandDefaults(dependent, defaults)

	if result.Branch != "main" {
		t.Fatalf("expected branch 'main', got %s", result.Branch)
	}
	if len(result.Tests) != 1 {
		t.Fatalf("expected 1 test, got %d", len(result.Tests))
	}
	if len(result.Labels) != 1 {
		t.Fatalf("expected 1 label, got %d", len(result.Labels))
	}
	if result.Notifications.SlackChannel != "#releases" {
		t.Fatalf("expected slack channel '#releases', got %s", result.Notifications.SlackChannel)
	}
	if result.PR.TitleTemplate != "chore: update dependencies" {
		t.Fatalf("expected PR title template 'chore: update dependencies', got %s", result.PR.TitleTemplate)
	}
}

func TestExpandDefaults_PartialDependent(t *testing.T) {
	defaults := manifest.Defaults{
		Branch: "main",
		Tests: []manifest.Command{
			{Cmd: []string{"go", "test", "./..."}, Dir: ""},
		},
		Labels: []string{"automation:cascade"},
		Notifications: manifest.Notifications{
			SlackChannel: "#releases",
		},
		PR: manifest.PRConfig{
			TitleTemplate: "chore: update dependencies",
			Reviewers:     []string{"octocat"},
		},
	}

	dependent := manifest.Dependent{
		Repo:       "test/repo",
		Module:     "test/module",
		ModulePath: ".",
		Branch:     "develop", // Override default
		Tests: []manifest.Command{
			{Cmd: []string{"task", "test"}, Dir: ""},
		},
		Notifications: manifest.Notifications{
			SlackChannel: "#custom-channel", // Override default
		},
	}

	result := manifest.ExpandDefaults(dependent, defaults)

	if result.Branch != "develop" {
		t.Fatalf("expected branch 'develop', got %s", result.Branch)
	}
	if len(result.Tests) != 2 { // defaults + dependent
		t.Fatalf("expected 2 tests, got %d", len(result.Tests))
	}
	if result.Notifications.SlackChannel != "#custom-channel" {
		t.Fatalf("expected slack channel '#custom-channel', got %s", result.Notifications.SlackChannel)
	}
	if result.PR.TitleTemplate != "chore: update dependencies" {
		t.Fatalf("expected PR title template from defaults, got %s", result.PR.TitleTemplate)
	}
}

func TestManifestYAMLOmitsEmptyDependentFields(t *testing.T) {
	manifestData := &manifest.Manifest{
		ManifestVersion: 1,
		Modules: []manifest.Module{
			{
				Name:   "go-errors",
				Module: "github.com/goliatone/go-errors",
				Repo:   "goliatone/go-errors",
				Dependents: []manifest.Dependent{
					{
						Repo:       "goliatone/go-logger",
						Module:     "github.com/goliatone/go-logger",
						ModulePath: ".",
					},
				},
			},
		},
	}

	data, err := yaml.Marshal(manifestData)
	if err != nil {
		t.Fatalf("yaml marshal: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("yaml unmarshal: %v", err)
	}

	modules, ok := parsed["modules"].([]any)
	if !ok || len(modules) == 0 {
		t.Fatalf("expected modules in YAML, got %#v", parsed["modules"])
	}
	module, ok := modules[0].(map[string]any)
	if !ok {
		t.Fatalf("expected module map, got %T", modules[0])
	}
	deps, ok := module["dependents"].([]any)
	if !ok || len(deps) != 1 {
		t.Fatalf("expected one dependent, got %#v", module["dependents"])
	}
	dep, ok := deps[0].(map[string]any)
	if !ok {
		t.Fatalf("expected dependent map, got %T", deps[0])
	}

	forbidden := []string{"branch", "tests", "extra_commands", "labels", "notifications", "pr", "canary", "skip", "env", "timeout"}
	for _, key := range forbidden {
		if _, exists := dep[key]; exists {
			t.Fatalf("expected dependent field %q to be omitted, map=%#v", key, dep)
		}
	}
}

func TestValidate_InvalidVersion(t *testing.T) {
	loader := manifest.NewLoader()
	m, err := loader.Load(filepath.Join("testdata", "invalid_version.yaml"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	err = manifest.Validate(m)
	if err == nil {
		t.Fatalf("Validate expected error but got none")
	}

	if !strings.Contains(err.Error(), "unsupported manifest version: 2") {
		t.Fatalf("Validate error = %v, want to contain 'unsupported manifest version: 2'", err)
	}
}

func TestValidate_SchemaErrors(t *testing.T) {
	loader := manifest.NewLoader()
	m, err := loader.Load(filepath.Join("testdata", "invalid_schema.yaml"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	err = manifest.Validate(m)
	if err == nil {
		t.Fatalf("Validate expected error but got none")
	}

	errorStr := err.Error()
	expectedErrors := []string{
		"module.module cannot be empty",
		"module.module_path cannot be empty",
		"name cannot be empty",
		"repo cannot be empty",
		"duplicate module name: duplicate-name",
		"dependent[0] repo cannot be empty",
		"module cannot be empty",
		"module_path cannot be empty",
		"duplicate dependent repo: example/dependent-1",
		"dependents key cannot be empty",
	}

	for _, expected := range expectedErrors {
		if !strings.Contains(errorStr, expected) {
			t.Errorf("Validate error = %v, want to contain '%s'", err, expected)
		}
	}
}

func TestValidate_CycleDetection(t *testing.T) {
	loader := manifest.NewLoader()
	m, err := loader.Load(filepath.Join("testdata", "invalid_cycle.yaml"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	err = manifest.Validate(m)
	if err == nil {
		t.Fatalf("Validate expected error but got none")
	}

	if !strings.Contains(err.Error(), "dependency cycle detected") {
		t.Fatalf("Validate error = %v, want to contain 'dependency cycle detected'", err)
	}
}

func TestValidate_NilManifest(t *testing.T) {
	err := manifest.Validate(nil)
	if err == nil {
		t.Fatalf("Validate expected error but got none")
	}

	if !strings.Contains(err.Error(), "manifest cannot be nil") {
		t.Fatalf("Validate error = %v, want to contain 'manifest cannot be nil'", err)
	}
}

func TestValidate_ValidationErrorType(t *testing.T) {
	// Test that ValidationError produces the expected format
	loader := manifest.NewLoader()
	m, err := loader.Load(filepath.Join("testdata", "invalid_schema.yaml"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	err = manifest.Validate(m)
	if err == nil {
		t.Fatalf("Validate expected error but got none")
	}

	if !strings.Contains(err.Error(), "validation failed with") {
		t.Fatalf("Validate error should be ValidationError type, got: %v", err)
	}
}

// Error Type Tests

func TestErrorTypes_LoadError(t *testing.T) {
	loader := manifest.NewLoader()
	_, err := loader.Load("testdata/nonexistent.yaml")

	if err == nil {
		t.Fatalf("Load() expected error but got none")
	}

	// Test that we can detect the error type
	if !manifest.IsLoadError(err) {
		t.Fatalf("expected LoadError, got %T", err)
	}

	// Test that the error message includes the path
	if !strings.Contains(err.Error(), "testdata/nonexistent.yaml") {
		t.Fatalf("error message should include file path, got: %v", err)
	}

	// Test that we can unwrap to get the original error
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("expected to unwrap to os.PathError, got %T", err)
	}
}

func TestErrorTypes_ParseError(t *testing.T) {
	loader := manifest.NewLoader()
	_, err := loader.Load("testdata/invalid.yaml")

	if err == nil {
		t.Fatalf("Load() expected error but got none")
	}

	// Test that we can detect the error type
	if !manifest.IsParseError(err) {
		t.Fatalf("expected ParseError, got %T", err)
	}

	// Test that the error message includes the path
	if !strings.Contains(err.Error(), "testdata/invalid.yaml") {
		t.Fatalf("error message should include file path, got: %v", err)
	}
}

func TestErrorTypes_ModuleNotFoundError(t *testing.T) {
	var m manifest.Manifest
	if err := testsupport.LoadGolden(filepath.Join("testdata", "basic_manifest.json"), &m); err != nil {
		t.Fatalf("load golden: %v", err)
	}

	_, err := manifest.FindModule(&m, "nonexistent-module")

	if err == nil {
		t.Fatalf("FindModule() expected error but got none")
	}

	// Test that we can detect the error type
	if !manifest.IsModuleNotFound(err) {
		t.Fatalf("expected ModuleNotFoundError, got %T", err)
	}

	// Test helper function to extract module name
	moduleName, ok := manifest.GetModuleName(err)
	if !ok {
		t.Fatalf("expected to extract module name from error")
	}
	if moduleName != "nonexistent-module" {
		t.Fatalf("expected module name 'nonexistent-module', got %s", moduleName)
	}

	// Test that the error message includes the module name
	if !strings.Contains(err.Error(), "nonexistent-module") {
		t.Fatalf("error message should include module name, got: %v", err)
	}
}

func TestErrorTypes_ValidationError(t *testing.T) {
	// Test nil manifest validation
	err := manifest.Validate(nil)

	if err == nil {
		t.Fatalf("Validate() expected error but got none")
	}

	// Test that we can detect the error type
	if !manifest.IsValidationError(err) {
		t.Fatalf("expected ValidationError, got %T", err)
	}

	// Test helper function to extract validation issues
	issues, ok := manifest.GetValidationIssues(err)
	if !ok {
		t.Fatalf("expected to extract validation issues from error")
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 validation issue, got %d", len(issues))
	}
	if issues[0] != "manifest cannot be nil" {
		t.Fatalf("expected 'manifest cannot be nil', got %s", issues[0])
	}
}

func TestErrorTypes_GenerateError(t *testing.T) {
	loader := manifest.NewLoader()
	_, err := loader.Generate("/tmp/test")

	if err == nil {
		t.Fatalf("Generate() expected error but got none")
	}

	// Test that we can detect the error type
	if !manifest.IsGenerateError(err) {
		t.Fatalf("expected GenerateError, got %T", err)
	}

	// Test that the error message includes the work directory
	if !strings.Contains(err.Error(), "/tmp/test") {
		t.Fatalf("error message should include work directory, got: %v", err)
	}
}

func TestErrorHelpers_NegativeCases(t *testing.T) {
	// Test that error helpers return false for wrong error types
	regularErr := errors.New("regular error")

	if manifest.IsLoadError(regularErr) {
		t.Fatalf("IsLoadError should return false for regular error")
	}
	if manifest.IsParseError(regularErr) {
		t.Fatalf("IsParseError should return false for regular error")
	}
	if manifest.IsModuleNotFound(regularErr) {
		t.Fatalf("IsModuleNotFound should return false for regular error")
	}
	if manifest.IsValidationError(regularErr) {
		t.Fatalf("IsValidationError should return false for regular error")
	}
	if manifest.IsGenerateError(regularErr) {
		t.Fatalf("IsGenerateError should return false for regular error")
	}

	// Test helper functions with wrong error types
	if _, ok := manifest.GetModuleName(regularErr); ok {
		t.Fatalf("GetModuleName should return false for regular error")
	}
	if _, ok := manifest.GetValidationIssues(regularErr); ok {
		t.Fatalf("GetValidationIssues should return false for regular error")
	}
}
