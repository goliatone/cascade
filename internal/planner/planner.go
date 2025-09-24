package planner

import (
	"context"
	"fmt"

	"github.com/goliatone/cascade/internal/manifest"
)

// Planner computes a cascade plan from a manifest and target release.
type Planner interface {
	Plan(ctx context.Context, m *manifest.Manifest, target Target) (*Plan, error)
}

// New returns a planner stub.
func New() Planner {
	return &planner{}
}

type planner struct{}

func (p *planner) Plan(ctx context.Context, m *manifest.Manifest, target Target) (*Plan, error) {
	// Validate target fields
	if target.Module == "" {
		return nil, &InvalidTargetError{Field: "module"}
	}
	if target.Version == "" {
		return nil, &InvalidTargetError{Field: "version"}
	}

	// Find the target module in manifest using the helper
	targetModule, err := manifest.FindModuleByPath(m, target.Module)
	if err != nil {
		return nil, &TargetNotFoundError{ModuleName: target.Module}
	}

	// Filter and sort dependents for processing
	filtered := FilterSkipped(targetModule.Dependents)
	canaries := SelectCanaries(filtered)
	sorted := SortDependents(canaries)

	// Process each dependent to create work items
	var items []WorkItem
	for _, dependent := range sorted {

		// Apply defaults to the dependent, with metadata about original PR config
		expanded, hadOriginalPR := manifest.ExpandDefaultsWithMetadata(dependent, m.Defaults)

		// If the dependent had no original PR config, use empty templates instead of defaults
		if !hadOriginalPR {
			expanded.PR.TitleTemplate = ""
			expanded.PR.BodyTemplate = ""
		}

		// Generate branch name and commit message using templates
		branchName := GenerateBranchName(target.Module, target.Version)
		commitMessage := RenderCommitMessage(m.Defaults.CommitTemplate, target)

		// Create work item
		item := WorkItem{
			Repo:          expanded.Repo,
			Module:        expanded.Module,
			ModulePath:    expanded.ModulePath,
			SourceModule:  target.Module,
			SourceVersion: target.Version,
			Branch:        expanded.Branch,
			BranchName:    branchName,
			CommitMessage: commitMessage,
			Tests:         expanded.Tests,
			ExtraCommands: expanded.ExtraCommands,
			Labels:        expanded.Labels,
			PR:            expanded.PR,
			Notifications: expanded.Notifications,
			Env:           expanded.Env,
			Timeout:       expanded.Timeout,
			Canary:        expanded.Canary,
			Skip:          false, // Already filtered out Skip=true above
		}

		// Validate the work item has all required fields
		if err := validateWorkItem(item, target); err != nil {
			return nil, &PlanningError{
				Target: target,
				Err:    err,
			}
		}

		// Normalize the work item (ensure maps/slices are empty instead of nil)
		item = normalizeWorkItem(item)

		// Special case: Ensure Reviewers and TeamReviewers are nil instead of empty slices to match expected JSON output
		// This preserves the existing behavior for the golden file tests
		if len(item.PR.Reviewers) == 0 {
			item.PR.Reviewers = nil
		}
		if len(item.PR.TeamReviewers) == 0 {
			item.PR.TeamReviewers = nil
		}

		items = append(items, item)
	}

	// Ensure items slice is never nil for consistent JSON marshaling
	if items == nil {
		items = []WorkItem{}
	}

	return &Plan{
		Target: target,
		Items:  items,
	}, nil
}

// validateWorkItem performs sanity checks on a WorkItem to ensure required fields
// are populated and numeric values are within reasonable bounds.
func validateWorkItem(item WorkItem, target Target) error {
	// Check required string fields are non-empty
	if item.Repo == "" {
		return fmt.Errorf("work item repo is empty")
	}
	if item.Module == "" {
		return fmt.Errorf("work item module is empty")
	}
	if item.Branch == "" {
		return fmt.Errorf("work item branch is empty")
	}
	if item.CommitMessage == "" {
		return fmt.Errorf("work item commit message is empty")
	}

	// Validate numeric/time fields stay within sane bounds
	if item.Timeout < 0 {
		return fmt.Errorf("work item timeout cannot be negative")
	}

	return nil
}

// normalizeWorkItem ensures maps and slices are empty values instead of nil
// to provide consistent JSON output.
func normalizeWorkItem(item WorkItem) WorkItem {
	// Normalize slices to empty instead of nil
	if item.Tests == nil {
		item.Tests = []manifest.Command{}
	}
	if item.ExtraCommands == nil {
		item.ExtraCommands = []manifest.Command{}
	}
	if item.Labels == nil {
		item.Labels = []string{}
	}

	// Note: Env map is left as is to preserve existing golden file behavior
	// In the future, when golden files are regenerated, this could be:
	// if item.Env == nil {
	//     item.Env = map[string]string{}
	// }

	// Note: PR config slices are handled by special case logic in Plan() method
	// to preserve existing golden file behavior

	return item
}
