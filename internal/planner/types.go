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
	Stats  PlanStats
}

// PlanStats captures statistics about the planning process.
type PlanStats struct {
	// TotalDependents is the total number of dependents considered
	TotalDependents int

	// WorkItemsCreated is the number of work items that were created
	WorkItemsCreated int

	// SkippedUpToDate is the number of dependents skipped because they're already up-to-date
	SkippedUpToDate int

	// CheckErrors is the number of errors encountered during dependency checking
	CheckErrors int

	// CI/CD mode metrics
	// CheckStrategy is the strategy used for dependency checking
	CheckStrategy string

	// CacheHits is the number of cache hits during dependency checking
	CacheHits int

	// CacheMisses is the number of cache misses during dependency checking
	CacheMisses int

	// RemoteChecks is the number of remote dependency checks performed
	RemoteChecks int

	// LocalChecks is the number of local dependency checks performed
	LocalChecks int

	// ParallelChecks indicates whether parallel checking was enabled
	ParallelChecks bool

	// CheckDuration is the total time spent checking dependencies
	CheckDuration time.Duration
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

// CheckStrategy defines the strategy for dependency checking.
type CheckStrategy string

const (
	// CheckStrategyLocal uses local workspace for dependency checking
	CheckStrategyLocal CheckStrategy = "local"

	// CheckStrategyRemote uses shallow clone from remote for dependency checking
	CheckStrategyRemote CheckStrategy = "remote"

	// CheckStrategyAuto tries local first, falls back to remote
	CheckStrategyAuto CheckStrategy = "auto"
)

// RemoteDependencyChecker performs dependency checks via remote operations.
type RemoteDependencyChecker interface {
	DependencyChecker

	// Warm prepopulates cache with dependency information
	Warm(ctx context.Context, dependents []manifest.Dependent) error

	// ClearCache removes all cached dependency information
	ClearCache() error
}

// CheckOptions configures dependency checking behavior.
type CheckOptions struct {
	// Strategy determines the dependency checking mode
	Strategy CheckStrategy

	// CacheEnabled enables caching of dependency information
	CacheEnabled bool

	// CacheTTL sets the time-to-live for cache entries
	CacheTTL time.Duration

	// ParallelChecks sets the number of parallel dependency checks
	ParallelChecks int

	// ShallowClone enables shallow cloning for remote checks
	ShallowClone bool

	// Timeout sets the timeout for individual dependency checks
	Timeout time.Duration
}

// cacheKey identifies a unique repository + ref combination in the cache.
type cacheKey struct {
	cloneURL string
	ref      string // branch or tag
}

// cacheEntry stores the parsed dependency information for a repository.
type cacheEntry struct {
	goModPath    string
	dependencies map[string]string // module -> version
	cachedAt     time.Time
	ttl          time.Duration
}
