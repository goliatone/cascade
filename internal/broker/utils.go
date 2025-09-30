package broker

import (
	"context"
	"fmt"

	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/pkg/gitutil"
)

// PRValidationError represents validation errors for PR input data.
type PRValidationError struct {
	Field   string
	Value   string
	Message string
	Err     error
}

func (e *PRValidationError) Error() string {
	if e.Value != "" && e.Err != nil {
		return fmt.Sprintf("broker: PR validation failed for field %s (value: %s): %s: %v", e.Field, e.Value, e.Message, e.Err)
	}
	if e.Value != "" {
		return fmt.Sprintf("broker: PR validation failed for field %s (value: %s): %s", e.Field, e.Value, e.Message)
	}
	if e.Err != nil {
		return fmt.Sprintf("broker: PR validation failed for field %s: %s: %v", e.Field, e.Message, e.Err)
	}
	return fmt.Sprintf("broker: PR validation failed for field %s: %s", e.Field, e.Message)
}

func (e *PRValidationError) Unwrap() error {
	return e.Err
}

// ParseRepoString parses a repository string in "owner/name" format.
// Handles various repository formats including GitHub Enterprise.
// Deprecated: Use gitutil.ExtractOwnerAndRepo() instead.
func ParseRepoString(repo string) (owner, name string, err error) {
	// Delegate to gitutil for consistent repository parsing
	owner, name, err = gitutil.ExtractOwnerAndRepo(repo)
	if err != nil {
		return "", "", err
	}

	// Validate owner and name using gitutil
	if err := gitutil.ValidateOwnerName(owner); err != nil {
		return "", "", fmt.Errorf("invalid owner name: %w", err)
	}
	if err := gitutil.ValidateRepoName(name); err != nil {
		return "", "", fmt.Errorf("invalid repository name: %w", err)
	}

	return owner, name, nil
}

// isValidGitHubName checks if a name follows GitHub naming conventions.
// Deprecated: Use gitutil.IsValidGitHubName() instead.
func isValidGitHubName(name string) bool {
	return gitutil.IsValidGitHubName(name)
}

// GenerateBranchName creates a consistent branch name for a work item.
// Format: cascade-update-{module-name}-{version}
func GenerateBranchName(item planner.WorkItem) string {
	// Sanitize module name for branch naming
	moduleName := sanitizeForBranch(item.Module)
	version := sanitizeForBranch(item.SourceVersion)

	// Use explicit branch name if provided
	if item.BranchName != "" {
		return item.BranchName
	}

	// Generate consistent branch name
	branchName := fmt.Sprintf("cascade-update-%s-%s", moduleName, version)

	// Ensure branch name is valid and not too long
	if len(branchName) > 250 { // Git has ~255 char limit for branch names
		// Truncate and add hash for uniqueness
		hash := fmt.Sprintf("%x", []byte(item.Module+item.SourceVersion))[:8]
		branchName = fmt.Sprintf("cascade-update-%s", hash)
	}

	return branchName
}

// sanitizeForBranch removes or replaces characters that are invalid in branch names.
// Deprecated: Use gitutil.SanitizeBranchName() instead.
func sanitizeForBranch(input string) string {
	return gitutil.SanitizeBranchName(input)
}

// FindExistingPR searches for an existing PR with the same head branch.
func FindExistingPR(ctx context.Context, provider Provider, repo, headBranch string) (*PullRequest, error) {
	if provider == nil {
		return nil, fmt.Errorf("provider cannot be nil")
	}

	prs, err := provider.ListPullRequests(ctx, repo, headBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to list pull requests for %s with head branch %s: %w", repo, headBranch, err)
	}

	// Return the first (most recent) PR if any exist
	if len(prs) > 0 {
		return prs[0], nil
	}

	return nil, nil
}

// ValidatePRInput validates PR input data against GitHub constraints.
func ValidatePRInput(input *PRInput) error {
	if input == nil {
		return &PRValidationError{Field: "input", Message: "PR input cannot be nil"}
	}

	// Validate repository
	if input.Repo == "" {
		return &PRValidationError{Field: "repo", Message: "repository is required"}
	}
	if _, _, err := ParseRepoString(input.Repo); err != nil {
		return &PRValidationError{Field: "repo", Message: fmt.Sprintf("invalid repository format: %v", err)}
	}

	// Validate base branch
	if input.BaseBranch == "" {
		return &PRValidationError{Field: "base_branch", Message: "base branch is required"}
	}
	if !isValidBranchName(input.BaseBranch) {
		return &PRValidationError{Field: "base_branch", Message: "invalid base branch name"}
	}

	// Validate head branch
	if input.HeadBranch == "" {
		return &PRValidationError{Field: "head_branch", Message: "head branch is required"}
	}
	if !isValidBranchName(input.HeadBranch) {
		return &PRValidationError{Field: "head_branch", Message: "invalid head branch name"}
	}

	// Validate title length (GitHub limit is ~256 characters)
	if input.Title == "" {
		return &PRValidationError{Field: "title", Message: "title is required"}
	}
	if len(input.Title) > 256 {
		return &PRValidationError{Field: "title", Message: "title exceeds 256 character limit"}
	}

	// Validate body length (GitHub limit is ~65536 characters)
	if len(input.Body) > 65536 {
		return &PRValidationError{Field: "body", Message: "body exceeds 65536 character limit"}
	}

	// Validate labels
	if len(input.Labels) > 100 { // GitHub limit
		return &PRValidationError{Field: "labels", Message: "too many labels (maximum 100)"}
	}

	for _, label := range input.Labels {
		if !isValidLabelName(label) {
			return &PRValidationError{Field: "labels", Message: fmt.Sprintf("invalid label name: %s", label)}
		}
	}

	return nil
}

// isValidBranchName checks if a branch name is valid according to Git rules.
// Deprecated: Use gitutil.ValidateBranchName() instead.
func isValidBranchName(name string) bool {
	return gitutil.ValidateBranchName(name) == nil
}

// isValidLabelName checks if a label name follows GitHub rules.
// Deprecated: Use gitutil.IsValidLabelName() instead.
func isValidLabelName(name string) bool {
	return gitutil.IsValidLabelName(name)
}

// SanitizeLabels enforces GitHub label rules and deduplicates labels.
// Deprecated: Use gitutil.SanitizeLabels() instead.
func SanitizeLabels(labels []string) []string {
	return gitutil.SanitizeLabels(labels)
}
