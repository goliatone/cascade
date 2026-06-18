package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveWatchTargetsExpandsGlobs(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "one", ".version")
	second := filepath.Join(dir, "two", ".version")
	if err := os.MkdirAll(filepath.Dir(first), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(second), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, []byte("v1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("v1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	targets, err := resolveWatchTargets([]string{filepath.Join(dir, "*", ".version")})
	if err != nil {
		t.Fatalf("resolveWatchTargets() error = %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("target count = %d, want 2", len(targets))
	}
}

func TestResolveWatchTargetsRejectsBadGlob(t *testing.T) {
	_, err := resolveWatchTargets([]string{"["})
	if err == nil {
		t.Fatal("resolveWatchTargets() error = nil, want bad glob error")
	}
}

func TestWatchCommandRequiresExec(t *testing.T) {
	target := filepath.Join(t.TempDir(), ".version")
	if err := os.WriteFile(target, []byte("v1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := newRootCommand()
	root.SetArgs([]string{"watch", target, "--once"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want missing exec error")
	}
	if !strings.Contains(err.Error(), "exec command is required") {
		t.Fatalf("Execute() error = %v, want missing exec message", err)
	}
}

func TestWatchCommandRejectsMissingPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), ".version")
	root := newRootCommand()
	root.SetArgs([]string{"watch", missing, "--exec", "echo changed"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want missing path error")
	}
	if !strings.Contains(err.Error(), "invalid watch targets") {
		t.Fatalf("Execute() error = %v, want invalid targets message", err)
	}
}

func TestWatchCommandOnceRunsCommand(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, ".version")
	marker := filepath.Join(dir, "marker")
	if err := os.WriteFile(target, []byte("v1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := newRootCommand()
	root.SetArgs([]string{
		"watch", target,
		"--once",
		"--exec", fmt.Sprintf("printf changed > %s", shellQuote(marker)),
		"--workspace", dir,
	})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "changed" {
		t.Fatalf("marker = %q, want changed", string(got))
	}
}

func TestWatchCommandVerboseOnceWritesLifecycleLogs(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, ".version")
	marker := filepath.Join(dir, "marker")
	if err := os.WriteFile(target, []byte("v1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	root := newRootCommand()
	root.SetArgs([]string{
		"watch", target,
		"--once",
		"--verbose",
		"--exec", fmt.Sprintf("printf changed > %s", shellQuote(marker)),
		"--workspace", dir,
	})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&stderr)

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := stderr.String()
	for _, want := range []string{
		"automation: starting run-1 for " + target,
		"automation: finished run-1 with exit code 0",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stderr = %q, want %q", got, want)
		}
	}
}

func TestWatchCommandRunsOnFileChange(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, ".version")
	marker := filepath.Join(dir, "marker")
	if err := os.WriteFile(target, []byte("v1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	root := newRootCommand()
	root.SetContext(ctx)
	root.SetArgs([]string{
		"watch", target,
		"--exec", fmt.Sprintf("printf hit > %s", shellQuote(marker)),
		"--debounce", "10ms",
		"--workspace", dir,
	})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})

	errs := make(chan error, 1)
	go func() {
		errs <- root.Execute()
	}()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := os.WriteFile(target, []byte(time.Now().Format(time.RFC3339Nano)), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(marker); err == nil {
			cancel()
			if err := <-errs; err != nil {
				t.Fatalf("Execute() error after cancel = %v", err)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for watch command shutdown")
	}
	t.Fatal("marker file was not written")
}

func TestSignalAwareContextFollowsParentCancellation(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	ctx, stop := signalAwareContext(parent)
	defer stop()

	cancelParent()

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("signal-aware context did not follow parent cancellation")
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
