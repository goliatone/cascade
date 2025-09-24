package planner

import (
	"sort"

	"github.com/goliatone/cascade/internal/manifest"
)

// FilterSkipped returns a new slice containing only dependents that don't have Skip: true.
// The input slice is not modified.
func FilterSkipped(dependents []manifest.Dependent) []manifest.Dependent {
	if len(dependents) == 0 {
		return nil
	}

	var filtered []manifest.Dependent
	for _, dep := range dependents {
		if !dep.Skip {
			filtered = append(filtered, dep)
		}
	}

	// Return empty slice instead of nil when all items are filtered out
	if filtered == nil {
		return []manifest.Dependent{}
	}
	return filtered
}

// SortDependents returns a new slice with dependents sorted deterministically by repo name.
// The input slice is not modified.
func SortDependents(dependents []manifest.Dependent) []manifest.Dependent {
	if len(dependents) == 0 {
		return nil
	}

	// Create a copy to avoid mutating the input
	sorted := make([]manifest.Dependent, len(dependents))
	copy(sorted, dependents)

	// Sort by repo name for deterministic ordering
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Repo < sorted[j].Repo
	})

	return sorted
}

// SelectCanaries returns a new slice containing canary dependents.
// Currently passes through all dependents unchanged, but provides
// a placeholder for future canary selection logic.
func SelectCanaries(dependents []manifest.Dependent) []manifest.Dependent {
	if len(dependents) == 0 {
		return nil
	}

	// Future implementation will filter based on canary flags,
	// version constraints, or other selection criteria.
	// For now, pass through all dependents unchanged.
	result := make([]manifest.Dependent, len(dependents))
	copy(result, dependents)
	return result
}
