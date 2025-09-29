package planner

import (
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// CompareVersions compares two semantic versions and returns true if current < target.
// Returns false if current >= target (up-to-date or ahead).
//
// Handles:
// - Standard semantic versions (v1.2.3, 1.2.3)
// - Pre-release versions (v1.0.0-alpha < v1.0.0)
// - Build metadata (v1.0.0+build ignored in comparison)
// - Pseudo-versions (v0.0.0-20230101120000-abcdef123456)
//
// Returns error for invalid version strings.
func CompareVersions(current, target string) (bool, error) {
	// Normalize versions by stripping 'v' prefix if present
	currentNorm := normalizeVersion(current)
	targetNorm := normalizeVersion(target)

	// Handle pseudo-versions specially
	if isPseudoVersion(currentNorm) {
		// If current is pseudo-version but target is not, assume update needed
		if !isPseudoVersion(targetNorm) {
			return true, nil
		}
		// Both pseudo-versions: compare lexicographically
		return currentNorm < targetNorm, nil
	}

	// Parse as semantic versions
	currentVer, err := semver.NewVersion(currentNorm)
	if err != nil {
		return false, fmt.Errorf("invalid current version %q: %w", current, err)
	}

	targetVer, err := semver.NewVersion(targetNorm)
	if err != nil {
		return false, fmt.Errorf("invalid target version %q: %w", target, err)
	}

	// Return true if current < target (needs update)
	// Return false if current >= target (up-to-date)
	return currentVer.LessThan(targetVer), nil
}

// normalizeVersion strips the 'v' prefix from a version string if present.
// Examples: v1.2.3 -> 1.2.3, 1.2.3 -> 1.2.3
func normalizeVersion(version string) string {
	return strings.TrimPrefix(version, "v")
}

// isPseudoVersion checks if a version string is a Go pseudo-version.
// Pseudo-versions have the format: v0.0.0-yyyymmddhhmmss-abcdefabcdef
// or: v0.0.0-20060102150405-abcdefabcdef
func isPseudoVersion(version string) bool {
	// Pseudo-versions contain a timestamp component after a dash
	// They typically start with v0.0.0- but can have other base versions
	// Check for the characteristic timestamp pattern: YYYYMMDDHHMMSS (14 digits)
	parts := strings.Split(version, "-")
	if len(parts) < 3 {
		return false
	}

	// Second part should be a 14-digit timestamp
	timestamp := parts[len(parts)-2]
	if len(timestamp) != 14 {
		return false
	}

	// Check if timestamp is all digits
	for _, ch := range timestamp {
		if ch < '0' || ch > '9' {
			return false
		}
	}

	// Last part should be a commit hash (12 characters, hex)
	commitHash := parts[len(parts)-1]
	if len(commitHash) != 12 {
		return false
	}

	for _, ch := range commitHash {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
			return false
		}
	}

	return true
}
