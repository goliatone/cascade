package localupdate

import "strings"

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
	StatusUpdate             Status = "update"
	StatusCurrent            Status = "current"
	StatusSkippedIndirect    Status = "skipped-indirect"
	StatusSkippedFilter      Status = "skipped-filter"
	StatusSkippedReplace     Status = "skipped-replace"
	StatusMissingLocalRepo   Status = "missing-local-repo"
	StatusMissingVersionFile Status = "missing-version-file"
	StatusInvalidVersion     Status = "invalid-version"
	StatusComparisonFailed   Status = "comparison-failed"
	StatusApplyFailed        Status = "apply-failed"
	StatusApplied            Status = "applied"
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

type ApplyOptions struct {
	DryRun bool
	Tidy   bool
}

type ApplyResult struct {
	Plan        Plan
	Items       []Item
	GoGetCount  int
	TidyRun     bool
	TidyFailed  bool
	TidyError   error
	HasFailures bool
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
		for _, part := range strings.Split(value, ",") {
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
