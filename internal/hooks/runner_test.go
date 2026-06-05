package hooks_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/goliatone/cascade/internal/hooks"
	"github.com/goliatone/cascade/pkg/config"
)

func TestHookHelperProcess(t *testing.T) {
	if os.Getenv("CASCADE_HOOK_HELPER") != "1" {
		return
	}

	switch os.Getenv("CASCADE_HOOK_ACTION") {
	case "args":
		args := os.Args[1:]
		for i, arg := range args {
			if arg == "--" {
				args = args[i+1:]
				break
			}
		}
		fmt.Println(strings.Join(args, "|"))
	case "cwd-env":
		wd, err := os.Getwd()
		if err != nil {
			fmt.Println(err)
			os.Exit(2)
		}
		fmt.Printf("%s\n%s\n%s\n%s\n", wd, os.Getenv("CUSTOM_VALUE"), os.Getenv("CASCADE_MODULE_DIR"), os.Getenv("CASCADE_UPDATE_STATUS"))
	case "fail":
		fmt.Println("planned failure")
		os.Exit(7)
	case "sleep":
		time.Sleep(200 * time.Millisecond)
		fmt.Println("too late")
	default:
		fmt.Println("ok")
	}
	os.Exit(0)
}

func TestRunner_RunHookShellCommand(t *testing.T) {
	runner := hooks.NewRunner(time.Minute)
	phases := []hooks.PhaseHooks{{
		Phase: hooks.PhaseAfterSuccess,
		Hooks: []config.HookConfig{{Name: "echo", Run: "echo ok"}},
	}}

	results, err := runner.Run(context.Background(), phases, hookContext(t.TempDir()))
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if !strings.Contains(results[0].Output, "ok") {
		t.Fatalf("expected output to contain ok, got %q", results[0].Output)
	}
}

func TestRunner_CmdDoesNotUseShellInterpretation(t *testing.T) {
	runner := hooks.NewRunner(time.Minute)
	phases := []hooks.PhaseHooks{{
		Phase: hooks.PhaseAfterSuccess,
		Hooks: []config.HookConfig{{
			Name: "args",
			Cmd:  []string{os.Args[0], "-test.run=TestHookHelperProcess", "--", "$CASCADE_HOOK_VALUE"},
			Env: map[string]string{
				"CASCADE_HOOK_HELPER": "1",
				"CASCADE_HOOK_ACTION": "args",
				"CASCADE_HOOK_VALUE":  "expanded",
			},
		}},
	}}

	results, err := runner.Run(context.Background(), phases, hookContext(t.TempDir()))
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if got := strings.TrimSpace(results[0].Output); got != "$CASCADE_HOOK_VALUE" {
		t.Fatalf("expected literal arg, got %q", got)
	}
}

func TestRunner_CmdExecutableUsesHookPATH(t *testing.T) {
	moduleDir := t.TempDir()
	binDir := filepath.Join(moduleDir, "bin")
	if err := os.Mkdir(binDir, 0o755); err != nil {
		t.Fatalf("create bin dir: %v", err)
	}
	program := "hook-path-helper"
	helperPath := filepath.Join(binDir, program)
	if runtime.GOOS == "windows" {
		helperPath += ".bat"
		if err := os.WriteFile(helperPath, []byte("@echo from-path\r\n"), 0o644); err != nil {
			t.Fatalf("write helper: %v", err)
		}
	} else {
		if err := os.WriteFile(helperPath, []byte("#!/bin/sh\necho from-path\n"), 0o755); err != nil {
			t.Fatalf("write helper: %v", err)
		}
	}

	runner := hooks.NewRunner(time.Minute)
	phases := []hooks.PhaseHooks{{
		Phase: hooks.PhaseAfterSuccess,
		Hooks: []config.HookConfig{{
			Name: "path-helper",
			Cmd:  []string{program},
			Env:  map[string]string{"PATH": "./bin"},
		}},
	}}

	results, err := runner.Run(context.Background(), phases, hookContext(moduleDir))
	if err != nil {
		t.Fatalf("Run failed: %v; results=%#v", err, results)
	}
	if got := strings.TrimSpace(results[0].Output); got != "from-path" {
		t.Fatalf("expected helper output, got %q", got)
	}
}

func TestRunner_UsesParentContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runner := hooks.NewRunner(time.Minute)
	phases := []hooks.PhaseHooks{{
		Phase: hooks.PhaseAfterSuccess,
		Hooks: []config.HookConfig{{
			Name: "sleep",
			Cmd:  []string{os.Args[0], "-test.run=TestHookHelperProcess"},
			Env: map[string]string{
				"CASCADE_HOOK_HELPER": "1",
				"CASCADE_HOOK_ACTION": "sleep",
			},
		}},
	}}

	results, err := runner.Run(ctx, phases, hookContext(t.TempDir()))
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if len(results) != 1 || !results[0].Failed() {
		t.Fatalf("expected failed result, got %#v", results)
	}
}

