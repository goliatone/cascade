package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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

func TestRunPlanLocalDiscoversGoWorkModulesFromNestedDirectory(t *testing.T) {
	repositoryRoot, nestedDir, workspace := writeLocalCommandGoWorkRepository(t)
	t.Chdir(filepath.Join(nestedDir, "pkg"))

	cmd := newPlanLocalCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)

	originalContainer := container
	container = nil
	defer func() { container = originalContainer }()

	if err := runPlanLocal(cmd, localCommandOptions{}); err != nil {
		t.Fatalf("run repository plan local failed: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"Local dependency plan for repository " + repositoryRoot,
		"Modules: 2",
		"Go workspace: " + filepath.Join(repositoryRoot, "go.work"),
		"Dependency workspace: " + workspace,
		"Module: github.com/goliatone/app",
		"Module: github.com/goliatone/app/adapter",
		"Repository summary:",
		"updates: 2",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, output)
		}
	}
}

func TestRunUpdateLocalDryRunDoesNotMutateAnyGoWorkModule(t *testing.T) {
	repositoryRoot, nestedDir, _ := writeLocalCommandGoWorkRepository(t)
	t.Chdir(repositoryRoot)
	paths := []string{filepath.Join(repositoryRoot, "go.mod"), filepath.Join(nestedDir, "go.mod")}
	before := map[string][]byte{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		before[path] = data
	}

	withLocalHookConfig(t, config.LocalUpdateHooksConfig{}, true, func() {
		cmd := newUpdateLocalCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		if err := runUpdateLocal(cmd, localCommandOptions{}); err != nil {
			t.Fatalf("run repository dry-run: %v", err)
		}
		if !strings.Contains(out.String(), "DRY RUN: Local dependency update plan for repository") {
			t.Fatalf("expected aggregate dry-run output, got:\n%s", out.String())
		}
	})

	for _, path := range paths {
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s after dry-run: %v", path, err)
		}
		if !bytes.Equal(before[path], after) {
			t.Fatalf("dry-run mutated %s", path)
		}
	}
}

func TestRunPlanLocalDoesNotRunConfiguredHooks(t *testing.T) {
	moduleDir, _ := writeLocalCommandWorkspace(t)
	t.Chdir(moduleDir)
	marker := filepath.Join(t.TempDir(), "hook")

	withLocalHookConfig(t, config.LocalUpdateHooksConfig{
		After: []config.HookConfig{hookMarker("after", marker, false)},
	}, false, func() {
		cmd := newPlanLocalCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)

		if err := runPlanLocal(cmd, localCommandOptions{}); err != nil {
			t.Fatalf("run plan local failed: %v\n%s", err, out.String())
		}
		assertNoFile(t, marker)
		if strings.Contains(out.String(), "Hooks:") {
			t.Fatalf("expected no hook output, got:\n%s", out.String())
		}
	})
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

func TestRunUpdateLocalReportsPlainProgressOnStderr(t *testing.T) {
	moduleDir, _ := writeLocalCommandWorkspace(t)
	t.Chdir(moduleDir)

	originalContainer := container
	originalConfig := cfg
	container = nil
	cfg = config.New()
	defer func() {
		container = originalContainer
		cfg = originalConfig
	}()

	cmd := newUpdateLocalCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)

	if err := runUpdateLocal(cmd, localCommandOptions{Only: []string{"current"}}); err != nil {
		t.Fatalf("run update local failed: %v", err)
	}
	progress := errOut.String()
	for _, want := range []string{"→ Planning local dependency updates", "✓ Planned 0 updates across 1 candidates"} {
		if !strings.Contains(progress, want) {
			t.Fatalf("progress missing %q: %q", want, progress)
		}
	}
	if strings.Contains(progress, "\x1b[") {
		t.Fatalf("redirected progress contains ANSI escapes: %q", progress)
	}
}

