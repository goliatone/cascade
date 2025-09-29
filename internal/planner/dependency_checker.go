package planner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goliatone/cascade/internal/manifest"
)

// Logger defines the interface for logging.
type Logger interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
	Debug(msg string, args ...any)
	Warn(msg string, args ...any)
}

// dependencyChecker implements the DependencyChecker interface.
type dependencyChecker struct {
	logger Logger
}

// NewDependencyChecker creates a new DependencyChecker with optional logger.
func NewDependencyChecker(logger Logger) DependencyChecker {
	return &dependencyChecker{
		logger: logger,
	}
}

// NeedsUpdate determines if a dependent repository needs an update to the target version.
// It returns true if the update is needed, false if already up-to-date or dependency not found.
//
// Edge case handling:
// - Repository not found in workspace → return true (assume needs cloning/updating)
// - go.mod not found → return error (malformed repository)
// - Dependency not in go.mod → return false with warning (nothing to update)
// - Replace directive present → return true with warning (manual review needed)
// - Parse failures → return error with context
func (c *dependencyChecker) NeedsUpdate(ctx context.Context, dependent manifest.Dependent, target Target, workspace string) (bool, error) {
	if c.logger != nil {
		c.logger.Debug("checking dependency version",
			"repo", dependent.Repo,
			"module", target.Module,
			"target_version", target.Version)
	}

	// 1. Locate repository in workspace
	repoPath, err := c.locateRepository(dependent, workspace)
	if err != nil {
		// Repository not found - assume it needs to be cloned and updated
		if c.logger != nil {
			c.logger.Warn("repository not found in workspace, assuming update needed",
				"repo", dependent.Repo,
				"workspace", workspace,
				"error", err.Error())
		}
		return true, nil
	}

	// 2. Find go.mod file
	goModPath, err := findGoModFile(repoPath)
	if err != nil {
		// go.mod not found is a hard error (malformed repository)
		return false, &DependencyCheckError{
			Dependent: dependent.Repo,
			Target:    target,
			Err:       fmt.Errorf("go.mod not found: %w", err),
		}
	}

	// 3. Parse go.mod
	modInfo, err := ParseGoMod(goModPath)
	if err != nil {
		return false, &DependencyCheckError{
			Dependent: dependent.Repo,
			Target:    target,
			Err:       fmt.Errorf("failed to parse go.mod: %w", err),
		}
	}

	// 4. Extract current dependency version
	currentVersion, err := ExtractDependency(modInfo, target.Module)
	if err != nil {
		// Dependency not found in go.mod - nothing to update
		if strings.Contains(err.Error(), "not found") {
			if c.logger != nil {
				c.logger.Warn("dependency not found in go.mod, skipping update",
					"repo", dependent.Repo,
					"module", target.Module,
					"reason", "manifest may be stale or dependency is indirect")
			}
			return false, nil
		}

		// Replace directive with local path - needs manual review
		if strings.Contains(err.Error(), "replace directive") {
			if c.logger != nil {
				c.logger.Warn("dependency has local replace directive, assuming update needed",
					"repo", dependent.Repo,
					"module", target.Module,
					"reason", "manual review required for replace directives")
			}
			return true, nil
		}

		// Other parse errors
		return false, &DependencyCheckError{
			Dependent: dependent.Repo,
			Target:    target,
			Err:       err,
		}
	}

	// 5. Compare versions
	needsUpdate, err := CompareVersions(currentVersion, target.Version)
	if err != nil {
		return false, &DependencyCheckError{
			Dependent: dependent.Repo,
			Target:    target,
			Err:       fmt.Errorf("failed to compare versions: %w", err),
		}
	}

	if c.logger != nil {
		if needsUpdate {
			c.logger.Info("dependency needs update",
				"repo", dependent.Repo,
				"module", target.Module,
				"current_version", currentVersion,
				"target_version", target.Version)
		} else {
			c.logger.Debug("dependency already up-to-date",
				"repo", dependent.Repo,
				"module", target.Module,
				"version", currentVersion)
		}
	}

	return needsUpdate, nil
}

// locateRepository finds the repository path in the workspace.
func (c *dependencyChecker) locateRepository(dependent manifest.Dependent, workspace string) (string, error) {
	if workspace == "" {
		return "", fmt.Errorf("workspace path not configured")
	}

	// Extract repository name from repo field (e.g., "goliatone/go-crud" -> "go-crud")
	parts := strings.Split(dependent.Repo, "/")
	if len(parts) == 0 {
		return "", fmt.Errorf("invalid repo format: %s", dependent.Repo)
	}
	repoName := parts[len(parts)-1]

	// Try direct path: workspace/repoName
	repoPath := filepath.Join(workspace, repoName)
	if c.directoryExists(repoPath) {
		return repoPath, nil
	}

	// Try with org: workspace/org/repoName
	if len(parts) >= 2 {
		org := parts[len(parts)-2]
		repoPath = filepath.Join(workspace, org, repoName)
		if c.directoryExists(repoPath) {
			return repoPath, nil
		}
	}

	// Try full path: workspace/host/org/repoName
	if len(parts) >= 3 {
		repoPath = filepath.Join(workspace, parts[len(parts)-3], parts[len(parts)-2], repoName)
		if c.directoryExists(repoPath) {
			return repoPath, nil
		}
	}

	return "", fmt.Errorf("repository not found in workspace")
}

// directoryExists checks if a path exists and is a directory.
func (c *dependencyChecker) directoryExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
