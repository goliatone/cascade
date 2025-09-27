package main

import (
	"time"

	"github.com/goliatone/cascade/internal/state"
	"github.com/goliatone/cascade/pkg/di"
)

// stateTracker persists per-item state and run summary updates during orchestration.
type stateTracker struct {
	module   string
	version  string
	summary  *state.Summary
	manager  state.Manager
	logger   di.Logger
	existing map[string]state.ItemState
}

func newStateTracker(module, version string, summary *state.Summary, manager state.Manager, logger di.Logger, existing []state.ItemState) *stateTracker {
	if summary == nil {
		summary = &state.Summary{
			Module:    module,
			Version:   version,
			StartTime: time.Now(),
		}
	} else {
		if summary.Module == "" {
			summary.Module = module
		}
		if summary.Version == "" {
			summary.Version = version
		}
		if summary.StartTime.IsZero() {
			summary.StartTime = time.Now()
		}
	}

	tracker := &stateTracker{
		module:   module,
		version:  version,
		summary:  summary,
		manager:  manager,
		logger:   logger,
		existing: make(map[string]state.ItemState, len(existing)),
	}

	for _, st := range existing {
		tracker.existing[st.Repo] = st
	}

	tracker.saveSummary()
	return tracker
}

func (t *stateTracker) record(item state.ItemState) {
	if t == nil || item.Repo == "" {
		return
	}

	prev, hasPrev := t.existing[item.Repo]
	if hasPrev {
		if item.Attempts <= prev.Attempts {
			item.Attempts = prev.Attempts + 1
		}
		if item.PRURL == "" {
			item.PRURL = prev.PRURL
		}
	}

	if item.Attempts == 0 {
		item.Attempts = 1
	}
	if item.LastUpdated.IsZero() {
		item.LastUpdated = time.Now()
	}

	t.existing[item.Repo] = item
	replaced := false
	for i := range t.summary.Items {
		if t.summary.Items[i].Repo == item.Repo {
			t.summary.Items[i] = item
			replaced = true
			break
		}
	}
	if !replaced {
		t.summary.Items = append(t.summary.Items, item)
	}

	t.summary.EndTime = item.LastUpdated
	if t.manager != nil {
		if err := t.manager.SaveItemState(t.module, t.version, item); err != nil && t.logger != nil {
			t.logger.Warn("failed to persist item state", "repo", item.Repo, "error", err)
		}
	}

	t.saveSummary()
}

func (t *stateTracker) saveSummary() {
	if t == nil || t.manager == nil || t.summary == nil {
		return
	}

	if err := t.manager.SaveSummary(t.summary); err != nil && t.logger != nil {
		t.logger.Warn("failed to persist run summary", "module", t.module, "version", t.version, "error", err)
	}
}

func (t *stateTracker) finalize() {
	if t == nil {
		return
	}

	t.summary.EndTime = time.Now()
	t.saveSummary()
}
