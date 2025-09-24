package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// Storage persists summaries and item states for cascade executions.
type Storage interface {
	LoadSummary(module, version string) (*Summary, error)
	SaveSummary(summary *Summary) error
	SaveItemState(module, version string, item ItemState) error
	LoadItemStates(module, version string) ([]ItemState, error)
}

// filesystemStorage implements Storage using local filesystem persistence.
type filesystemStorage struct {
	rootDir string
	logger  Logger
	mu      sync.RWMutex
}

// NewFilesystemStorage creates a new filesystem-based storage implementation.
// Root directory resolution follows: config override -> $XDG_STATE_HOME/cascade -> ~/.cache/cascade
func NewFilesystemStorage(rootDir string, logger Logger) (Storage, error) {
	if rootDir == "" {
		var err error
		rootDir, err = resolveStateDir()
		if err != nil {
			return nil, fmt.Errorf("failed to resolve state directory: %w", err)
		}
	}

	if err := ensureDir(rootDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create state directory %s: %w", rootDir, err)
	}

	return &filesystemStorage{
		rootDir: rootDir,
		logger:  logger,
	}, nil
}

// resolveStateDir determines the root directory for state files.
func resolveStateDir() (string, error) {
	// Check CASCADE_STATE_DIR environment variable first
	if dir := os.Getenv("CASCADE_STATE_DIR"); dir != "" {
		return dir, nil
	}

	// Try XDG_STATE_HOME
	if xdgStateHome := os.Getenv("XDG_STATE_HOME"); xdgStateHome != "" {
		return filepath.Join(xdgStateHome, "cascade"), nil
	}

	// Fall back to ~/.cache/cascade
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}

	return filepath.Join(homeDir, ".cache", "cascade"), nil
}

// ensureDir creates a directory with the specified permissions if it doesn't exist.
func ensureDir(path string, perm os.FileMode) error {
	if err := os.MkdirAll(path, perm); err != nil {
		return err
	}
	return nil
}

// summaryPath returns the file path for a module/version summary.
func (fs *filesystemStorage) summaryPath(module, version string) string {
	return filepath.Join(fs.rootDir, module, version, "summary.json")
}

// itemsDir returns the directory path for storing individual item states.
func (fs *filesystemStorage) itemsDir(module, version string) string {
	return filepath.Join(fs.rootDir, module, version, "items")
}

// itemPath returns the file path for a specific item state.
func (fs *filesystemStorage) itemPath(module, version, repo string) string {
	hash := sha256.Sum256([]byte(repo))
	prefix := hex.EncodeToString(hash[:8])
	safe := sanitizeRepoComponent(repo)
	if safe == "" {
		safe = "repo"
	}
	name := fmt.Sprintf("%s_%s.json", prefix, safe)
	return filepath.Join(fs.itemsDir(module, version), name)
}

// LoadSummary loads a summary for the given module and version.
func (fs *filesystemStorage) LoadSummary(module, version string) (*Summary, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	path := fs.summaryPath(module, version)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to read summary file %s: %w", path, err)
	}

	var summary Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		fs.logger.Error("failed to unmarshal summary", "path", path, "error", err)
		return nil, ErrCorrupt
	}

	fs.logger.Debug("loaded summary", "module", module, "version", version, "path", path)
	return &summary, nil
}

// SaveSummary saves a summary atomically using a temp file and rename.
func (fs *filesystemStorage) SaveSummary(summary *Summary) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	path := fs.summaryPath(summary.Module, summary.Version)
	if err := ensureDir(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("failed to create directory for summary: %w", err)
	}

	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal summary: %w", err)
	}

	if err := atomicWrite(path, data, 0600); err != nil {
		return fmt.Errorf("failed to save summary to %s: %w", path, err)
	}

	fs.logger.Debug("saved summary", "module", summary.Module, "version", summary.Version, "path", path)
	return nil
}

// SaveItemState saves an individual item state, merging with existing state if present.
func (fs *filesystemStorage) SaveItemState(module, version string, item ItemState) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	itemsDir := fs.itemsDir(module, version)
	if err := ensureDir(itemsDir, 0700); err != nil {
		return fmt.Errorf("failed to create items directory: %w", err)
	}

	path := fs.itemPath(module, version, item.Repo)

	// Load existing state if present to merge attempts and preserve history
	var existing ItemState
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &existing); err == nil {
			item.Attempts = existing.Attempts + 1
			const maxCommandLogs = 50
			item.CommandLogs = append(existing.CommandLogs, item.CommandLogs...)
			if len(item.CommandLogs) > maxCommandLogs {
				item.CommandLogs = item.CommandLogs[len(item.CommandLogs)-maxCommandLogs:]
			}
		} else {
			item.Attempts = 1
		}
	} else {
		item.Attempts = 1
	}

	data, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal item state: %w", err)
	}

	if err := atomicWrite(path, data, 0600); err != nil {
		return fmt.Errorf("failed to save item state to %s: %w", path, err)
	}

	fs.logger.Debug("saved item state", "module", module, "version", version, "repo", item.Repo, "path", path)
	return nil
}

// LoadItemStates loads all item states for a module/version pair.
func (fs *filesystemStorage) LoadItemStates(module, version string) ([]ItemState, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	itemsDir := fs.itemsDir(module, version)

	entries, err := os.ReadDir(itemsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []ItemState{}, nil // Return empty slice if directory doesn't exist
		}
		return nil, fmt.Errorf("failed to read items directory %s: %w", itemsDir, err)
	}

	var items []ItemState
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(itemsDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			fs.logger.Error("failed to read item state file", "path", path, "error", err)
			continue
		}

		var item ItemState
		if err := json.Unmarshal(data, &item); err != nil {
			fs.logger.Error("failed to unmarshal item state", "path", path, "error", err)
			continue
		}

		items = append(items, item)
	}

	fs.logger.Debug("loaded item states", "module", module, "version", version, "count", len(items))
	return items, nil
}

// atomicWrite writes data to a file atomically using a temporary file and rename.
func atomicWrite(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".tmp-"+filepath.Base(path))
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Cleanup temp file on error
	defer func() {
		if tmpFile != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
		}
	}()

	if err := tmpFile.Chmod(perm); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	if _, err := tmpFile.Write(data); err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync data: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	tmpFile = nil // Prevent cleanup

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath) // Cleanup on rename failure
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// nopStorage is a stub implementation for testing.
type nopStorage struct{}

func (n *nopStorage) LoadSummary(module, version string) (*Summary, error) {
	return nil, ErrNotImplemented
}

func (n *nopStorage) SaveSummary(summary *Summary) error {
	return ErrNotImplemented
}

func (n *nopStorage) SaveItemState(module, version string, item ItemState) error {
	return ErrNotImplemented
}

func (n *nopStorage) LoadItemStates(module, version string) ([]ItemState, error) {
	return nil, ErrNotImplemented
}

var repoSanitizer = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizeRepoComponent(repo string) string {
	trimmed := strings.TrimSpace(repo)
	replaced := repoSanitizer.ReplaceAllString(trimmed, "-")
	return strings.Trim(replaced, "-._")
}
