package broker

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/goliatone/cascade/internal/planner"
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
func ParseRepoString(repo string) (owner, name string, err error) {
	if repo == "" {
		return "", "", errors.New("repository string cannot be empty")
	}

	// Handle URLs (https://github.com/owner/repo, https://enterprise.com/owner/repo)
	if strings.HasPrefix(repo, "https://") || strings.HasPrefix(repo, "http://") {
		// Extract owner/name from URL
		parts := strings.Split(repo, "/")
		if len(parts) < 5 {
			return "", "", fmt.Errorf("invalid repository URL format: %s", repo)
		}
		owner = parts[len(parts)-2]
		name = parts[len(parts)-1]
		// Remove .git suffix if present
		name = strings.TrimSuffix(name, ".git")
	} else {
		// Handle owner/name format
		parts := strings.Split(repo, "/")
		if len(parts) != 2 {
			return "", "", fmt.Errorf("repository must be in 'owner/name' format, got: %s", repo)
		}
		owner = parts[0]
		name = parts[1]
	}

	// Validate owner and name
	if owner == "" || name == "" {
		return "", "", fmt.Errorf("both owner and repository name must be non-empty")
	}

	// Basic validation for GitHub naming rules
	if !isValidGitHubName(owner) {
		return "", "", fmt.Errorf("invalid owner name: %s", owner)
	}
	if !isValidGitHubName(name) {
		return "", "", fmt.Errorf("invalid repository name: %s", name)
	}

	return owner, name, nil
}

// isValidGitHubName checks if a name follows GitHub naming conventions.
func isValidGitHubName(name string) bool {
	if len(name) == 0 || len(name) > 100 {
		return false
	}
	// GitHub names can contain alphanumeric characters, hyphens, underscores, and dots
	// but cannot start or end with hyphens or dots
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?$`, name)
	return matched
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
func sanitizeForBranch(input string) string {
	// Replace common problematic characters
	sanitized := strings.ReplaceAll(input, "/", "-")
	sanitized = strings.ReplaceAll(sanitized, "@", "-at-")
	sanitized = strings.ReplaceAll(sanitized, "+", "-plus-")
	sanitized = strings.ReplaceAll(sanitized, ".", "-")
	sanitized = strings.ReplaceAll(sanitized, " ", "-")

	// Remove any remaining problematic characters
	re := regexp.MustCompile(`[^a-zA-Z0-9-_]`)
	sanitized = re.ReplaceAllString(sanitized, "")

	// Remove consecutive dashes
	re = regexp.MustCompile(`-+`)
	sanitized = re.ReplaceAllString(sanitized, "-")

	// Trim dashes from start and end
	sanitized = strings.Trim(sanitized, "-")

	return sanitized
}

// FindExistingPR searches for an existing PR with the same head branch.
// This is a placeholder that should use the provider's list/search methods.
func FindExistingPR(ctx context.Context, provider any, repo, headBranch string) (*PullRequest, error) {
	// TODO: Implement using provider list/search methods
	// For now, return nil to indicate no existing PR found
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
func isValidBranchName(name string) bool {
	if len(name) == 0 || len(name) > 250 {
		return false
	}
	// Basic Git branch name validation
	if strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") ||
		strings.Contains(name, "..") || strings.Contains(name, "//") {
		return false
	}
	return true
}

// isValidLabelName checks if a label name follows GitHub rules.
func isValidLabelName(name string) bool {
	if len(name) == 0 || len(name) > 50 {
		return false
	}
	// GitHub labels cannot contain certain characters
	invalidChars := []string{",", ";", ":", "\"", "'", "<", ">", "&"}
	for _, char := range invalidChars {
		if strings.Contains(name, char) {
			return false
		}
	}
	return true
}

// SanitizeLabels enforces GitHub label rules and deduplicates labels.
func SanitizeLabels(labels []string) []string {
	seen := make(map[string]bool)
	var sanitized []string

	for _, label := range labels {
		// Trim whitespace
		label = strings.TrimSpace(label)

		// Skip empty labels
		if label == "" {
			continue
		}

		// Truncate if too long
		if len(label) > 50 {
			label = label[:50]
		}

		// Remove invalid characters
		for _, char := range []string{",", ";", ":", "\"", "'", "<", ">", "&"} {
			label = strings.ReplaceAll(label, char, "")
		}

		// Skip if we've seen this label already (case-insensitive deduplication)
		labelLower := strings.ToLower(label)
		if seen[labelLower] {
			continue
		}

		// Skip if label became empty after sanitization
		if label == "" {
			continue
		}

		seen[labelLower] = true
		sanitized = append(sanitized, label)
	}

	// Limit to GitHub's maximum of 100 labels
	if len(sanitized) > 100 {
		sanitized = sanitized[:100]
	}

	return sanitized
}
