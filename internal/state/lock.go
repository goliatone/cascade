package state

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Locker guards access to state files to avoid concurrent writers.
type Locker interface {
	// Acquire attempts to acquire a lock for the given module/version pair.
	// Returns ErrLocked if the lock is already held by another process.
	Acquire(module, version string) (LockGuard, error)
	// TryAcquire attempts to acquire a lock without blocking.
	// Returns ErrLocked immediately if the lock is unavailable.
	TryAcquire(module, version string) (LockGuard, error)
	// AcquireWithContext attempts to acquire a lock with context cancellation support.
	// Returns context.Canceled if the context is cancelled before acquiring the lock.
	AcquireWithContext(ctx context.Context, module, version string) (LockGuard, error)
}

// LockGuard represents an acquired lock that must be released when finished.
// The lock is automatically released when the context is cancelled.
type LockGuard interface {
	Release() error
	// Context returns the context associated with this lock, if any.
	Context() context.Context
}

// filesystemLocker implements file-based advisory locking for state directories.
type filesystemLocker struct {
	rootDir string
	logger  Logger
	mu      sync.RWMutex
	// Track active locks to prevent double-locking within same process
	activeLocks map[string]*lockFile
}

// lockFile represents an individual lock file and its cleanup.
type lockFile struct {
	path     string
	file     *os.File
	released bool
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
}

// NewFilesystemLocker creates a new filesystem-based locker.
func NewFilesystemLocker(rootDir string, logger Logger) Locker {
	return &filesystemLocker{
		rootDir:     rootDir,
		logger:      logger,
		activeLocks: make(map[string]*lockFile),
	}
}

// Acquire attempts to acquire a lock for the given module/version pair.
func (fl *filesystemLocker) Acquire(module, version string) (LockGuard, error) {
	return fl.acquireLock(context.Background(), module, version, false)
}

// TryAcquire attempts to acquire a lock without blocking.
func (fl *filesystemLocker) TryAcquire(module, version string) (LockGuard, error) {
	return fl.acquireLock(context.Background(), module, version, true)
}

// AcquireWithContext attempts to acquire a lock with context cancellation support.
func (fl *filesystemLocker) AcquireWithContext(ctx context.Context, module, version string) (LockGuard, error) {
	return fl.acquireLock(ctx, module, version, false)
}

// acquireLock is the internal implementation for acquiring locks.
func (fl *filesystemLocker) acquireLock(ctx context.Context, module, version string, nonBlocking bool) (LockGuard, error) {
	if module == "" || version == "" {
		return nil, fmt.Errorf("module and version cannot be empty")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	lockKey := filepath.Join(module, version)
	lockPath := filepath.Join(fl.rootDir, module, version, ".cascade.lock")

	attempt := 0
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		guard, err := fl.tryAcquireOnce(ctx, lockKey, lockPath, module, version)
		if err == nil {
			return guard, nil
		}

		if !errors.Is(err, ErrLocked) {
			return nil, err
		}

		if nonBlocking {
			return nil, err
		}

		// backoff before retrying
		delay := time.Duration(100+attempt*50) * time.Millisecond
		if delay > 500*time.Millisecond {
			delay = 500 * time.Millisecond
		}
		attempt++
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (fl *filesystemLocker) tryAcquireOnce(ctx context.Context, lockKey, lockPath, module, version string) (LockGuard, error) {
	fl.mu.Lock()
	if existing, exists := fl.activeLocks[lockKey]; exists && !existing.released {
		fl.mu.Unlock()
		return nil, fmt.Errorf("%w: already locked by this process", ErrLocked)
	}
	fl.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(lockPath), 0700); err != nil {
		return nil, fmt.Errorf("failed to create lock directory: %w", err)
	}

	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if os.IsExist(err) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("failed to create lock file %s: %w", lockPath, err)
	}

	pid := os.Getpid()
	timestamp := time.Now().UTC().Format(time.RFC3339)
	lockInfo := fmt.Sprintf("pid:%d\ntime:%s\nmodule:%s\nversion:%s\n", pid, timestamp, module, version)
	if _, err := file.WriteString(lockInfo); err != nil {
		file.Close()
		os.Remove(lockPath)
		return nil, fmt.Errorf("failed to write lock info: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		os.Remove(lockPath)
		return nil, fmt.Errorf("failed to sync lock file: %w", err)
	}

	lockCtx, cancel := context.WithCancel(ctx)
	lock := &lockFile{
		path:   lockPath,
		file:   file,
		ctx:    lockCtx,
		cancel: cancel,
	}

	fl.mu.Lock()
	fl.activeLocks[lockKey] = lock
	fl.mu.Unlock()

	fl.logger.Debug("acquired lock", "module", module, "version", version, "path", lockPath)

	guard := &filesystemLockGuard{
		locker:  fl,
		lock:    lock,
		lockKey: lockKey,
		module:  module,
		version: version,
	}

	go func() {
		<-lockCtx.Done()
		if lockCtx.Err() == context.Canceled {
			guard.Release()
		}
	}()

	return guard, nil
}

// filesystemLockGuard implements LockGuard for filesystem locks.
type filesystemLockGuard struct {
	locker  *filesystemLocker
	lock    *lockFile
	lockKey string
	module  string
	version string
}

// Context returns the context associated with this lock.
func (lg *filesystemLockGuard) Context() context.Context {
	if lg.lock.ctx != nil {
		return lg.lock.ctx
	}
	return context.Background()
}

// Release releases the lock and cleans up resources.
func (lg *filesystemLockGuard) Release() error {
	lg.lock.mu.Lock()
	defer lg.lock.mu.Unlock()

	if lg.lock.released {
		return nil // Already released
	}

	lg.lock.released = true

	// Cancel the context to stop the auto-cleanup goroutine
	if lg.lock.cancel != nil {
		lg.lock.cancel()
	}

	// Close file handle
	if err := lg.lock.file.Close(); err != nil {
		lg.locker.logger.Error("failed to close lock file", "path", lg.lock.path, "error", err)
	}

	// Remove lock file
	if err := os.Remove(lg.lock.path); err != nil {
		lg.locker.logger.Error("failed to remove lock file", "path", lg.lock.path, "error", err)
	}

	// Remove from active locks map
	lg.locker.mu.Lock()
	delete(lg.locker.activeLocks, lg.lockKey)
	lg.locker.mu.Unlock()

	lg.locker.logger.Debug("released lock", "module", lg.module, "version", lg.version, "path", lg.lock.path)
	return nil
}

// nopLocker is a no-op implementation for testing.
type nopLocker struct{}

type nopLockGuard struct {
	ctx context.Context
}

func (nopLocker) Acquire(module, version string) (LockGuard, error) {
	return nopLockGuard{}, nil
}

func (nopLocker) TryAcquire(module, version string) (LockGuard, error) {
	return nopLockGuard{}, nil
}

func (nopLocker) AcquireWithContext(ctx context.Context, module, version string) (LockGuard, error) {
	return nopLockGuard{ctx: ctx}, nil
}

func (nopLockGuard) Release() error {
	return nil
}

func (g nopLockGuard) Context() context.Context {
	if g.ctx != nil {
		return g.ctx
	}
	return context.Background()
}

// NewNopLocker creates a no-op locker for testing.
func NewNopLocker() Locker {
	return nopLocker{}
}