func TestRunner_RelativeDirEnvAndContext(t *testing.T) {
	moduleDir := t.TempDir()
	subDir := filepath.Join(moduleDir, "sub")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	runner := hooks.NewRunner(time.Minute)
	phases := []hooks.PhaseHooks{{
		Phase: hooks.PhaseAfterSuccess,
		Hooks: []config.HookConfig{{
			Name: "cwd-env",
			Cmd:  []string{os.Args[0], "-test.run=TestHookHelperProcess"},
			Dir:  "sub",
			Env: map[string]string{
				"CASCADE_HOOK_HELPER": "1",
				"CASCADE_HOOK_ACTION": "cwd-env",
				"CUSTOM_VALUE":        "custom",
			},
		}},
	}}

	results, err := runner.Run(context.Background(), phases, hookContext(moduleDir))
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(results[0].Output), "\n")
	if len(lines) != 4 {
		t.Fatalf("unexpected output: %q", results[0].Output)
	}
	if !samePath(lines[0], subDir) {
		t.Errorf("expected cwd %q, got %q", subDir, lines[0])
	}
	if lines[1] != "custom" {
		t.Errorf("expected custom env, got %q", lines[1])
	}
	if !samePath(lines[2], moduleDir) {
		t.Errorf("expected module dir env %q, got %q", moduleDir, lines[2])
	}
	if lines[3] != "success" {
		t.Errorf("expected update status success, got %q", lines[3])
	}
}

func TestRunner_TimeoutFailure(t *testing.T) {
	runner := hooks.NewRunner(time.Minute)
	phases := []hooks.PhaseHooks{{
		Phase: hooks.PhaseAfterSuccess,
		Hooks: []config.HookConfig{{
			Name:    "sleep",
			Cmd:     []string{os.Args[0], "-test.run=TestHookHelperProcess"},
			Timeout: 10 * time.Millisecond,
			Env: map[string]string{
				"CASCADE_HOOK_HELPER": "1",
				"CASCADE_HOOK_ACTION": "sleep",
			},
		}},
	}}

	results, err := runner.Run(context.Background(), phases, hookContext(t.TempDir()))
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if len(results) != 1 || !results[0].Timeout {
		t.Fatalf("expected timeout result, got %#v", results)
	}
}

func TestRunner_StopsSamePhaseAfterFailureAndContinuesNextPhase(t *testing.T) {
	runner := hooks.NewRunner(time.Minute)
	phases := []hooks.PhaseHooks{
		{
			Phase: hooks.PhaseAfterSuccess,
			Hooks: []config.HookConfig{
				{
					Name: "fail",
					Cmd:  []string{os.Args[0], "-test.run=TestHookHelperProcess"},
					Env: map[string]string{
						"CASCADE_HOOK_HELPER": "1",
						"CASCADE_HOOK_ACTION": "fail",
					},
				},
				{Name: "skipped", Run: "echo skipped"},
			},
		},
		{
			Phase: hooks.PhaseAlways,
			Hooks: []config.HookConfig{{Name: "always", Run: "echo always"}},
		},
	}

	results, err := runner.Run(context.Background(), phases, hookContext(t.TempDir()))
	if err == nil {
		t.Fatal("expected hook failure")
	}
	if len(results) != 2 {
		t.Fatalf("expected failed hook and always hook results, got %d: %#v", len(results), results)
	}
	if !results[0].Failed() || results[0].ExitCode != 7 {
		t.Fatalf("expected first result to fail with exit 7, got %#v", results[0])
	}
	if results[1].Failed() || !strings.Contains(results[1].Output, "always") {
		t.Fatalf("expected always hook to run successfully, got %#v", results[1])
	}
}

func hookContext(moduleDir string) hooks.Context {
	return hooks.Context{
		Command:      "update local",
		Module:       "example.com/app",
		ModuleDir:    moduleDir,
		Workspace:    filepath.Dir(moduleDir),
		UpdateStatus: "success",
		UpdatedCount: 1,
		TidyRan:      true,
		TidyFailed:   false,
	}
}

func samePath(a, b string) bool {
	cleanA, errA := filepath.EvalSymlinks(a)
	cleanB, errB := filepath.EvalSymlinks(b)
	if errA == nil && errB == nil {
		return cleanA == cleanB
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
