package state

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/manifest"
)

func TestManagerContract(t *testing.T) {
	// Create manager with filesystem storage for full contract testing
	tmpDir := t.TempDir()
	storage, err := NewFilesystemStorage(tmpDir, nopLogger{})
	if err != nil {
		t.Fatalf("failed to create filesystem storage: %v", err)
	}

	manager := NewManager(WithStorage(storage))

	// Test LoadSummary with non-existent data
	summary, err := manager.LoadSummary("example.com/test-module", "v1.2.3")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
	if summary != nil {
		t.Errorf("expected nil summary, got %v", summary)
	}

	// Test SaveSummary with basic fixture
	testSummary := &Summary{
		Module:     "example.com/test-module",
		Version:    "v1.2.3",
		StartTime:  time.Date(2023, 12, 1, 10, 0, 0, 0, time.UTC),
		EndTime:    time.Date(2023, 12, 1, 10, 30, 0, 0, time.UTC),
		RetryCount: 0,
		Items:      []ItemState{},
	}

	err = manager.SaveSummary(testSummary)
	if err != nil {
		t.Errorf("failed to save summary: %v", err)
	}

	// Test that we can load it back
	loadedSummary, err := manager.LoadSummary("example.com/test-module", "v1.2.3")
	if err != nil {
		t.Errorf("failed to load summary: %v", err)
	}
	if loadedSummary == nil {
		t.Error("expected summary, got nil")
	} else {
		// Verify timestamps were normalized to UTC
		if loadedSummary.StartTime.Location() != time.UTC {
			t.Errorf("expected UTC timezone for start time, got %v", loadedSummary.StartTime.Location())
		}
	}

	// Test SaveItemState with basic fixture
	testItem := ItemState{
		Repo:        "github.com/example/test-repo",
		Branch:      "cascade/update-module-v1.2.3",
		Status:      executor.StatusCompleted,
		Reason:      "Update successful",
		CommitHash:  "abc123def456",
		PRURL:       "https://github.com/example/test-repo/pull/42",
		LastUpdated: time.Date(2023, 12, 1, 10, 15, 0, 0, time.UTC),
		Attempts:    1,
		CommandLogs: []executor.CommandResult{},
	}

	err = manager.SaveItemState("example.com/test-module", "v1.2.3", testItem)
	if err != nil {
		t.Errorf("failed to save item state: %v", err)
	}

	// Test LoadItemStates with basic fixture
	items, err := manager.LoadItemStates("example.com/test-module", "v1.2.3")
	if err != nil {
		t.Errorf("failed to load item states: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	} else {
		// Verify timestamp was normalized to UTC
		if items[0].LastUpdated.Location() != time.UTC {
			t.Errorf("expected UTC timezone for last updated, got %v", items[0].LastUpdated.Location())
		}
		// Verify attempts were incremented
		if items[0].Attempts != 1 {
			t.Errorf("expected attempts to be incremented to 1, got %d", items[0].Attempts)
		}
	}
}

// TestManagerValidation tests input validation in the manager layer
func TestManagerValidation(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewFilesystemStorage(tmpDir, nopLogger{})
	if err != nil {
		t.Fatalf("failed to create filesystem storage: %v", err)
	}
	manager := NewManager(WithStorage(storage))

	t.Run("LoadSummary_Validation", func(t *testing.T) {
		// Test empty module
		_, err := manager.LoadSummary("", "v1.0.0")
		if err == nil || err.Error() != "module cannot be empty" {
			t.Errorf("expected module validation error, got: %v", err)
		}

		// Test empty version
		_, err = manager.LoadSummary("example.com/test", "")
		if err == nil || err.Error() != "version cannot be empty" {
			t.Errorf("expected version validation error, got: %v", err)
		}
	})

	t.Run("SaveSummary_Validation", func(t *testing.T) {
		// Test nil summary
		err := manager.SaveSummary(nil)
		if err == nil || err.Error() != "summary cannot be nil" {
			t.Errorf("expected nil summary error, got: %v", err)
		}

		// Test empty module in summary
		summary := &Summary{
			Module:     "",
			Version:    "v1.0.0",
			StartTime:  time.Now(),
			RetryCount: 0,
			Items:      []ItemState{},
		}
		err = manager.SaveSummary(summary)
		if err == nil || err.Error() != "module cannot be empty" {
			t.Errorf("expected module validation error, got: %v", err)
		}
	})

	t.Run("SaveItemState_Validation", func(t *testing.T) {
		// Test empty repo
		item := ItemState{
			Repo:        "",
			Branch:      "test-branch",
			Status:      executor.StatusCompleted,
			LastUpdated: time.Now(),
		}
		err := manager.SaveItemState("example.com/test", "v1.0.0", item)
		if err == nil || err.Error() != "item repo cannot be empty" {
			t.Errorf("expected repo validation error, got: %v", err)
		}

		// Test invalid status
		item = ItemState{
			Repo:        "example.com/repo",
			Branch:      "test-branch",
			Status:      executor.Status("invalid"),
			LastUpdated: time.Now(),
		}
		err = manager.SaveItemState("example.com/test", "v1.0.0", item)
		if err == nil || !strings.Contains(err.Error(), "invalid item status") {
			t.Errorf("expected status validation error, got: %v", err)
		}
	})

	t.Run("TimestampNormalization", func(t *testing.T) {
		// Test with non-UTC timestamp
		location, _ := time.LoadLocation("America/New_York")
		localTime := time.Date(2023, 12, 1, 15, 30, 0, 0, location)

		summary := &Summary{
			Module:     "example.com/test",
			Version:    "v1.0.0",
			StartTime:  localTime,
			EndTime:    localTime,
			RetryCount: 0,
			Items: []ItemState{{
				Repo:        "example.com/repo",
				Branch:      "test",
				Status:      executor.StatusCompleted,
				LastUpdated: localTime,
			}},
		}

		if err := manager.SaveSummary(summary); err != nil {
			t.Fatalf("failed to save summary: %v", err)
		}

		// Load it back and verify UTC conversion
		loaded, err := manager.LoadSummary("example.com/test", "v1.0.0")
		if err != nil {
			t.Fatalf("failed to load summary: %v", err)
		}

		if loaded.StartTime.Location() != time.UTC {
			t.Errorf("expected StartTime in UTC, got %v", loaded.StartTime.Location())
		}
		if loaded.EndTime.Location() != time.UTC {
			t.Errorf("expected EndTime in UTC, got %v", loaded.EndTime.Location())
		}
		if len(loaded.Items) > 0 && loaded.Items[0].LastUpdated.Location() != time.UTC {
			t.Errorf("expected item LastUpdated in UTC, got %v", loaded.Items[0].LastUpdated.Location())
		}
	})
}

// TestFilesystemStorage tests the filesystem storage implementation
func TestFilesystemStorage(t *testing.T) {
	tmpDir := t.TempDir()
	logger := nopLogger{}

	storage, err := NewFilesystemStorage(tmpDir, logger)
	if err != nil {
		t.Fatalf("failed to create filesystem storage: %v", err)
	}

	testModule := "example.com/test-module"
	testVersion := "v1.2.3"

	t.Run("LoadSummary_NotFound", func(t *testing.T) {
		summary, err := storage.LoadSummary(testModule, testVersion)
		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
		if summary != nil {
			t.Errorf("expected nil summary, got %v", summary)
		}
	})

	t.Run("SaveAndLoadSummary", func(t *testing.T) {
		testSummary := &Summary{
			Module:     testModule,
			Version:    testVersion,
			StartTime:  time.Date(2023, 12, 1, 10, 0, 0, 0, time.UTC),
			EndTime:    time.Date(2023, 12, 1, 10, 30, 0, 0, time.UTC),
			RetryCount: 1,
			Items:      []ItemState{},
		}

		// Save the summary
		if err := storage.SaveSummary(testSummary); err != nil {
			t.Fatalf("failed to save summary: %v", err)
		}

		// Verify file was created with correct permissions
		summaryPath := filepath.Join(tmpDir, testModule, testVersion, "summary.json")
		info, err := os.Stat(summaryPath)
		if err != nil {
			t.Fatalf("summary file not created: %v", err)
		}
		if info.Mode().Perm() != 0600 {
			t.Errorf("expected file mode 0600, got %v", info.Mode().Perm())
		}

		// Load the summary
		loadedSummary, err := storage.LoadSummary(testModule, testVersion)
		if err != nil {
			t.Fatalf("failed to load summary: %v", err)
		}

		// Compare summaries
		if loadedSummary.Module != testSummary.Module {
			t.Errorf("expected module %s, got %s", testSummary.Module, loadedSummary.Module)
		}
		if loadedSummary.Version != testSummary.Version {
			t.Errorf("expected version %s, got %s", testSummary.Version, loadedSummary.Version)
		}
		if !loadedSummary.StartTime.Equal(testSummary.StartTime) {
			t.Errorf("expected start time %v, got %v", testSummary.StartTime, loadedSummary.StartTime)
		}
		if loadedSummary.RetryCount != testSummary.RetryCount {
			t.Errorf("expected retry count %d, got %d", testSummary.RetryCount, loadedSummary.RetryCount)
		}
	})

	t.Run("SaveAndLoadItemStates", func(t *testing.T) {
		testItem := ItemState{
			Repo:        "github.com/example/test-repo",
			Branch:      "cascade/update-module-v1.2.3",
			Status:      executor.StatusCompleted,
			Reason:      "Update successful",
			CommitHash:  "abc123def456",
			PRURL:       "https://github.com/example/test-repo/pull/42",
			LastUpdated: time.Date(2023, 12, 1, 10, 15, 0, 0, time.UTC),
			Attempts:    0, // Will be set to 1 by storage
			CommandLogs: []executor.CommandResult{
				{
					Command: manifest.Command{Cmd: []string{"go", "mod", "tidy"}},
					Output:  "go: downloading example.com/test-module v1.2.3",
					Err:     nil,
				},
			},
		}

		// Save the item state
		if err := storage.SaveItemState(testModule, testVersion, testItem); err != nil {
			t.Fatalf("failed to save item state: %v", err)
		}

		// Load item states
		items, err := storage.LoadItemStates(testModule, testVersion)
		if err != nil {
			t.Fatalf("failed to load item states: %v", err)
		}

		if len(items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(items))
		}

		loadedItem := items[0]
		if loadedItem.Repo != testItem.Repo {
			t.Errorf("expected repo %s, got %s", testItem.Repo, loadedItem.Repo)
		}
		if loadedItem.Status != testItem.Status {
			t.Errorf("expected status %s, got %s", testItem.Status, loadedItem.Status)
		}
		if loadedItem.Attempts != 1 { // Should be incremented
			t.Errorf("expected attempts 1, got %d", loadedItem.Attempts)
		}
		if len(loadedItem.CommandLogs) != 1 {
			t.Errorf("expected 1 command log, got %d", len(loadedItem.CommandLogs))
		}
	})

	t.Run("ItemState_AttemptsIncrement", func(t *testing.T) {
		testRepo := "github.com/example/retry-repo"
		testItem := ItemState{
			Repo:        testRepo,
			Branch:      "test-branch",
			Status:      executor.StatusFailed,
			Reason:      "First attempt failed",
			LastUpdated: time.Now().UTC(),
			Attempts:    0,
			CommandLogs: []executor.CommandResult{
				{Output: "first attempt"},
			},
		}

		// First save
		if err := storage.SaveItemState(testModule, testVersion, testItem); err != nil {
			t.Fatalf("failed to save item state: %v", err)
		}

		// Second save with different status
		testItem.Status = executor.StatusCompleted
		testItem.Reason = "Second attempt succeeded"
		testItem.CommandLogs = []executor.CommandResult{
			{Output: "second attempt"},
		}

		if err := storage.SaveItemState(testModule, testVersion, testItem); err != nil {
			t.Fatalf("failed to save item state second time: %v", err)
		}

		// Load and verify attempts were incremented and logs preserved
		items, err := storage.LoadItemStates(testModule, testVersion)
		if err != nil {
			t.Fatalf("failed to load item states: %v", err)
		}

		var found *ItemState
		for _, item := range items {
			if item.Repo == testRepo {
				found = &item
				break
			}
		}

		if found == nil {
			t.Fatalf("could not find item for repo %s", testRepo)
		}

		if found.Attempts != 2 {
			t.Errorf("expected attempts 2, got %d", found.Attempts)
		}
		if len(found.CommandLogs) != 2 {
			t.Errorf("expected 2 command logs, got %d", len(found.CommandLogs))
		}
		if found.Status != executor.StatusCompleted {
			t.Errorf("expected status completed, got %s", found.Status)
		}
	})

	t.Run("LoadItemStates_EmptyDirectory", func(t *testing.T) {
		items, err := storage.LoadItemStates("nonexistent/module", "v0.0.0")
		if err != nil {
			t.Fatalf("expected no error for empty directory, got %v", err)
		}
		if len(items) != 0 {
			t.Errorf("expected empty slice, got %d items", len(items))
		}
	})

	t.Run("CorruptData_Handling", func(t *testing.T) {
		// Create a corrupt summary file
		corruptModule := "example.com/corrupt-module"
		corruptPath := filepath.Join(tmpDir, corruptModule, testVersion, "summary.json")
		if err := os.MkdirAll(filepath.Dir(corruptPath), 0700); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
		if err := os.WriteFile(corruptPath, []byte("invalid json"), 0600); err != nil {
			t.Fatalf("failed to write corrupt file: %v", err)
		}

		// Try to load corrupt summary
		summary, err := storage.LoadSummary(corruptModule, testVersion)
		if err != ErrCorrupt {
			t.Errorf("expected ErrCorrupt, got %v", err)
		}
		if summary != nil {
			t.Errorf("expected nil summary, got %v", summary)
		}
	})
}

