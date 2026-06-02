package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goliatone/cascade/pkg/config"
	"github.com/goliatone/cascade/pkg/di"
)

func TestLocalCommandsRegistered(t *testing.T) {
	root := newRootCommand()
	if _, _, err := root.Find([]string{"plan", "local"}); err != nil {
		t.Fatalf("plan local not registered: %v", err)
	}
	if _, _, err := root.Find([]string{"update", "local"}); err != nil {
		t.Fatalf("update local not registered: %v", err)
	}
	for _, subcmd := range root.Commands() {
		if subcmd.Name() == "update" && isProductionCommand(subcmd) {
			t.Fatalf("update command must not require production credentials")
		}
	}
}

func TestRunPlanLocalRendersCandidates(t *testing.T) {
	moduleDir, workspace := writeLocalCommandWorkspace(t)
	t.Chdir(moduleDir)

	cmd := newPlanLocalCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)

	originalContainer := container
	container = nil
	defer func() { container = originalContainer }()

	if err := runPlanLocal(cmd, localCommandOptions{}); err != nil {
		t.Fatalf("run plan local failed: %v", err)
	}

	output := out.String()
	for _, want := range []string{
		"Local dependency plan for github.com/goliatone/app",
		"Workspace: " + workspace,
		"github.com/goliatone/old v1.0.0 -> v1.1.0 [update]",
		"github.com/goliatone/current v1.0.0 -> v1.0.0 [current]",
		"github.com/goliatone/replaced v1.0.0 -> - [skipped-replace]",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, output)
		}
	}
}

