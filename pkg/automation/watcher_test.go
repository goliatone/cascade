package automation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFilesystemEventSourceMissingPath(t *testing.T) {
	source := NewFilesystemEventSource()
	_, _, err := source.Watch(context.Background(), []WatchTarget{{Path: filepath.Join(t.TempDir(), "missing")}})
	if err == nil {
		t.Fatal("Watch() error = nil, want missing path error")
	}
	if !errors.Is(err, ErrWatchFailed) {
		t.Fatalf("Watch() error = %v, want ErrWatchFailed", err)
	}
}

func TestFilesystemEventSourceFileWrite(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, ".version")
	if err := os.WriteFile(target, []byte("v1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	source := NewFilesystemEventSource()
	events, errs, err := source.Watch(ctx, []WatchTarget{{Path: target}})
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	if err := os.WriteFile(target, []byte("v1.0.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(3 * time.Second)
	for {
		select {
		case event := <-events:
			if event.Path != cleanPath(target) {
				t.Fatalf("event path = %q, want %q", event.Path, cleanPath(target))
			}
			if event.WatchRoot != cleanPath(target) {
				t.Fatalf("watch root = %q, want %q", event.WatchRoot, cleanPath(target))
			}
			if event.Op == OperationUnknown {
				t.Fatalf("event op = %q, want known op", event.Op)
			}
			return
		case err := <-errs:
			t.Fatalf("watch error = %v", err)
		case <-deadline:
			t.Fatal("timed out waiting for filesystem event")
		}
	}
}

func TestFilesystemEventSourceStopsOnContextCancel(t *testing.T) {
	target := filepath.Join(t.TempDir(), ".version")
	if err := os.WriteFile(target, []byte("v1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	source := NewFilesystemEventSource()
	events, _, err := source.Watch(ctx, []WatchTarget{{Path: target}})
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	cancel()

	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("events channel remained open after cancellation")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for watcher shutdown")
	}
}

func TestFilesystemEventSourceRecursiveWatchesCreatedDirectories(t *testing.T) {
	root := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	source := &FilesystemEventSource{Recursive: true}
	events, errs, err := source.Watch(ctx, []WatchTarget{{Path: root}})
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	nested := filepath.Join(root, "created")
	target := filepath.Join(nested, ".version")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := os.WriteFile(target, []byte(time.Now().Format(time.RFC3339Nano)), 0o644); err != nil {
				t.Fatal(err)
			}
		case event := <-events:
			if event.Path == cleanPath(target) {
				return
			}
		case err := <-errs:
			t.Fatalf("watch error = %v", err)
		case <-deadline:
			t.Fatal("timed out waiting for event inside created recursive directory")
		}
	}
}