func TestRunUpdateLocalEnforcesConfiguredTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX shell script")
	}
	moduleDir, workspace := writeLocalCommandWorkspace(t)
	t.Chdir(moduleDir)
	prependBlockingGoBinary(t)
	marker := filepath.Join(t.TempDir(), "failure-hook")
	projectConfig := `hooks:
  update:
    local:
      after_failure:
        - name: failure
          cmd: [` + strconvQuote(os.Args[0]) + `, -test.run=TestCommandLocalHookHelperProcess]
          timeout: 10s
          env:
            CASCADE_COMMAND_HOOK_HELPER: "1"
            CASCADE_HOOK_MARKER: ` + strconvQuote(marker) + `
            CASCADE_HOOK_VALUE: failure
`
	if err := os.WriteFile(filepath.Join(moduleDir, ".cascade.yaml"), []byte(projectConfig), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	originalContainer := container
	originalConfig := cfg
	container = nil
	cfg = nil
	defer func() {
		container = originalContainer
		cfg = originalConfig
	}()

	cmd := newRootCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"update", "local", "--workspace", workspace, "--only", "old", "--no-tidy", "--timeout", "40ms"})

	started := time.Now()
	err := cmd.Execute()
	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("expected timed out update to fail")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("configured timeout was not enforced; command took %s", elapsed)
	}
	if !strings.Contains(err.Error(), "local dependency update timed out after 40ms") {
		t.Fatalf("expected clear timeout error, got %v", err)
	}
	if !strings.Contains(out.String(), "local update interrupted: context deadline exceeded") {
		t.Fatalf("expected timeout in result summary, got:\n%s", out.String())
	}
	if !strings.Contains(errOut.String(), "Failed to update github.com/goliatone/old") {
		t.Fatalf("expected failed update progress, got:\n%s", errOut.String())
	}
	assertMarker(t, marker, "failure|failure")
	if !strings.Contains(errOut.String(), "Completed 1 local update hook") {
		t.Fatalf("expected failure hook to run after the apply timeout, got:\n%s", errOut.String())
	}
}

func TestRunUpdateLocalSkipsUnselectedHookProgress(t *testing.T) {
	moduleDir, _ := writeLocalCommandWorkspace(t)
	t.Chdir(moduleDir)
	marker := filepath.Join(t.TempDir(), "failure-hook")

	withLocalHookConfig(t, config.LocalUpdateHooksConfig{
		AfterFailure: []config.HookConfig{hookMarker("failure", marker, false)},
	}, false, func() {
		cmd := newUpdateLocalCommand()
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)

		if err := runUpdateLocal(cmd, localCommandOptions{Only: []string{"current"}}); err != nil {
			t.Fatalf("run update local failed: %v", err)
		}
		assertNoFile(t, marker)
		if strings.Contains(errOut.String(), "local update hook") || strings.Contains(out.String(), "Hooks:") {
			t.Fatalf("expected unselected hooks to stay silent, stdout=%q stderr=%q", out.String(), errOut.String())
		}
	})
}

func TestRunUpdateLocalSkipsUnselectedSuccessHookProgressAfterFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX shell script")
	}
	moduleDir, _ := writeLocalCommandWorkspace(t)
	t.Chdir(moduleDir)
	prependGoBinary(t, "#!/bin/sh\nexit 1\n")
	marker := filepath.Join(t.TempDir(), "success-hook")

	withLocalHookConfig(t, config.LocalUpdateHooksConfig{
		AfterSuccess: []config.HookConfig{hookMarker("success", marker, false)},
	}, false, func() {
		cmd := newUpdateLocalCommand()
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)

		err := runUpdateLocal(cmd, localCommandOptions{Only: []string{"old"}, NoTidy: true})
		if err == nil {
			t.Fatal("expected update failure")
		}
		assertNoFile(t, marker)
		if strings.Contains(errOut.String(), "local update hook") || strings.Contains(out.String(), "Hooks:") {
			t.Fatalf("expected unselected hooks to stay silent, stdout=%q stderr=%q", out.String(), errOut.String())
		}
	})
}

