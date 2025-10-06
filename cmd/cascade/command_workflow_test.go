package main

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goliatone/cascade/internal/broker"
	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/internal/state"
	"github.com/goliatone/cascade/pkg/config"
	"github.com/goliatone/cascade/pkg/di"
)

type testLogger struct{}

func (testLogger) Debug(string, ...any) {}
func (testLogger) Info(string, ...any)  {}
func (testLogger) Warn(string, ...any)  {}
func (testLogger) Error(string, ...any) {}

type testDIContainer struct {
	cfg    *config.Config
	logger di.Logger
}

func (c *testDIContainer) Manifest() manifest.Loader             { return nil }
func (c *testDIContainer) ManifestGenerator() manifest.Generator { return nil }
func (c *testDIContainer) Planner() planner.Planner              { return nil }
func (c *testDIContainer) Executor() executor.Executor           { return nil }
func (c *testDIContainer) Broker() broker.Broker                 { return nil }
func (c *testDIContainer) BrokerWithManifestNotifications(*di.ManifestNotifications) (broker.Broker, error) {
	return nil, nil
}
func (c *testDIContainer) State() state.Manager     { return nil }
func (c *testDIContainer) Config() *config.Config   { return c.cfg }
func (c *testDIContainer) Logger() di.Logger        { return c.logger }
func (c *testDIContainer) HTTPClient() *http.Client { return nil }
func (c *testDIContainer) Close() error             { return nil }

func prepareWorkflowCommandTest(t *testing.T) (*config.Config, string) {
	t.Helper()

	tempDir := t.TempDir()
	modulePath := "github.com/test/workflow"
	goModContent := "module " + modulePath + "\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goModContent), 0o644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	originalContainer := container

	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	cfg := &config.Config{Module: modulePath}
	container = &testDIContainer{
		cfg:    cfg,
		logger: testLogger{},
	}

	t.Cleanup(func() {
		container = originalContainer
		if err := os.Chdir(originalWD); err != nil {
			t.Errorf("failed to restore working directory: %v", err)
		}
	})

	return cfg, tempDir
}

func execTestGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v (output: %s)", args, err, string(output))
	}
}

func TestBuildWorkflowTemplateData_Defaults(t *testing.T) {
	modulePath := "github.com/example/project"
	outputPath := "workflow.yml"

	data, err := buildWorkflowTemplateData(workflowGenerateRequest{}, nil, modulePath, "", outputPath)
	if err != nil {
		t.Fatalf("buildWorkflowTemplateData returned error: %v", err)
	}

	if data.ModulePath != modulePath {
		t.Errorf("expected module path %q, got %q", modulePath, data.ModulePath)
	}
	if data.RepoOwner != "example" || data.RepoName != "project" {
		t.Errorf("expected repo owner/repo example/project, got %s/%s", data.RepoOwner, data.RepoName)
	}
	if data.GoVersion != defaultWorkflowGoVersion {
		t.Errorf("expected Go version %q, got %q", defaultWorkflowGoVersion, data.GoVersion)
	}
	if data.TagPattern != defaultWorkflowTagPattern {
		t.Errorf("expected tag pattern %q, got %q", defaultWorkflowTagPattern, data.TagPattern)
	}
	if data.StateDir != defaultWorkflowStateDir {
		t.Errorf("expected state dir %q, got %q", defaultWorkflowStateDir, data.StateDir)
	}
	if data.BinaryPath != defaultWorkflowBinaryPath {
		t.Errorf("expected binary path %q, got %q", defaultWorkflowBinaryPath, data.BinaryPath)
	}
	if data.Secrets.GitHubToken != defaultGitHubTokenEnv || data.Secrets.SlackToken != defaultSlackTokenEnv {
		t.Errorf("unexpected secrets defaults: %+v", data.Secrets)
	}
}

func TestBuildWorkflowTemplateData_GitFallback(t *testing.T) {
	tempDir := t.TempDir()
	execTestGitCommand(t, tempDir, "init")
	execTestGitCommand(t, tempDir, "remote", "add", "origin", "git@github.com:owner/repo.git")

	data, err := buildWorkflowTemplateData(workflowGenerateRequest{}, nil, "example.com/internal/project", tempDir, "workflow.yml")
	if err != nil {
		t.Fatalf("buildWorkflowTemplateData returned error: %v", err)
	}

	if data.RepoOwner != "owner" || data.RepoName != "repo" {
		t.Errorf("expected repo owner/repo owner/repo from git remote, got %s/%s", data.RepoOwner, data.RepoName)
	}
}

