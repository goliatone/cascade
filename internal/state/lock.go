package state

// Locker guards access to state files to avoid concurrent writers.
type Locker interface {
	Acquire(module, version string) (LockGuard, error)
}

// LockGuard represents an acquired lock that must be released when finished.
type LockGuard interface {
	Release() error
}

type nopLocker struct{}

type nopLockGuard struct{}

func (nopLocker) Acquire(module, version string) (LockGuard, error) {
	return nopLockGuard{}, nil
}

func (nopLockGuard) Release() error {
	return nil
}
