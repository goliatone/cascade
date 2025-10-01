package state

import (
	"errors"
	"time"

	"github.com/goliatone/cascade/internal/executor"
)

// Manager describes the persistence contract for cascade summaries and item state.
type Manager interface {
	LoadSummary(module, version string) (*Summary, error)
	SaveSummary(summary *Summary) error
	SaveItemState(module, version string, item ItemState) error
	LoadItemStates(module, version string) ([]ItemState, error)
}

// Summary captures the aggregate status of a cascade run for a module/version pair.
type Summary struct {
	Module          string      `json:"module"`
	Version         string      `json:"version"`
	StartTime       time.Time   `json:"start_time"`
	EndTime         time.Time   `json:"end_time"`
	Items           []ItemState `json:"items"`
	SkippedUpToDate []string    `json:"skipped_up_to_date,omitempty"`
	RetryCount      int         `json:"retry_count"`
}

// ItemState describes the last known status for a particular repository update.
type ItemState struct {
	Repo        string                   `json:"repo"`
	Branch      string                   `json:"branch"`
	Status      executor.Status          `json:"status"`
	Reason      string                   `json:"reason"`
	CommitHash  string                   `json:"commit_hash"`
	PRURL       string                   `json:"pr_url"`
	LastUpdated time.Time                `json:"last_updated"`
	Attempts    int                      `json:"attempts"`
	CommandLogs []executor.CommandResult `json:"command_logs"`
}

var (
	// ErrNotFound indicates that a requested summary or item state does not exist.
	ErrNotFound = errors.New("state: not found")
	// ErrCorrupt indicates that persisted state data could not be decoded.
	ErrCorrupt = errors.New("state: corrupt data")
	// ErrLocked indicates that a state file is locked by another process.
	ErrLocked = errors.New("state: locked")
	// ErrNotImplemented indicates functionality is not yet implemented.
	ErrNotImplemented = errors.New("state: not implemented")
)

// Clock exposes time retrieval for deterministic testing.
type Clock interface {
	Now() time.Time
}

// Logger captures the structured logging surface the manager relies on.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Error(msg string, args ...any)
}
