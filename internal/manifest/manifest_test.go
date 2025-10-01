package manifest_test

import (
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
		Defaults:        manifest.Defaults{},
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
	content := string(data)
	if !strings.Contains(content, "repo: goliatone/go-logger") {
		t.Fatalf("expected repo to be present in YAML, got:\n%s", content)
	}

	for _, field := range []string{"branch:", "tests:", "extra_commands:", "labels:", "notifications:", "pr:", "canary:", "skip:", "env:", "timeout:"} {
		if strings.Contains(content, field) {
			t.Fatalf("expected optional field %q to be omitted, YAML:\n%s", field, content)
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
		"name cannot be empty",
		"repo cannot be empty",
		"duplicate module name: duplicate-name",
		"dependent[0] repo cannot be empty",
		"module cannot be empty",
		"module_path cannot be empty",
		"duplicate dependent repo: example/dependent-1",
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
