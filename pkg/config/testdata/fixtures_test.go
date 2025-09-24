package testdata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestLoadFixtures verifies that all test fixtures can be loaded successfully.
func TestLoadFixtures(t *testing.T) {
	testCases := []struct {
		name     string
		filename string
	}{
		{"valid config", "valid_config.yaml"},
		{"minimal config", "minimal_config.yaml"},
		{"invalid config", "invalid_config.yaml"},
		{"partial config", "partial_config.yaml"},
		{"env config", "env_config.env"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(".", tc.filename)
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("Failed to read fixture %s: %v", tc.filename, err)
			}
			if len(content) == 0 {
				t.Fatalf("Fixture %s is empty", tc.filename)
			}
		})
	}
}

// TestLoadGoldenFiles verifies that all golden files can be loaded and are valid JSON.
func TestLoadGoldenFiles(t *testing.T) {
	testCases := []struct {
		name     string
		filename string
	}{
		{"merged config", "golden/merged_config.json"},
		{"validated config", "golden/validated_config.json"},
		{"config errors", "golden/config_errors.json"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(".", tc.filename)
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("Failed to read golden file %s: %v", tc.filename, err)
			}

			// Verify it's valid JSON
			var data any
			if err := json.Unmarshal(content, &data); err != nil {
				t.Fatalf("Golden file %s contains invalid JSON: %v", tc.filename, err)
			}
		})
	}
}

// TestMockEnvironment verifies the mock environment functionality.
func TestMockEnvironment(t *testing.T) {
	mock := NewMockEnvironment(nil)

	// Test setting and getting values
	mock.Set("TEST_KEY", "test_value")
	if got := mock.Get("TEST_KEY"); got != "test_value" {
		t.Errorf("Expected 'test_value', got %s", got)
	}

	// Test LookupEnv
	value, exists := mock.LookupEnv("TEST_KEY")
	if !exists || value != "test_value" {
		t.Errorf("LookupEnv failed: exists=%v, value=%s", exists, value)
	}

	// Test non-existent key
	_, exists = mock.LookupEnv("NONEXISTENT")
	if exists {
		t.Error("Expected non-existent key to return false")
	}

	// Test loading from file
	if err := mock.LoadFromFile("env_config.env"); err != nil {
		t.Fatalf("Failed to load env file: %v", err)
	}

	if got := mock.Get("CASCADE_WORKSPACE"); got == "" {
		t.Error("Expected CASCADE_WORKSPACE to be set from env file")
	}
}

// TestMockFileSystem verifies the mock file system functionality.
func TestMockFileSystem(t *testing.T) {
	mock := NewMockFileSystem()

	// Test writing and reading files
	content := []byte("test content")
	mock.WriteFile("/test/file.txt", content)

	read, err := mock.ReadFile("/test/file.txt")
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(read) != "test content" {
		t.Errorf("Expected 'test content', got %s", string(read))
	}

	// Test file existence
	if !mock.Exists("/test/file.txt") {
		t.Error("File should exist")
	}

	if mock.Exists("/nonexistent") {
		t.Error("Non-existent file should not exist")
	}

	// Test directories
	mock.MkdirAll("/test/dir")
	if !mock.IsDir("/test/dir") {
		t.Error("Directory should exist")
	}
}

// TestMockFlagParser verifies the mock flag parser functionality.
func TestMockFlagParser(t *testing.T) {
	mock := NewMockFlagParser()

	// Test string flags
	mock.SetString("workspace", "/tmp/test")
	if got := mock.GetString("workspace"); got != "/tmp/test" {
		t.Errorf("Expected '/tmp/test', got %s", got)
	}

	// Test integer flags
	mock.SetInt("parallel", 4)
	if got := mock.GetInt("parallel"); got != 4 {
		t.Errorf("Expected 4, got %d", got)
	}

	// Test boolean flags
	mock.SetBool("dry-run", true)
	if got := mock.GetBool("dry-run"); !got {
		t.Error("Expected true, got false")
	}

	// Test duration flags
	mock.SetDuration("timeout", "5m")
	if got := mock.GetString("timeout"); got != "5m" {
		t.Errorf("Expected '5m', got %s", got)
	}

	// Test arguments
	args := []string{"command", "arg1", "arg2"}
	mock.SetArgs(args)
	if got := mock.GetArgs(); len(got) != 3 || got[0] != "command" {
		t.Errorf("Expected %v, got %v", args, got)
	}
}