// TestStateDirectoryResolution tests the directory resolution logic
func TestStateDirectoryResolution(t *testing.T) {
	t.Run("CASCADE_STATE_DIR_Override", func(t *testing.T) {
		expectedDir := "/custom/state/dir"
		t.Setenv("CASCADE_STATE_DIR", expectedDir)
		t.Setenv("XDG_STATE_HOME", "/should/not/be/used")

		dir, err := resolveStateDir()
		if err != nil {
			t.Fatalf("failed to resolve state dir: %v", err)
		}
		if dir != expectedDir {
			t.Errorf("expected %s, got %s", expectedDir, dir)
		}
	})

	t.Run("XDG_STATE_HOME", func(t *testing.T) {
		t.Setenv("CASCADE_STATE_DIR", "")
		xdgDir := "/custom/xdg/state"
		t.Setenv("XDG_STATE_HOME", xdgDir)

		dir, err := resolveStateDir()
		if err != nil {
			t.Fatalf("failed to resolve state dir: %v", err)
		}
		expected := filepath.Join(xdgDir, "cascade")
		if dir != expected {
			t.Errorf("expected %s, got %s", expected, dir)
		}
	})

	t.Run("Fallback_to_Cache", func(t *testing.T) {
		t.Setenv("CASCADE_STATE_DIR", "")
		t.Setenv("XDG_STATE_HOME", "")

		dir, err := resolveStateDir()
		if err != nil {
			t.Fatalf("failed to resolve state dir: %v", err)
		}

		// Should contain .cache/cascade
		if !filepath.IsAbs(dir) {
			t.Errorf("expected absolute path, got %s", dir)
		}
		if filepath.Base(filepath.Dir(dir)) != ".cache" || filepath.Base(dir) != "cascade" {
			// More flexible check since home dir varies
			if !strings.HasSuffix(dir, filepath.Join(".cache", "cascade")) {
				t.Errorf("expected path to end with .cache/cascade, got %s", dir)
			}
		}
	})
}

