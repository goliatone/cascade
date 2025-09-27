package planner

import (
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