func TestRunUpdateLocalRunsConfiguredSuccessHooks(t *testing.T) {
	moduleDir, _ := writeLocalCommandWorkspace(t)
	t.Chdir(moduleDir)

	markerDir := t.TempDir()
	afterMarker := filepath.Join(markerDir, "after")
	successMarker := filepath.Join(markerDir, "success")
	failureMarker := filepath.Join(markerDir, "failure")
	alwaysMarker := filepath.Join(markerDir, "always")

	withLocalHookConfig(t, config.LocalUpdateHooksConfig{
		After:        []config.HookConfig{hookMarker("after", afterMarker, false)},
		AfterSuccess: []config.HookConfig{hookMarker("success", successMarker, false)},
		AfterFailure: []config.HookConfig{hookMarker("failure", failureMarker, false)},
		Always:       []config.HookConfig{hookMarker("always", alwaysMarker, false)},
	}, false, func() {
		cmd := newUpdateLocalCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)

		if err := runUpdateLocal(cmd, localCommandOptions{Only: []string{"current"}}); err != nil {
			t.Fatalf("run update local with hooks failed: %v\n%s", err, out.String())
		}

		assertMarker(t, afterMarker, "after|success")
		assertMarker(t, successMarker, "success|success")
		assertMarker(t, alwaysMarker, "always|success")
		assertNoFile(t, failureMarker)
		for _, want := range []string{
			"Hooks:",
			"after_success after [ok]",
			"after_success success [ok]",
			"always always [ok]",
		} {
			if !strings.Contains(out.String(), want) {
				t.Fatalf("expected hook output to contain %q, got:\n%s", want, out.String())
			}
		}
	})
}

func TestRunUpdateLocalDryRunSkipsConfiguredHooks(t *testing.T) {
	moduleDir, _ := writeLocalCommandWorkspace(t)
	t.Chdir(moduleDir)

	marker := filepath.Join(t.TempDir(), "hook")
	withLocalHookConfig(t, config.LocalUpdateHooksConfig{
		After: []config.HookConfig{hookMarker("after", marker, false)},
	}, true, func() {
		cmd := newUpdateLocalCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)

		if err := runUpdateLocal(cmd, localCommandOptions{}); err != nil {
			t.Fatalf("run update local dry-run with hooks failed: %v\n%s", err, out.String())
		}
		assertNoFile(t, marker)
		if !strings.Contains(out.String(), "skipping hook execution during dry-run") {
			t.Fatalf("expected dry-run hook skip message, got:\n%s", out.String())
		}
	})
}

func TestRunUpdateLocalNoHooksSkipsConfiguredHooks(t *testing.T) {
	moduleDir, _ := writeLocalCommandWorkspace(t)
	t.Chdir(moduleDir)

	marker := filepath.Join(t.TempDir(), "hook")
	withLocalHookConfig(t, config.LocalUpdateHooksConfig{
		After: []config.HookConfig{hookMarker("after", marker, false)},
	}, false, func() {
		cmd := newUpdateLocalCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)

		if err := runUpdateLocal(cmd, localCommandOptions{Only: []string{"current"}, NoHooks: true}); err != nil {
			t.Fatalf("run update local --no-hooks failed: %v\n%s", err, out.String())
		}
		assertNoFile(t, marker)
		if strings.Contains(out.String(), "Hooks:") {
			t.Fatalf("expected no hook output, got:\n%s", out.String())
		}
	})
}

