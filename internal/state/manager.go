// Package state provides persistence and checkpoint management for cascade runs.
//
// # Checkpoint File Format
//
// The state package stores cascade checkpoints in JSON format with the following structure:
//
// Summary files (summary.json):
//   - module: string - The Go module being updated
//   - version: string - The target version for the update
//   - start_time: RFC3339 timestamp in UTC
//   - end_time: RFC3339 timestamp in UTC (zero time if still running)
//   - retry_count: int - Number of retry attempts for this cascade
//   - items: array of item states
//
// Item state files (items/<repo>.json):
//   - repo: string - Repository name (e.g., github.com/example/repo)
//   - branch: string - Branch name for this update
//   - status: enum - One of: completed, manual-review, failed, skipped
//   - reason: string - Human-readable reason for the current status
//   - commit_hash: string - Git commit hash if changes were made
//   - pr_url: string - Pull request URL if created
//   - last_updated: RFC3339 timestamp in UTC
//   - attempts: int - Number of attempts for this repository
//   - command_logs: array of command results with output and errors
//
// # Recovery Workflow
//
// 1. Load Summary: Resume operations by calling LoadSummary(module, version)
//   - Returns ErrNotFound if no checkpoint exists for the module/version pair
//   - Returns ErrCorrupt if the checkpoint data is malformed or unreadable
//
// 2. Load Item States: Retrieve individual repository states with LoadItemStates()
//   - Filters items by status to determine which need retry or manual review
//   - Command logs provide audit trail for debugging failed operations
//
// 3. Resume Execution: Continue from where the cascade left off
//   - Skip completed items unless forced retry is requested
//   - Retry failed items up to configured attempt limits
//   - Surface manual-review items for user intervention
//
// 4. Checkpoint Updates: Save incremental progress with SaveItemState()
//   - Atomic writes prevent corruption during concurrent access
//   - Timestamps normalized to UTC for consistent sorting
//   - Command logs truncated to prevent unbounded growth
//
// # Storage Layout
//
// Files are organized hierarchically by module and version:
//
//	<state_dir>/<module>/<version>/summary.json
//	<state_dir>/<module>/<version>/items/<repo_hash>.json
//
// Where:
//   - state_dir defaults to $XDG_STATE_HOME/cascade or ~/.cache/cascade
//   - repo_hash is a SHA256 hash of the repository name for filesystem safety
//   - Retention policy automatically prunes old versions based on configuration
//
// # Concurrency and Locking
//
// Advisory locks prevent multiple cascade processes from corrupting state:
//   - Locks are acquired per module/version pair during write operations
//   - Read-only operations (dry-run, status queries) bypass locking
//   - Locks automatically release on process termination or context cancellation
//   - ErrLocked returned with actionable message when lock acquisition fails
package state

import (
	"fmt"
	"strings"

	"github.com/goliatone/cascade/internal/executor"
)

// ManagerOption configures the Manager during construction.
type ManagerOption func(*managerConfig)

type managerConfig struct {
	Storage Storage
	Locker  Locker
	Clock   Clock
	Logger  Logger
}

// NewManager constructs a state manager with the supplied options.
func NewManager(opts ...ManagerOption) Manager {
	cfg := managerConfig{}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(&cfg)
	}

	if cfg.Storage == nil {
		cfg.Storage = &nopStorage{}
	}

	if cfg.Locker == nil {
		cfg.Locker = nopLocker{}
	}

	if cfg.Clock == nil {
		cfg.Clock = systemClock{}
	}

	if cfg.Logger == nil {
		cfg.Logger = nopLogger{}
	}

	return &manager{
		storage: cfg.Storage,
		locker:  cfg.Locker,
		clock:   cfg.Clock,
		logger:  cfg.Logger,
	}
}

// WithStorage overrides the storage backend for the manager.
func WithStorage(storage Storage) ManagerOption {
	return func(cfg *managerConfig) {
		cfg.Storage = storage
	}
}

// WithLocker overrides the locking implementation for the manager.
func WithLocker(locker Locker) ManagerOption {
	return func(cfg *managerConfig) {
		cfg.Locker = locker
	}
}

// WithClock overrides the clock implementation for the manager.
func WithClock(clock Clock) ManagerOption {
	return func(cfg *managerConfig) {
		cfg.Clock = clock
	}
}

// WithLogger overrides the logger implementation for the manager.
func WithLogger(logger Logger) ManagerOption {
	return func(cfg *managerConfig) {
		cfg.Logger = logger
	}
}