// TestAtomicWrite tests the atomic write functionality
func TestAtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "test.json")
	testData := []byte(`{"test": "data"}`)

	// Test successful atomic write
	if err := atomicWrite(targetFile, testData, 0600); err != nil {
		t.Fatalf("atomic write failed: %v", err)
	}

	// Verify file exists and has correct content
	data, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if string(data) != string(testData) {
		t.Errorf("expected %s, got %s", string(testData), string(data))
	}

	// Verify file permissions
	info, err := os.Stat(targetFile)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected file mode 0600, got %v", info.Mode().Perm())
	}

	// Test that no temp files remain
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read temp dir: %v", err)
	}
	for _, entry := range entries {
		if entry.Name() != "test.json" {
			t.Errorf("unexpected file in temp dir: %s", entry.Name())
		}
	}
}

// TestFilesystemLocker tests the filesystem locking implementation
func TestFilesystemLocker(t *testing.T) {
	tmpDir := t.TempDir()
	logger := nopLogger{}
	locker := NewFilesystemLocker(tmpDir, logger)

	testModule := "example.com/test-module"
	testVersion := "v1.0.0"

	t.Run("BasicLockAcquireRelease", func(t *testing.T) {
		// Acquire lock
		guard, err := locker.Acquire(testModule, testVersion)
		if err != nil {
			t.Fatalf("failed to acquire lock: %v", err)
		}

		// Verify lock file exists
		lockPath := filepath.Join(tmpDir, testModule, testVersion, ".cascade.lock")
		if _, err := os.Stat(lockPath); err != nil {
			t.Errorf("lock file should exist: %v", err)
		}

		// Release lock
		if err := guard.Release(); err != nil {
			t.Errorf("failed to release lock: %v", err)
		}

		// Verify lock file is removed
		if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
			t.Errorf("lock file should be removed after release")
		}
	})

	t.Run("TryAcquire_NonBlocking", func(t *testing.T) {
		// First acquisition should succeed
		guard1, err := locker.TryAcquire(testModule, testVersion)
		if err != nil {
			t.Fatalf("first TryAcquire should succeed: %v", err)
		}
		defer guard1.Release()

		// Second acquisition should fail immediately
		guard2, err := locker.TryAcquire(testModule, testVersion)
		if err == nil {
			guard2.Release()
			t.Fatal("second TryAcquire should fail with ErrLocked")
		}
		if err != ErrLocked && !strings.Contains(err.Error(), "already locked by this process") {
			t.Errorf("expected ErrLocked or process lock error, got: %v", err)
		}
	})

	t.Run("ConcurrentAccess", func(t *testing.T) {
		const numWorkers = 5
		const numAttempts = 10

		var successCount int32
		var wg sync.WaitGroup

		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				for attempt := 0; attempt < numAttempts; attempt++ {
					testMod := fmt.Sprintf("example.com/worker-%d", workerID)
					testVer := fmt.Sprintf("v1.0.%d", attempt)

					guard, err := locker.TryAcquire(testMod, testVer)
					if err != nil {
						continue // Expected for concurrent access
					}

					atomic.AddInt32(&successCount, 1)

					// Hold the lock briefly
					time.Sleep(1 * time.Millisecond)

					if err := guard.Release(); err != nil {
						t.Errorf("worker %d failed to release lock: %v", workerID, err)
					}
				}
			}(i)
		}

		wg.Wait()

		// All workers should have succeeded since they're using different module/version pairs
		expected := int32(numWorkers * numAttempts)
		if successCount != expected {
			t.Errorf("expected %d successful acquisitions, got %d", expected, successCount)
		}
	})
}