func TestRunUpdateLocalHookFailureReturnsExecutionError(t *testing.T) {
	moduleDir, _ := writeLocalCommandWorkspace(t)
	t.Chdir(moduleDir)

	markerDir := t.TempDir()
	failMarker := filepath.Join(markerDir, "fail")
	alwaysMarker := filepath.Join(markerDir, "always")
	withLocalHookConfig(t, config.LocalUpdateHooksConfig{
		AfterSuccess: []config.HookConfig{hookMarker("fail", failMarker, true)},
		Always:       []config.HookConfig{hookMarker("always", alwaysMarker, false)},
	}, false, func() {
		cmd := newUpdateLocalCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)

		err := runUpdateLocal(cmd, localCommandOptions{Only: []string{"current"}})
		if err == nil {
			t.Fatal("expected hook failure")
		}
		cliErr, ok := err.(*CLIError)
		if !ok || cliErr.Code != ExitExecutionError {
			t.Fatalf("expected execution CLI error, got %#v", err)
		}
		assertMarker(t, failMarker, "fail|success")
		assertMarker(t, alwaysMarker, "always|success")
		output := out.String()
		for _, want := range []string{
			"after_success fail [failed]",
			"always always [ok]",
			"planned hook failure",
		} {
			if !strings.Contains(output, want) {
				t.Fatalf("expected output to contain %q, got:\n%s", want, output)
			}
		}
	})
}

func TestRunUpdateLocalUsesCommandContextForHooks(t *testing.T) {
	moduleDir, _ := writeLocalCommandWorkspace(t)
	t.Chdir(moduleDir)

	marker := filepath.Join(t.TempDir(), "hook")
	withLocalHookConfig(t, config.LocalUpdateHooksConfig{
		AfterSuccess: []config.HookConfig{hookSleepMarker("slow", marker)},
	}, false, func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		cmd := newUpdateLocalCommand()
		cmd.SetContext(ctx)
		var out bytes.Buffer
		cmd.SetOut(&out)

		err := runUpdateLocal(cmd, localCommandOptions{Only: []string{"current"}})
		if err == nil {
			t.Fatal("expected canceled hook execution to fail")
		}
		cliErr, ok := err.(*CLIError)
		if !ok || cliErr.Code != ExitExecutionError {
			t.Fatalf("expected execution CLI error, got %#v", err)
		}
		assertNoFile(t, marker)
		if !strings.Contains(out.String(), "after_success slow [failed]") {
			t.Fatalf("expected failed hook output, got:\n%s", out.String())
		}
	})
}

func TestRunUpdateLocalApplyFailureRunsFailureAndAlwaysHooks(t *testing.T) {
	moduleDir, _ := writeLocalCommandWorkspace(t)
	t.Chdir(moduleDir)
	t.Setenv("GOPROXY", "off")

	markerDir := t.TempDir()
	successMarker := filepath.Join(markerDir, "success")
	failureMarker := filepath.Join(markerDir, "failure")
	alwaysMarker := filepath.Join(markerDir, "always")
	withLocalHookConfig(t, config.LocalUpdateHooksConfig{
		AfterSuccess: []config.HookConfig{hookMarker("success", successMarker, false)},
		AfterFailure: []config.HookConfig{hookMarker("failure", failureMarker, false)},
		Always:       []config.HookConfig{hookMarker("always", alwaysMarker, false)},
	}, false, func() {
		cmd := newUpdateLocalCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)

		err := runUpdateLocal(cmd, localCommandOptions{Only: []string{"old"}, NoTidy: true})
		if err == nil {
			t.Fatal("expected apply failure")
		}
		cliErr, ok := err.(*CLIError)
		if !ok || cliErr.Code != ExitExecutionError {
			t.Fatalf("expected execution CLI error, got %#v", err)
		}
		assertNoFile(t, successMarker)
		assertMarker(t, failureMarker, "failure|failure")
		assertMarker(t, alwaysMarker, "always|failure")
		output := out.String()
		for _, want := range []string{
			"[apply-failed]",
			"after_failure failure [ok]",
			"always always [ok]",
		} {
			if !strings.Contains(output, want) {
				t.Fatalf("expected output to contain %q, got:\n%s", want, output)
			}
		}
	})
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

