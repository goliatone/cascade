package testsupport

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// LoadFixture reads a fixture file.
func LoadFixture(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// WriteGolden serialises data as JSON to the golden path.
func WriteGolden(path string, data any) error {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, bytes, 0o644)
}

// LoadGolden deserialises JSON golden data into v.
func LoadGolden(path string, v any) error {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, v)
}

// GoldenPath resolves a golden name in testdata directory.
func GoldenPath(baseDir, name string) string {
	return filepath.Join(baseDir, name)
}

// LoadStateSummary loads a state Summary from a fixture file.
func LoadStateSummary(path string) (*StateSummary, error) {
	var summary StateSummary
	err := LoadGolden(path, &summary)
	return &summary, err
}

// LoadStateItems loads state ItemStates from a fixture file.
func LoadStateItems(path string) ([]StateItemState, error) {
	var items []StateItemState
	err := LoadGolden(path, &items)
	return items, err
}

// StateSummary represents a state summary for testing (avoiding import cycles).
type StateSummary struct {
	Module     string           `json:"module"`
	Version    string           `json:"version"`
	StartTime  string           `json:"start_time"`
	EndTime    string           `json:"end_time"`
	Items      []StateItemState `json:"items"`
	RetryCount int              `json:"retry_count"`
}

// StateItemState represents a state item for testing (avoiding import cycles).
type StateItemState struct {
	Repo        string               `json:"repo"`
	Branch      string               `json:"branch"`
	Status      string               `json:"status"`
	Reason      string               `json:"reason"`
	CommitHash  string               `json:"commit_hash"`
	PRURL       string               `json:"pr_url"`
	LastUpdated string               `json:"last_updated"`
	Attempts    int                  `json:"attempts"`
	CommandLogs []StateCommandResult `json:"command_logs"`
}

// StateCommandResult represents a command result for testing.
type StateCommandResult struct {
	Command StateCommand `json:"command"`
	Output  string       `json:"output"`
	Err     *string      `json:"err,omitempty"`
}

// StateCommand represents a command for testing.
type StateCommand struct {
	Name    string            `json:"name"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	Timeout string            `json:"timeout"`
}