func TestRunUpdateLocalDryRunRendersPlanWithoutMutation(t *testing.T) {
	moduleDir, _ := writeLocalCommandWorkspace(t)
	t.Chdir(moduleDir)
	before, err := os.ReadFile(filepath.Join(moduleDir, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}

	mockContainer, err := di.New(di.WithConfig(&config.Config{
		Executor: config.ExecutorConfig{DryRun: true},
	}))
	if err != nil {
		t.Fatalf("create container: %v", err)
	}
	originalContainer := container
	container = mockContainer
	defer func() { container = originalContainer }()

	cmd := newUpdateLocalCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runUpdateLocal(cmd, localCommandOptions{}); err != nil {
		t.Fatalf("run update local dry-run failed: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(moduleDir, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod after: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("dry-run mutated go.mod")
	}
	if !strings.Contains(out.String(), "DRY RUN: Local dependency update plan") {
		t.Fatalf("expected dry-run output, got:\n%s", out.String())
	}
}

func TestRunUpdateLocalWithoutContainerDoesNotPanic(t *testing.T) {
	moduleDir, _ := writeLocalCommandWorkspace(t)
	t.Chdir(moduleDir)

	originalContainer := container
	container = nil
	defer func() { container = originalContainer }()

	cmd := newUpdateLocalCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := runUpdateLocal(cmd, localCommandOptions{Only: []string{"current"}})
	if err != nil {
		t.Fatalf("expected nil-container update with no updates to succeed, got: %v", err)
	}
}

func TestIsDefaultLocalCacheWorkspaceRecognizesXDG(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", xdg)

	if !isDefaultLocalCacheWorkspace(filepath.Join(xdg, "cascade")) {
		t.Fatalf("expected XDG cache workspace to be recognized as default")
	}
	if isDefaultLocalCacheWorkspace(filepath.Join(t.TempDir(), "workspace")) {
		t.Fatalf("expected arbitrary workspace not to be recognized as default")
	}
}

func TestRootPlanLocalDoesNotInitializeGitHubProvider(t *testing.T) {
	withClearedGitHubEnv(t, func() {
		moduleDir, workspace := writeLocalCommandWorkspace(t)
		t.Chdir(moduleDir)
		t.Setenv("HOME", t.TempDir())
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())

		originalContainer := container
		originalCfg := cfg
		container = nil
		cfg = nil
		defer func() {
			container = originalContainer
			cfg = originalCfg
		}()

		root := newRootCommand()
		var out bytes.Buffer
		var errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		root.SetArgs([]string{"plan", "local", "--workspace", workspace})

		if err := root.Execute(); err != nil {
			t.Fatalf("root plan local failed: %v", err)
		}

		combined := out.String() + errOut.String()
		if strings.Contains(combined, "GitHub provider") || strings.Contains(combined, "github token") {
			t.Fatalf("local-only command initialized GitHub provider:\n%s", combined)
		}
		if !strings.Contains(combined, "Local dependency plan") {
			t.Fatalf("expected local plan output, got:\n%s", combined)
		}
		if container != nil {
			t.Fatalf("expected local-only command to skip DI container initialization")
		}
	})
}

func TestRootUpdateLocalDryRunDoesNotInitializeGitHubProvider(t *testing.T) {
	withClearedGitHubEnv(t, func() {
		moduleDir, workspace := writeLocalCommandWorkspace(t)
		t.Chdir(moduleDir)
		t.Setenv("HOME", t.TempDir())
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		before, err := os.ReadFile(filepath.Join(moduleDir, "go.mod"))
		if err != nil {
			t.Fatalf("read go.mod before dry-run: %v", err)
		}

		originalContainer := container
		originalCfg := cfg
		container = nil
		cfg = nil
		defer func() {
			container = originalContainer
			cfg = originalCfg
		}()

		root := newRootCommand()
		var out bytes.Buffer
		var errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		root.SetArgs([]string{"update", "local", "--workspace", workspace, "--dry-run"})

		if err := root.Execute(); err != nil {
			t.Fatalf("root update local dry-run failed: %v", err)
		}
		after, err := os.ReadFile(filepath.Join(moduleDir, "go.mod"))
		if err != nil {
			t.Fatalf("read go.mod after dry-run: %v", err)
		}
		if !bytes.Equal(before, after) {
			t.Fatalf("root dry-run mutated go.mod")
		}

		combined := out.String() + errOut.String()
		if strings.Contains(combined, "GitHub provider") || strings.Contains(combined, "github token") {
			t.Fatalf("local-only command initialized GitHub provider:\n%s", combined)
		}
		if !strings.Contains(combined, "DRY RUN: Local dependency update plan") {
			t.Fatalf("expected dry-run local update output, got:\n%s", combined)
		}
		if container != nil {
			t.Fatalf("expected local-only command to skip DI container initialization")
		}
	})
}

func TestUpdateCommandRequiresSubcommand(t *testing.T) {
	cmd := newUpdateCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected update without subcommand to fail")
	}
	if !strings.Contains(err.Error(), "update requires a subcommand") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "local") {
		t.Fatalf("expected help output to mention local subcommand, got:\n%s", out.String())
	}
}

func writeLocalCommandWorkspace(t *testing.T) (moduleDir, workspace string) {
	t.Helper()
	workspace = t.TempDir()
	moduleDir = filepath.Join(workspace, "app")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatalf("create module dir: %v", err)
	}
	goMod := `module github.com/goliatone/app

go 1.24

require (
	github.com/goliatone/current v1.0.0
	github.com/goliatone/old v1.0.0
	github.com/goliatone/replaced v1.0.0
)

replace github.com/goliatone/replaced => ../replaced
`
	if err := os.WriteFile(filepath.Join(moduleDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write module go.mod: %v", err)
	}
	writeLocalCommandSibling(t, workspace, "current", "v1.0.0")
	writeLocalCommandSibling(t, workspace, "old", "v1.1.0")
	writeLocalCommandSibling(t, workspace, "replaced", "v1.1.0")
	return moduleDir, workspace
}

func writeLocalCommandSibling(t *testing.T, workspace, name, version string) {
	t.Helper()
	dir := filepath.Join(workspace, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create sibling dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/goliatone/"+name+"\n"), 0o644); err != nil {
		t.Fatalf("write sibling go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".version"), []byte(version), 0o644); err != nil {
		t.Fatalf("write sibling version: %v", err)
	}
}
