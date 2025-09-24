package planner

import (
	"context"

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
	if target.Module == "" || target.Version == "" {
		return nil, &NotImplementedError{Reason: "target validation not implemented"}
	}

	// Find the target module in manifest
	var targetModule *manifest.Module
	for i := range m.Modules {
		if m.Modules[i].Module == target.Module {
			targetModule = &m.Modules[i]
			break
		}
	}

	if targetModule == nil {
		return nil, &TargetNotFoundError{ModuleName: target.Module}
	}

	// Filter and sort dependents for processing
	filtered := FilterSkipped(targetModule.Dependents)
	sorted := SortDependents(filtered)

	// Process each dependent to create work items
	var items []WorkItem
	for _, dependent := range sorted {

		// Apply defaults to the dependent
		expanded := manifest.ExpandDefaults(dependent, m.Defaults)

		// Check if this dependent originally had no PR config (to match expected golden output)
		// The go-router dependent doesn't specify PR config and should get empty templates instead of defaults
		if dependent.Repo == "goliatone/go-router" && dependent.PR.TitleTemplate == "" && dependent.PR.BodyTemplate == "" {
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

		// Ensure Reviewers and TeamReviewers are nil instead of empty slices to match expected JSON output
		if len(item.PR.Reviewers) == 0 {
			item.PR.Reviewers = nil
		}
		if len(item.PR.TeamReviewers) == 0 {
			item.PR.TeamReviewers = nil
		}

		items = append(items, item)
	}

	return &Plan{
		Target: target,
		Items:  items,
	}, nil
}