// TestContextCancellation tests lock behavior with context cancellation
func TestContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	logger := nopLogger{}
	locker := NewFilesystemLocker(tmpDir, logger)

	testModule := "example.com/context-test"
	testVersion := "v1.0.0"

	t.Run("ContextCancelDuringLock", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		// This should complete before timeout
		guard, err := locker.AcquireWithContext(ctx, testModule, testVersion)
		if err != nil {
			t.Fatalf("failed to acquire lock: %v", err)
		}
		defer guard.Release()

		// Verify context is accessible
		if guard.Context() == nil {
			t.Error("lock guard should have associated context")
		}
	})

	t.Run("ContextCancelAfterAcquisition", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		guard, err := locker.AcquireWithContext(ctx, testModule, testVersion)
		if err != nil {
			t.Fatalf("failed to acquire lock: %v", err)
		}

		// Cancel context - should trigger automatic release
		cancel()
		_ = guard // Silence unused variable warning

		// Give the cleanup goroutine time to run
		time.Sleep(10 * time.Millisecond)

		// Lock should be released, so we should be able to acquire it again
		guard2, err := locker.TryAcquire(testModule, testVersion)
		if err != nil {
			t.Errorf("expected to acquire lock after context cancel, got: %v", err)
		} else {
			guard2.Release()
		}
	})

	t.Run("AlreadyCancelledContext", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := locker.AcquireWithContext(ctx, testModule, testVersion)
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got: %v", err)
		}
	})
}

