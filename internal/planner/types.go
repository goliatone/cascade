package planner

import (
	"context"
	"time"

	"github.com/goliatone/cascade/internal/manifest"
)

// Target describes the module and version we are planning updates for.
type Target struct {
	Module  string
	Version string
}

// Plan is the deterministic set of work items derived from a manifest + target.
type Plan struct {
	Target Target
	Items  []WorkItem
}

// WorkItem represents the actions required to update a dependent repository.
type WorkItem struct {
	Repo          string
	CloneURL      string
	Module        string
	ModulePath    string
	SourceModule  string
	SourceVersion string
	Branch        string
	BranchName    string
	CommitMessage string
	Tests         []manifest.Command
	ExtraCommands []manifest.Command
	Labels        []string
	PR            manifest.PRConfig
	Notifications manifest.Notifications
	Env           map[string]string
	Timeout       time.Duration
	Canary        bool
	Skip          bool
}

// Metadata captures optional context for downstream consumers.
type Metadata struct {
	Summary string
}

// DependencyChecker validates whether a dependent repository needs a dependency update.
type DependencyChecker interface {
	// NeedsUpdate determines if a dependent requires an update to the target version.
	// It returns true if the update is needed, false if already up-to-date.
	// If the check cannot be performed (e.g., repository not found), returns true to fail-open.
	NeedsUpdate(ctx context.Context, dependent manifest.Dependent, target Target, workspace string) (bool, error)
}

// CheckResult captures the outcome of a dependency version check.
type CheckResult struct {
	// NeedsUpdate indicates whether the dependent requires the update
	NeedsUpdate bool

	// CurrentVersion is the version currently in the dependent's go.mod
	CurrentVersion string

	// TargetVersion is the version being released
	TargetVersion string

	// Reason explains the decision (e.g., "already up-to-date", "dependency not found", "version mismatch")
	Reason string

	// CheckedAt records when the check was performed
	CheckedAt time.Time
}

// DependencyCheckError represents an error that occurred during dependency checking.
type DependencyCheckError struct {
	// Dependent is the repository being checked
	Dependent string

	// Target is the module and version being checked for
	Target Target

	// Err is the underlying error
	Err error
}

// Error implements the error interface.
func (e *DependencyCheckError) Error() string {
	return "dependency check failed for " + e.Dependent + " (target: " + e.Target.Module + "@" + e.Target.Version + "): " + e.Err.Error()
}

// Unwrap returns the underlying error.
func (e *DependencyCheckError) Unwrap() error {
	return e.Err
}