func TestCommandLocalHookHelperProcess(t *testing.T) {
	if os.Getenv("CASCADE_COMMAND_HOOK_HELPER") != "1" {
		return
	}
	marker := os.Getenv("CASCADE_HOOK_MARKER")
	value := os.Getenv("CASCADE_HOOK_VALUE") + "|" + os.Getenv("CASCADE_UPDATE_STATUS")
	if os.Getenv("CASCADE_HOOK_SLEEP") == "1" {
		time.Sleep(200 * time.Millisecond)
	}
	if marker != "" {
		if err := os.WriteFile(marker, []byte(value), 0o644); err != nil {
			t.Fatalf("write marker: %v", err)
		}
	}
	if os.Getenv("CASCADE_HOOK_FAIL") == "1" {
		fmt.Fprintln(os.Stdout, "planned hook failure")
		os.Exit(9)
	}
	os.Exit(0)
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

func TestRootUpdateLocalDryRunReportsMatchedGlobalHookRule(t *testing.T) {
	withClearedGitHubEnv(t, func() {
		moduleDir, workspace := writeLocalCommandWorkspace(t)
		t.Chdir(moduleDir)
		t.Setenv("HOME", t.TempDir())
		xdg := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", xdg)
		marker := filepath.Join(t.TempDir(), "hook")

		writeGlobalCascadeConfig(t, xdg, `workspace:
  path: `+strconvQuote(workspace)+`
hooks:
  update:
    local:
      rules:
        - name: global/test
          match:
            modules: [github.com/goliatone/app]
          after_success:
            - name: global-test
              run: "touch `+marker+`"
`)

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
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs([]string{"update", "local", "--dry-run"})

		if err := root.Execute(); err != nil {
			t.Fatalf("root update local dry-run failed: %v\n%s", err, out.String())
		}
		assertNoFile(t, marker)

		output := out.String()
		for _, want := range []string{
			"Local update hooks configured; skipping hook execution during dry-run.",
			"Matched hook rules:",
			"global/test [global ",
		} {
			if !strings.Contains(output, want) {
				t.Fatalf("expected output to contain %q, got:\n%s", want, output)
			}
		}
	})
}

func TestRootUpdateLocalDryRunSkipsNonMatchingGlobalHookRule(t *testing.T) {
	withClearedGitHubEnv(t, func() {
		moduleDir, workspace := writeLocalCommandWorkspace(t)
		t.Chdir(moduleDir)
		t.Setenv("HOME", t.TempDir())
		xdg := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", xdg)

		writeGlobalCascadeConfig(t, xdg, `workspace:
  path: `+strconvQuote(workspace)+`
hooks:
  update:
    local:
      rules:
        - name: global/test
          match:
            modules: [github.com/goliatone/other]
          after_success:
            - run: echo nope
`)

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
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs([]string{"update", "local", "--dry-run"})

		if err := root.Execute(); err != nil {
			t.Fatalf("root update local dry-run failed: %v\n%s", err, out.String())
		}
		output := out.String()
		if strings.Contains(output, "Matched hook rules:") || strings.Contains(output, "global/test") {
			t.Fatalf("expected non-matching rule to stay out of dry-run output, got:\n%s", output)
		}
	})
}

func TestRootUpdateLocalNoHooksSuppressesMatchedGlobalHookRule(t *testing.T) {
	withClearedGitHubEnv(t, func() {
		moduleDir, workspace := writeLocalCommandWorkspace(t)
		t.Chdir(moduleDir)
		t.Setenv("HOME", t.TempDir())
		xdg := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", xdg)

		writeGlobalCascadeConfig(t, xdg, `workspace:
  path: `+strconvQuote(workspace)+`
hooks:
  update:
    local:
      rules:
        - name: global/test
          match:
            modules: [github.com/goliatone/app]
          after_success:
            - run: echo nope
`)

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
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs([]string{"update", "local", "--dry-run", "--no-hooks"})

		if err := root.Execute(); err != nil {
			t.Fatalf("root update local dry-run failed: %v\n%s", err, out.String())
		}
		output := out.String()
		if strings.Contains(output, "Matched hook rules:") || strings.Contains(output, "skipping hook execution during dry-run") {
			t.Fatalf("expected --no-hooks to suppress hook output, got:\n%s", output)
		}
	})
}

func TestRootUpdateLocalRunsMatchedGlobalHookRule(t *testing.T) {
	withClearedGitHubEnv(t, func() {
		moduleDir, workspace := writeLocalCommandWorkspace(t)
		t.Chdir(moduleDir)
		t.Setenv("HOME", t.TempDir())
		xdg := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", xdg)
		marker := filepath.Join(t.TempDir(), "hook")

		writeGlobalCascadeConfig(t, xdg, `workspace:
  path: `+strconvQuote(workspace)+`
hooks:
  update:
    local:
      rules:
        - name: global/test
          match:
            modules: [github.com/goliatone/app]
          after_success:
            - name: global-test
              cmd: [`+strconvQuote(os.Args[0])+`, -test.run=TestCommandLocalHookHelperProcess]
              env:
                CASCADE_COMMAND_HOOK_HELPER: "1"
                CASCADE_HOOK_MARKER: `+strconvQuote(marker)+`
                CASCADE_HOOK_VALUE: global-test
`)

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
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs([]string{"update", "local", "--only", "current"})

		if err := root.Execute(); err != nil {
			t.Fatalf("root update local failed: %v\n%s", err, out.String())
		}
		assertMarker(t, marker, "global-test|success")
		if !strings.Contains(out.String(), "after_success global-test [ok]") {
			t.Fatalf("expected global hook result output, got:\n%s", out.String())
		}
	})
}

func TestRootUpdateLocalProjectConfigDisablesGlobalHookRule(t *testing.T) {
	withClearedGitHubEnv(t, func() {
		moduleDir, workspace := writeLocalCommandWorkspace(t)
		t.Chdir(moduleDir)
		t.Setenv("HOME", t.TempDir())
		xdg := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", xdg)
		marker := filepath.Join(t.TempDir(), "hook")

		writeGlobalCascadeConfig(t, xdg, `workspace:
  path: `+strconvQuote(workspace)+`
hooks:
  update:
    local:
      rules:
        - name: global/test
          match:
            modules: [github.com/goliatone/app]
          after_success:
            - name: global-test
              cmd: [`+strconvQuote(os.Args[0])+`, -test.run=TestCommandLocalHookHelperProcess]
              env:
                CASCADE_COMMAND_HOOK_HELPER: "1"
                CASCADE_HOOK_MARKER: `+strconvQuote(marker)+`
                CASCADE_HOOK_VALUE: global-test
`)
		projectConfig := `hooks:
  update:
    local:
      disabled_rules:
        - global/test
`
		if err := os.WriteFile(filepath.Join(moduleDir, ".cascade.yaml"), []byte(projectConfig), 0o644); err != nil {
			t.Fatalf("write project config: %v", err)
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
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs([]string{"update", "local", "--only", "current"})

		if err := root.Execute(); err != nil {
			t.Fatalf("root update local failed: %v\n%s", err, out.String())
		}
		assertNoFile(t, marker)
		if strings.Contains(out.String(), "global-test") || strings.Contains(out.String(), "Hooks:") {
			t.Fatalf("expected disabled global hook to stay silent, got:\n%s", out.String())
		}
	})
}

func TestRootPlanLocalUsesWorkspaceFromDiscoveredConfig(t *testing.T) {
	withClearedGitHubEnv(t, func() {
		moduleDir, workspace := writeDetachedLocalCommandWorkspace(t)
		subdir := filepath.Join(moduleDir, "internal", "pkg")
		if err := os.MkdirAll(subdir, 0o755); err != nil {
			t.Fatalf("create subdir: %v", err)
		}
		configFile := filepath.Join(moduleDir, ".cascade.yaml")
		configContent := "workspace:\n  path: " + strconvQuote(workspace) + "\n"
		if err := os.WriteFile(configFile, []byte(configContent), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}
		t.Chdir(subdir)
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
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs([]string{"plan", "local"})

		if err := root.Execute(); err != nil {
			t.Fatalf("root plan local with config workspace failed: %v\n%s", err, out.String())
		}

		output := out.String()
		if !strings.Contains(output, "Workspace: "+workspace) {
			t.Fatalf("expected configured workspace in output, got:\n%s", output)
		}
		if !strings.Contains(output, "github.com/goliatone/old v1.0.0 -> v1.1.0 [update]") {
			t.Fatalf("expected configured workspace candidate, got:\n%s", output)
		}
	})
}

func TestRootUpdateLocalUsesCascadeWorkspaceEnvEvenWhenDefaultCachePath(t *testing.T) {
	withClearedGitHubEnv(t, func() {
		xdgCache := t.TempDir()
		workspace := filepath.Join(xdgCache, "cascade")
		moduleDir := writeDetachedLocalCommandWorkspaceAt(t, workspace)
		t.Chdir(moduleDir)
		t.Setenv("XDG_CACHE_HOME", xdgCache)
		t.Setenv("CASCADE_WORKSPACE", workspace)
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
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs([]string{"update", "local", "--dry-run"})

		if err := root.Execute(); err != nil {
			t.Fatalf("root update local with env workspace failed: %v\n%s", err, out.String())
		}

		output := out.String()
		if !strings.Contains(output, "Workspace: "+workspace) {
			t.Fatalf("expected env workspace in output, got:\n%s", output)
		}
		if !strings.Contains(output, "DRY RUN: Local dependency update plan") {
			t.Fatalf("expected dry-run update plan, got:\n%s", output)
		}
	})
}

func TestRootUpdateLocalRejectsInvalidHookConfig(t *testing.T) {
	withClearedGitHubEnv(t, func() {
		moduleDir, workspace := writeLocalCommandWorkspace(t)
		t.Chdir(moduleDir)
		t.Setenv("HOME", t.TempDir())
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())

		configContent := `workspace:
  path: ` + strconvQuote(workspace) + `
hooks:
  update:
    local:
      after:
        - name: invalid
          run: echo invalid
          cmd: [echo, invalid]
`
		if err := os.WriteFile(filepath.Join(moduleDir, ".cascade.yaml"), []byte(configContent), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
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
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs([]string{"update", "local", "--dry-run"})

		err := root.Execute()
		if err == nil {
			t.Fatal("expected invalid hook config to fail")
		}
		cliErr, ok := err.(*CLIError)
		if !ok || cliErr.Code != ExitConfigError {
			t.Fatalf("expected config CLI error, got %#v", err)
		}
		if !strings.Contains(err.Error(), "hook must define exactly one of run or cmd") {
			t.Fatalf("expected hook validation error, got: %v", err)
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

func writeLocalCommandGoWorkRepository(t *testing.T) (repositoryRoot, nestedDir, workspace string) {
	t.Helper()
	workspace = t.TempDir()
	repositoryRoot = filepath.Join(workspace, "app")
	nestedDir = filepath.Join(repositoryRoot, "adapter")
	if err := os.MkdirAll(filepath.Join(repositoryRoot, ".git"), 0o755); err != nil {
		t.Fatalf("create repository metadata: %v", err)
	}
	rootMod := `module github.com/goliatone/app

go 1.24

require github.com/goliatone/old v1.0.0
`
	nestedMod := `module github.com/goliatone/app/adapter

go 1.24

require github.com/goliatone/older v1.0.0
`
	if err := os.MkdirAll(filepath.Join(nestedDir, "pkg"), 0o755); err != nil {
		t.Fatalf("create nested module: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repositoryRoot, "go.mod"), []byte(rootMod), 0o644); err != nil {
		t.Fatalf("write root go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "go.mod"), []byte(nestedMod), 0o644); err != nil {
		t.Fatalf("write nested go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repositoryRoot, "go.work"), []byte("go 1.24\nuse (\n.\n./adapter\n)\n"), 0o644); err != nil {
		t.Fatalf("write go.work: %v", err)
	}
	writeLocalCommandSibling(t, workspace, "old", "v1.1.0")
	writeLocalCommandSibling(t, workspace, "older", "v1.2.0")
	return repositoryRoot, nestedDir, workspace
}

func prependBlockingGoBinary(t *testing.T) {
	t.Helper()
	prependGoBinary(t, "#!/bin/sh\nexec sleep 30\n")
}

func prependGoBinary(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "go")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write go helper: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func writeDetachedLocalCommandWorkspace(t *testing.T) (moduleDir, workspace string) {
	t.Helper()
	workspace = t.TempDir()
	moduleDir = writeDetachedLocalCommandWorkspaceAt(t, workspace)
	return moduleDir, workspace
}

func writeDetachedLocalCommandWorkspaceAt(t *testing.T, workspace string) string {
	t.Helper()
	moduleDir := filepath.Join(t.TempDir(), "app")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatalf("create module dir: %v", err)
	}
	goMod := `module github.com/goliatone/app

go 1.24

require github.com/goliatone/old v1.0.0
`
	if err := os.WriteFile(filepath.Join(moduleDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write module go.mod: %v", err)
	}
	writeLocalCommandSibling(t, workspace, "old", "v1.1.0")
	return moduleDir
}

func strconvQuote(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}

func writeGlobalCascadeConfig(t *testing.T, xdgConfigHome, content string) string {
	t.Helper()
	dir := filepath.Join(xdgConfigHome, "cascade")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create global config dir: %v", err)
	}
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	return path
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

func withLocalHookConfig(t *testing.T, localHooks config.LocalUpdateHooksConfig, dryRun bool, fn func()) {
	t.Helper()
	mockContainer, err := di.New(di.WithConfig(&config.Config{
		Executor: config.ExecutorConfig{DryRun: dryRun},
		Hooks: config.HooksConfig{
			Update: config.UpdateHooksConfig{
				Local: localHooks,
			},
		},
	}))
	if err != nil {
		t.Fatalf("create container: %v", err)
	}
	originalContainer := container
	container = mockContainer
	defer func() { container = originalContainer }()
	fn()
}

func hookMarker(name, marker string, fail bool) config.HookConfig {
	env := map[string]string{
		"CASCADE_COMMAND_HOOK_HELPER": "1",
		"CASCADE_HOOK_MARKER":         marker,
		"CASCADE_HOOK_VALUE":          name,
	}
	if fail {
		env["CASCADE_HOOK_FAIL"] = "1"
	}
	return config.HookConfig{
		Name: name,
		Cmd:  []string{os.Args[0], "-test.run=TestCommandLocalHookHelperProcess"},
		Env:  env,
	}
}

func hookSleepMarker(name, marker string) config.HookConfig {
	env := map[string]string{
		"CASCADE_COMMAND_HOOK_HELPER": "1",
		"CASCADE_HOOK_MARKER":         marker,
		"CASCADE_HOOK_VALUE":          name,
		"CASCADE_HOOK_SLEEP":          "1",
	}
	return config.HookConfig{
		Name: name,
		Cmd:  []string{os.Args[0], "-test.run=TestCommandLocalHookHelperProcess"},
		Env:  env,
	}
}

func assertMarker(t *testing.T, marker, want string) {
	t.Helper()
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read marker %s: %v", marker, err)
	}
	if string(got) != want {
		t.Fatalf("expected marker %s to contain %q, got %q", marker, want, string(got))
	}
}

func assertNoFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s not to exist, stat err=%v", path, err)
	}
}