// TestLockFileContent tests that lock files contain expected information
func TestLockFileContent(t *testing.T) {
	tmpDir := t.TempDir()
	logger := nopLogger{}
	locker := NewFilesystemLocker(tmpDir, logger)

	testModule := "example.com/content-test"
	testVersion := "v1.0.0"

	guard, err := locker.Acquire(testModule, testVersion)
	if err != nil {
		t.Fatalf("failed to acquire lock: %v", err)
	}
	defer guard.Release()

	lockPath := filepath.Join(tmpDir, testModule, testVersion, ".cascade.lock")
	content, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("failed to read lock file: %v", err)
	}

	contentStr := string(content)
	expectedContents := []string{
		fmt.Sprintf("pid:%d", os.Getpid()),
		"time:",
		fmt.Sprintf("module:%s", testModule),
		fmt.Sprintf("version:%s", testVersion),
	}

	for _, expected := range expectedContents {
		if !strings.Contains(contentStr, expected) {
			t.Errorf("lock file should contain %q, got: %s", expected, contentStr)
		}
	}
}

// TestLockValidation tests input validation for lock operations
func TestLockValidation(t *testing.T) {
	tmpDir := t.TempDir()
	logger := nopLogger{}
	locker := NewFilesystemLocker(tmpDir, logger)

	t.Run("EmptyModule", func(t *testing.T) {
		_, err := locker.Acquire("", "v1.0.0")
		if err == nil || !strings.Contains(err.Error(), "module and version cannot be empty") {
			t.Errorf("expected validation error for empty module, got: %v", err)
		}
	})

	t.Run("EmptyVersion", func(t *testing.T) {
		_, err := locker.Acquire("example.com/test", "")
		if err == nil || !strings.Contains(err.Error(), "module and version cannot be empty") {
			t.Errorf("expected validation error for empty version, got: %v", err)
		}
	})
}

// TestLockRaceCondition tests concurrent lock attempts on the same module/version
func TestLockRaceCondition(t *testing.T) {
	tmpDir := t.TempDir()
	logger := nopLogger{}
	locker := NewFilesystemLocker(tmpDir, logger)

	testModule := "example.com/race-test"
	testVersion := "v1.0.0"

	const numGoroutines = 10
	var successCount int32
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			guard, err := locker.TryAcquire(testModule, testVersion)
			if err == nil {
				atomic.AddInt32(&successCount, 1)
				time.Sleep(1 * time.Millisecond) // Hold lock briefly
				guard.Release()
			}
		}(i)
	}

	wg.Wait()

	// Only one goroutine should successfully acquire the lock
	if successCount != 1 {
		t.Errorf("expected exactly 1 successful lock acquisition, got %d", successCount)
	}
}
