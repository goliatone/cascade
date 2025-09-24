package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// PrepareEnv merges base and custom environment maps into a string slice.
func PrepareEnv(base, custom map[string]string) []string {
	result := make(map[string]string)
	for k, v := range base {
		result[k] = v
	}
	for k, v := range custom {
		result[k] = v
	}

	env := make([]string, 0, len(result))
	for k, v := range result {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}

// ValidateTimeout ensures timeout is not negative; returns default 15m if zero.
func ValidateTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return 15 * time.Minute
	}
	return timeout
}

// CreateTempWorkspace creates an isolated workspace directory.
func CreateTempWorkspace(base, item string) (string, error) {
	if base == "" {
		base = os.TempDir()
	}
	path := filepath.Join(base, item)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", err
	}
	return path, nil
}

// CleanupWorkspace removes the workspace directory.
func CleanupWorkspace(path string) error {
	if path == "" {
		return nil
	}
	return os.RemoveAll(path)
}
