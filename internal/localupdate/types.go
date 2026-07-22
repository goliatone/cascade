package localupdate

import (
	"strings"
	"time"
)

const DefaultPrefix = "github.com/goliatone/"

type Request struct {
	CurrentModule     string
	ModuleDir         string
	Workspace         string
	WorkspaceExplicit bool
	Prefixes          []string
	IncludeIndirect   bool
	Only              []string
	Exclude           []string
	Tidy              bool
	DryRun            bool
}

type Status string

const (
	StatusUpdate               Status = "update"
	StatusCurrent              Status = "current"
	StatusSkippedIndirect      Status = "skipped-indirect"
	StatusSkippedFilter        Status = "skipped-filter"
	StatusSkippedReplace       Status = "skipped-replace"
	StatusMissingLocalRepo     Status = "missing-local-repo"
	StatusMissingVersionFile   Status = "missing-version-file"
	StatusInvalidVersion       Status = "invalid-version"
	StatusInvalidLocalModule   Status = "invalid-local-module"
	StatusAmbiguousLocalModule Status = "ambiguous-local-module"
	StatusComparisonFailed     Status = "comparison-failed"
	StatusApplyFailed          Status = "apply-failed"
	StatusApplied              Status = "applied"
)

type Item struct {
	Module         string
	CurrentVersion string
	LocalVersion   string
	LocalPath      string
	Indirect       bool
	Status         Status
	NeedsUpdate    bool
	Reason         string
}

type Plan struct {
	CurrentModule string
	ModuleDir     string
	Workspace     string
	Items         []Item
}

func (p Plan) Updates() []Item {
	items := make([]Item, 0)
	for _, item := range p.Items {
		if item.NeedsUpdate {
			items = append(items, item)
		}
	}
	return items
}

type ModuleTarget struct {
	ModulePath string
	ModuleDir  string
}

type Repository struct {
	Root         string
	WorkFile     string
	Modules      []ModuleTarget
	ExternalUses []string
}

type RepositoryPlan struct {
	Repository Repository
	Plans      []Plan
}

func (p RepositoryPlan) Updates() int {
	count := 0
	for _, plan := range p.Plans {
		count += len(plan.Updates())
	}
	return count
}

func (p RepositoryPlan) Candidates() int {
	count := 0
	for _, plan := range p.Plans {
		count += len(plan.Items)
	}
	return count
}

func (p RepositoryPlan) Workspace() string {
	if len(p.Plans) == 0 {
		return ""
	}
	return p.Plans[0].Workspace
}

type ApplyOptions struct {
	DryRun bool
	Tidy   bool
	Notify func(ApplyEvent)
}

type ApplyEventKind string

const (
	ApplyBatchStarted  ApplyEventKind = "batch-started"
	ApplyBatchFinished ApplyEventKind = "batch-finished"
	ApplyBatchFallback ApplyEventKind = "batch-fallback"
	ApplyItemStarted   ApplyEventKind = "item-started"
	ApplyItemFinished  ApplyEventKind = "item-finished"
	ApplyTidyStarted   ApplyEventKind = "tidy-started"
	ApplyTidyFinished  ApplyEventKind = "tidy-finished"
)

type ApplyEvent struct {
	Kind      ApplyEventKind
	Module    string
	ModuleDir string
	Item      Item
	Index     int
	Total     int
	Err       error
	Duration  time.Duration
}

type ApplyResult struct {
	Plan           Plan
	Items          []Item
	GoGetCount     int
	GoCommandCount int
	TidyRun        bool
	TidyFailed     bool
	TidyError      error
	Interruption   error
	HasFailures    bool
}

type RepositoryApplyResult struct {
	Plan         RepositoryPlan
	Results      []*ApplyResult
	Interruption error
	HasFailures  bool
}

func (r *RepositoryApplyResult) UpdatedCount() int {
	count := 0
	if r == nil {
		return count
	}
	for _, result := range r.Results {
		if result != nil {
			count += result.GoGetCount
		}
	}
	return count
}

func (r *RepositoryApplyResult) TidyCount() int {
	count := 0
	if r == nil {
		return count
	}
	for _, result := range r.Results {
		if result != nil && result.TidyRun {
			count++
		}
	}
	return count
}

func (r *RepositoryApplyResult) TidyFailed() bool {
	if r == nil {
		return false
	}
	for _, result := range r.Results {
		if result != nil && result.TidyFailed {
			return true
		}
	}
	return false
}

func NormalizeRequest(req Request) Request {
	req.Prefixes = splitValues(req.Prefixes)
	req.Only = splitValues(req.Only)
	req.Exclude = splitValues(req.Exclude)
	if len(req.Prefixes) == 0 {
		req.Prefixes = []string{DefaultPrefix}
	}
	return req
}

func splitValues(values []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, value := range values {
		for part := range strings.SplitSeq(value, ",") {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" || seen[trimmed] {
				continue
			}
			seen[trimmed] = true
			out = append(out, trimmed)
		}
	}
	return out
}