func TestRunWorkflowGenerate_CreatesWorkflowFile(t *testing.T) {
	cfg, tempDir := prepareWorkflowCommandTest(t)
	cfg.Executor.DryRun = false

	outputRel := filepath.Join("ci", "cascade.yml")
	req := workflowGenerateRequest{OutputPath: outputRel}

	if err := runWorkflowGenerate(req); err != nil {
		t.Fatalf("runWorkflowGenerate returned error: %v", err)
	}

	outputPath := filepath.Join(tempDir, outputRel)
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read generated workflow: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "Cascade Dependency Release") {
		t.Errorf("generated workflow missing expected name: %s", content)
	}
	if !strings.Contains(content, "v*.*.*") {
		t.Errorf("generated workflow missing tag pattern: %s", content)
	}
}

func TestRunWorkflowGenerate_DryRunSkipsWrite(t *testing.T) {
	cfg, tempDir := prepareWorkflowCommandTest(t)
	cfg.Executor.DryRun = true

	outputRel := filepath.Join("ci", "dry-run.yml")
	req := workflowGenerateRequest{OutputPath: outputRel}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runWorkflowGenerate(req)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runWorkflowGenerate returned error: %v", err)
	}

	preview, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("failed to read preview output: %v", readErr)
	}

	if !strings.Contains(string(preview), "Cascade Dependency Release") {
		t.Errorf("dry-run output missing workflow preview: %s", string(preview))
	}

	if _, statErr := os.Stat(filepath.Join(tempDir, outputRel)); !os.IsNotExist(statErr) {
		t.Errorf("expected no file to be written during dry-run, stat error: %v", statErr)
	}
}

func TestRunWorkflowGenerate_CustomTemplate(t *testing.T) {
	cfg, tempDir := prepareWorkflowCommandTest(t)
	cfg.Executor.DryRun = false

	templateContent := "name: Custom\nmodule: {{ .ModulePath }}\nfile: {{ .WorkflowFile }}\n"
	templatePath := filepath.Join(tempDir, "custom.tmpl")
	if err := os.WriteFile(templatePath, []byte(templateContent), 0o644); err != nil {
		t.Fatalf("failed to write custom template: %v", err)
	}

	outputRel := "custom.yml"
	req := workflowGenerateRequest{
		TemplatePath: templatePath,
		OutputPath:   outputRel,
	}

	if err := runWorkflowGenerate(req); err != nil {
		t.Fatalf("runWorkflowGenerate returned error: %v", err)
	}

	generated, err := os.ReadFile(filepath.Join(tempDir, outputRel))
	if err != nil {
		t.Fatalf("failed to read generated file: %v", err)
	}

	content := string(generated)
	if !strings.Contains(content, "name: Custom") {
		t.Errorf("custom template not applied: %s", content)
	}
	if !strings.Contains(content, "module: github.com/test/workflow") {
		t.Errorf("custom template missing module path: %s", content)
	}
	if !strings.Contains(content, "file: custom.yml") {
		t.Errorf("custom template missing workflow file reference: %s", content)
	}
}

func TestRunWorkflowGenerate_CustomTemplateMissingVariable(t *testing.T) {
	cfg, tempDir := prepareWorkflowCommandTest(t)
	cfg.Executor.DryRun = false

	templatePath := filepath.Join(tempDir, "invalid.tmpl")
	if err := os.WriteFile(templatePath, []byte("{{ .DoesNotExist }}"), 0o644); err != nil {
		t.Fatalf("failed to write invalid template: %v", err)
	}

	req := workflowGenerateRequest{
		TemplatePath: templatePath,
		OutputPath:   "invalid.yml",
	}

	err := runWorkflowGenerate(req)
	if err == nil {
		t.Fatal("expected error from missing template variable, got nil")
	}

	if !strings.Contains(err.Error(), "failed to render workflow template") {
		t.Errorf("expected render error, got: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(tempDir, "invalid.yml")); !os.IsNotExist(statErr) {
		t.Errorf("expected no file to be written on error, stat error: %v", statErr)
	}
}
