package state_test

import (
	"fmt"
	"log"
	"time"

	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/state"
)

// SimpleLogger implements state.Logger for examples
type SimpleLogger struct{}

func (SimpleLogger) Debug(string, ...any) {}
func (SimpleLogger) Info(string, ...any)  {}
func (SimpleLogger) Error(string, ...any) {}

// ExampleManager_Basic demonstrates basic usage of the state manager
func ExampleManager_Basic() {
	// Create a temporary directory for this example
	tempDir := "/tmp/cascade-state-example"

	// Create filesystem storage
	storage, err := state.NewFilesystemStorage(tempDir, SimpleLogger{})
	if err != nil {
		log.Fatalf("Failed to create storage: %v", err)
	}

	// Create manager with storage
	manager := state.NewManager(state.WithStorage(storage))

	// Create a summary for a cascade run
	summary := &state.Summary{
		Module:     "example.com/my-module",
		Version:    "v1.2.3",
		StartTime:  time.Now().UTC(),
		RetryCount: 0,
		Items:      []state.ItemState{},
	}

	// Save the initial summary
	if err := manager.SaveSummary(summary); err != nil {
		log.Fatalf("Failed to save summary: %v", err)
	}

	fmt.Println("Created cascade checkpoint for example.com/my-module v1.2.3")
	// Output: Created cascade checkpoint for example.com/my-module v1.2.3
}

// ExampleManager_ItemStates demonstrates saving and loading item states
func ExampleManager_ItemStates() {
	// Setup (same as basic example)
	tempDir := "/tmp/cascade-item-example"
	storage, _ := state.NewFilesystemStorage(tempDir, SimpleLogger{})
	manager := state.NewManager(state.WithStorage(storage))

	module := "example.com/my-module"
	version := "v1.2.3"

	// Save item state for a repository
	item := state.ItemState{
		Repo:        "github.com/example/repo1",
		Branch:      "cascade-update-v1.2.3",
		Status:      executor.StatusCompleted,
		Reason:      "Successfully updated dependencies",
		CommitHash:  "abc123",
		PRURL:       "https://github.com/example/repo1/pull/42",
		LastUpdated: time.Now().UTC(),
		Attempts:    0, // Will be incremented by storage
		CommandLogs: []executor.CommandResult{},
	}

	if err := manager.SaveItemState(module, version, item); err != nil {
		log.Fatalf("Failed to save item state: %v", err)
	}

	// Load all item states for this cascade
	items, err := manager.LoadItemStates(module, version)
	if err != nil {
		log.Fatalf("Failed to load item states: %v", err)
	}

	fmt.Printf("Found %d repository updates\n", len(items))
	for _, item := range items {
		fmt.Printf("- %s: %s (attempts: %d)\n", item.Repo, item.Status, item.Attempts)
	}
	// Output: Found 1 repository updates
	// - github.com/example/repo1: completed (attempts: 1)
}

// ExampleManager_Resume demonstrates how to resume a cascade from checkpoint
func ExampleManager_Resume() {
	// Setup
	tempDir := "/tmp/cascade-resume-example"
	storage, _ := state.NewFilesystemStorage(tempDir, SimpleLogger{})
	manager := state.NewManager(state.WithStorage(storage))

	module := "example.com/my-module"
	version := "v1.2.3"

	// Simulate existing checkpoint
	summary := &state.Summary{
		Module:    module,
		Version:   version,
		StartTime: time.Now().Add(-1 * time.Hour).UTC(),
		// EndTime is zero - cascade is still running
		RetryCount: 1,
		Items:      []state.ItemState{},
	}
	manager.SaveSummary(summary)

	// Save some item states with mixed results
	items := []state.ItemState{
		{
			Repo:        "github.com/example/repo1",
			Branch:      "cascade-update-v1.2.3",
			Status:      executor.StatusCompleted,
			Reason:      "Update successful",
			LastUpdated: time.Now().UTC(),
		},
		{
			Repo:        "github.com/example/repo2",
			Branch:      "cascade-update-v1.2.3",
			Status:      executor.StatusFailed,
			Reason:      "Build tests failed",
			LastUpdated: time.Now().UTC(),
		},
		{
			Repo:        "github.com/example/repo3",
			Branch:      "cascade-update-v1.2.3",
			Status:      executor.StatusManualReview,
			Reason:      "Breaking changes detected",
			LastUpdated: time.Now().UTC(),
		},
	}

	for _, item := range items {
		manager.SaveItemState(module, version, item)
	}

	// Now demonstrate resume logic
	fmt.Println("Resuming cascade...")

	// Load the summary to check if cascade was complete
	loadedSummary, err := manager.LoadSummary(module, version)
	if err != nil {
		log.Fatalf("Failed to load summary: %v", err)
	}

	if loadedSummary.EndTime.IsZero() {
		fmt.Println("Cascade is incomplete - resuming from checkpoint")
	}

	// Load item states to see what needs work
	loadedItems, err := manager.LoadItemStates(module, version)
	if err != nil {
		log.Fatalf("Failed to load item states: %v", err)
	}

	// Categorize items by status
	statusCounts := make(map[executor.Status]int)
	for _, item := range loadedItems {
		statusCounts[item.Status]++
	}

	fmt.Printf("Checkpoint contains: %d completed, %d failed, %d manual-review\n",
		statusCounts[executor.StatusCompleted],
		statusCounts[executor.StatusFailed],
		statusCounts[executor.StatusManualReview])

	// Output: Resuming cascade...
	// Cascade is incomplete - resuming from checkpoint
	// Checkpoint contains: 1 completed, 1 failed, 1 manual-review
}

// ExampleManager_ErrorHandling demonstrates error handling patterns
func ExampleManager_ErrorHandling() {
	tempDir := "/tmp/cascade-error-example"
	storage, _ := state.NewFilesystemStorage(tempDir, SimpleLogger{})
	manager := state.NewManager(state.WithStorage(storage))

	// Try to load non-existent summary
	summary, err := manager.LoadSummary("nonexistent.com/module", "v1.0.0")
	if err == state.ErrNotFound {
		fmt.Println("Summary not found - starting fresh cascade")
	}
	_ = summary // Avoid unused variable

	// Try to save invalid item state
	invalidItem := state.ItemState{
		Repo:   "", // Empty repo should fail validation
		Branch: "test-branch",
		Status: executor.StatusCompleted,
	}

	err = manager.SaveItemState("example.com/test", "v1.0.0", invalidItem)
	if err != nil {
		fmt.Printf("Validation failed: %s\n", err.Error())
	}

	// Output: Summary not found - starting fresh cascade
	// Validation failed: item repo cannot be empty
}