type manager struct {
	storage Storage
	locker  Locker
	clock   Clock
	logger  Logger
}

func (m *manager) LoadSummary(module, version string) (*Summary, error) {
	if err := validateModuleVersion(module, version); err != nil {
		return nil, err
	}

	m.logger.Debug("Loading summary", "module", module, "version", version)
	summary, err := m.storage.LoadSummary(module, version)
	if err != nil {
		m.logger.Error("Failed to load summary", "module", module, "version", version, "error", err)
		return nil, err
	}
	m.logger.Info("Successfully loaded summary", "module", module, "version", version, "items_count", len(summary.Items))
	return summary, nil
}

func (m *manager) SaveSummary(summary *Summary) error {
	if summary == nil {
		return fmt.Errorf("summary cannot be nil")
	}

	if err := validateModuleVersion(summary.Module, summary.Version); err != nil {
		return err
	}

	m.logger.Debug("Saving summary", "module", summary.Module, "version", summary.Version, "items_count", len(summary.Items))

	// Normalize timestamps to UTC
	normalizedSummary := *summary
	normalizedSummary.StartTime = summary.StartTime.UTC()
	if !summary.EndTime.IsZero() {
		normalizedSummary.EndTime = summary.EndTime.UTC()
	}

	// Normalize item timestamps
	for i := range normalizedSummary.Items {
		normalizedSummary.Items[i].LastUpdated = normalizedSummary.Items[i].LastUpdated.UTC()
	}

	err := m.storage.SaveSummary(&normalizedSummary)
	if err != nil {
		m.logger.Error("Failed to save summary", "module", summary.Module, "version", summary.Version, "error", err)
		return err
	}
	m.logger.Info("Successfully saved summary", "module", summary.Module, "version", summary.Version)
	return nil
}

func (m *manager) SaveItemState(module, version string, item ItemState) error {
	if err := validateModuleVersion(module, version); err != nil {
		return err
	}

	if err := validateItemState(item); err != nil {
		return err
	}

	m.logger.Debug("Saving item state", "module", module, "version", version, "repo", item.Repo, "branch", item.Branch, "status", item.Status)

	// Attempts are managed by the storage layer. Callers should pass zero to
	// indicate "let persistence increment".

	// Normalize timestamp to UTC
	normalizedItem := item
	normalizedItem.LastUpdated = item.LastUpdated.UTC()

	err := m.storage.SaveItemState(module, version, normalizedItem)
	if err != nil {
		m.logger.Error("Failed to save item state", "module", module, "version", version, "repo", item.Repo, "error", err)
		return err
	}
	m.logger.Info("Successfully saved item state", "module", module, "version", version, "repo", item.Repo, "status", item.Status)
	return nil
}

func (m *manager) LoadItemStates(module, version string) ([]ItemState, error) {
	if err := validateModuleVersion(module, version); err != nil {
		return nil, err
	}

	m.logger.Debug("Loading item states", "module", module, "version", version)
	items, err := m.storage.LoadItemStates(module, version)
	if err != nil {
		m.logger.Error("Failed to load item states", "module", module, "version", version, "error", err)
		return nil, err
	}
	m.logger.Info("Successfully loaded item states", "module", module, "version", version, "count", len(items))
	return items, nil
}

// validateModuleVersion validates that module and version are present and non-empty.
func validateModuleVersion(module, version string) error {
	if strings.TrimSpace(module) == "" {
		return fmt.Errorf("module cannot be empty")
	}
	if strings.TrimSpace(version) == "" {
		return fmt.Errorf("version cannot be empty")
	}
	return nil
}

// validateItemState validates that the item state has required fields and valid status.
func validateItemState(item ItemState) error {
	if strings.TrimSpace(item.Repo) == "" {
		return fmt.Errorf("item repo cannot be empty")
	}
	if strings.TrimSpace(item.Branch) == "" {
		return fmt.Errorf("item branch cannot be empty")
	}
	if !isValidStatus(item.Status) {
		return fmt.Errorf("invalid item status: %s", item.Status)
	}
	return nil
}

// isValidStatus checks if the status enum is valid.
func isValidStatus(status executor.Status) bool {
	switch status {
	case executor.StatusCompleted, executor.StatusManualReview, executor.StatusFailed, executor.StatusSkipped:
		return true
	default:
		return false
	}
}
