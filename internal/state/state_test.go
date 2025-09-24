package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/manifest"
)

func TestManagerContract(t *testing.T) {
	t.Skip("Contract tests will be enabled after Task 3 implementation")

	manager := NewManager()

	// Test LoadSummary with basic fixture
	summary, err := manager.LoadSummary("example.com/test-module", "v1.2.3")
	if err != ErrNotImplemented {
		t.Errorf("expected ErrNotImplemented, got %v", err)
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
	if err != ErrNotImplemented {
		t.Errorf("expected ErrNotImplemented, got %v", err)
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
	if err != ErrNotImplemented {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}

	// Test LoadItemStates with basic fixture
	items, err := manager.LoadItemStates("example.com/test-module", "v1.2.3")
	if err != ErrNotImplemented {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
	if items != nil {
		t.Errorf("expected nil items, got %v", items)
	}
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
