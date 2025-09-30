package gitutil

import (
	"fmt"
	"regexp"
	"strings"
)

// ValidateBranchName validates a git branch name according to git rules.
// Returns an error if the name is invalid.
func ValidateBranchName(name string) error {
	if len(name) == 0 {
		return fmt.Errorf("branch name cannot be empty")
	}

	if len(name) > 250 {
		return fmt.Errorf("branch name exceeds maximum length of 250 characters")
	}

	// Check for invalid patterns
	if strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") {
		return fmt.Errorf("branch name cannot start or end with '/'")
	}

	if strings.Contains(name, "..") {
		return fmt.Errorf("branch name cannot contain '..'")
	}

	if strings.Contains(name, "//") {
		return fmt.Errorf("branch name cannot contain '//'")
	}

	if strings.HasSuffix(name, ".lock") {
		return fmt.Errorf("branch name cannot end with '.lock'")
	}

	// Check for control characters
	for _, c := range name {
		if c < 32 || c == 127 {
			return fmt.Errorf("branch name cannot contain control characters")
		}
	}

	return nil
}

// ValidateRepoName validates a GitHub repository name.
// Returns an error if the name is invalid.
func ValidateRepoName(name string) error {
	return validateGitHubName(name, "repository")
}

// ValidateOwnerName validates a GitHub owner or organization name.
// Returns an error if the name is invalid.
func ValidateOwnerName(name string) error {
	return validateGitHubName(name, "owner")
}

// validateGitHubName checks if a name follows GitHub naming conventions.
func validateGitHubName(name, fieldType string) error {
	if len(name) == 0 {
		return fmt.Errorf("%s name cannot be empty", fieldType)
	}

	if len(name) > 100 {
		return fmt.Errorf("%s name exceeds maximum length of 100 characters", fieldType)
	}

	// GitHub names can contain alphanumeric characters, hyphens, underscores, and dots
	// but cannot start or end with hyphens or dots
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?$`, name)
	if !matched {
		return fmt.Errorf("invalid %s name: must start and end with alphanumeric characters and can only contain alphanumeric, dots, hyphens, and underscores", fieldType)
	}

	return nil
}

// IsValidGitHubName checks if a name follows GitHub naming conventions.
// Returns true if valid, false otherwise.
func IsValidGitHubName(name string) bool {
	if len(name) == 0 || len(name) > 100 {
		return false
	}
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?$`, name)
	return matched
}

// SanitizeBranchName sanitizes a string to create a valid git branch name.
// Replaces or removes invalid characters.
func SanitizeBranchName(input string) string {
	// Replace common problematic characters
	sanitized := strings.ReplaceAll(input, "/", "-")
	sanitized = strings.ReplaceAll(sanitized, "@", "-at-")
	sanitized = strings.ReplaceAll(sanitized, "+", "-plus-")
	sanitized = strings.ReplaceAll(sanitized, ".", "-")
	sanitized = strings.ReplaceAll(sanitized, " ", "-")
	sanitized = strings.ReplaceAll(sanitized, ":", "-")

	// Remove any remaining problematic characters
	re := regexp.MustCompile(`[^a-zA-Z0-9-_]`)
	sanitized = re.ReplaceAllString(sanitized, "")

	// Remove consecutive dashes
	re = regexp.MustCompile(`-+`)
	sanitized = re.ReplaceAllString(sanitized, "-")

	// Trim dashes from start and end
	sanitized = strings.Trim(sanitized, "-")

	// Ensure we have something left
	if sanitized == "" {
		sanitized = "branch"
	}

	// Enforce maximum length
	if len(sanitized) > 250 {
		sanitized = sanitized[:250]
		// Trim trailing dash if we cut in the middle of a word
		sanitized = strings.TrimRight(sanitized, "-")
	}

	return sanitized
}

// IsValidLabelName checks if a label name follows GitHub label rules.
func IsValidLabelName(name string) bool {
	if len(name) == 0 || len(name) > 50 {
		return false
	}
	// GitHub labels cannot contain certain characters
	invalidChars := []string{",", ";", "\"", "'", "<", ">", "&"}
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
		for _, char := range []string{",", ";", "\"", "'", "<", ">", "&"} {
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
