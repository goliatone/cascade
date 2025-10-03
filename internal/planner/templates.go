package planner

import (
	"regexp"
	"strings"
)

var (
	modulePlaceholder  = regexp.MustCompile(`(?i)\{\{\s*\.?module\s*\}\}`)
	versionPlaceholder = regexp.MustCompile(`(?i)\{\{\s*\.?version\s*\}\}`)
)

// RenderCommitMessage renders a commit message template with placeholder substitution.
// Supports {{ module }} and {{ version }} placeholders via simple string replacement.
// Returns a sensible default if template is empty.
func RenderCommitMessage(template string, target Target) string {
	if template == "" {
		return "Update " + target.Module + " to " + target.Version
	}

	result := modulePlaceholder.ReplaceAllString(template, target.Module)
	result = versionPlaceholder.ReplaceAllString(result, target.Version)
	return result
}

// GenerateBranchName creates a sanitized branch name from module and version.
// Converts to lowercase and replaces problematic characters with hyphens.
func GenerateBranchName(module, version string) string {
	// Extract just the module name from the full path (e.g., "go-errors" from "github.com/goliatone/go-errors")
	parts := strings.Split(module, "/")
	moduleName := parts[len(parts)-1]

	// Sanitize module name and version separately
	cleanModule := sanitizeBranchSegment(moduleName)
	cleanVersion := sanitizeBranchSegment(version)

	// Combine with "auto/" prefix and format
	branchName := "auto/" + cleanModule + "-" + cleanVersion

	return branchName
}

// sanitizeBranchSegment cleans up a single segment (module or version) for use in branch names.
func sanitizeBranchSegment(segment string) string {
	// Convert to lowercase
	result := strings.ToLower(segment)

	// Replace spaces with hyphens
	result = strings.ReplaceAll(result, " ", "-")

	// Replace multiple slashes with single slash
	for strings.Contains(result, "//") {
		result = strings.ReplaceAll(result, "//", "/")
	}

	// Replace any remaining problematic characters with hyphens
	problematicChars := []string{"@", "#", "$", "%", "^", "&", "*", "(", ")", "+", "=", "[", "]", "{", "}", "|", "\\", ":", ";", "\"", "'", "<", ">", ",", "?", "`", "~"}
	for _, char := range problematicChars {
		result = strings.ReplaceAll(result, char, "-")
	}

	// Clean up multiple consecutive hyphens
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}

	// Remove leading/trailing hyphens
	result = strings.Trim(result, "-")

	return result
}
