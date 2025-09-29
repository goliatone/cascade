package planner

import (
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name        string
		current     string
		target      string
		needsUpdate bool
		expectError bool
	}{
		// Basic semantic version comparisons
		{
			name:        "current less than target",
			current:     "v1.0.0",
			target:      "v1.0.1",
			needsUpdate: true,
		},
		{
			name:        "current equals target",
			current:     "v1.0.1",
			target:      "v1.0.1",
			needsUpdate: false,
		},
		{
			name:        "current greater than target",
			current:     "v1.0.2",
			target:      "v1.0.1",
			needsUpdate: false,
		},
		{
			name:        "major version difference",
			current:     "v1.5.0",
			target:      "v2.0.0",
			needsUpdate: true,
		},
		{
			name:        "minor version difference",
			current:     "v1.5.0",
			target:      "v1.6.0",
			needsUpdate: true,
		},
		{
			name:        "patch version difference",
			current:     "v1.5.0",
			target:      "v1.5.1",
			needsUpdate: true,
		},

		// Versions without 'v' prefix
		{
			name:        "no prefix - current less than target",
			current:     "1.0.0",
			target:      "1.0.1",
			needsUpdate: true,
		},
		{
			name:        "no prefix - current equals target",
			current:     "1.0.1",
			target:      "1.0.1",
			needsUpdate: false,
		},
		{
			name:        "mixed prefix - current with v, target without",
			current:     "v1.0.0",
			target:      "1.0.1",
			needsUpdate: true,
		},
		{
			name:        "mixed prefix - current without v, target with",
			current:     "1.0.0",
			target:      "v1.0.1",
			needsUpdate: true,
		},

		// Pre-release versions
		{
			name:        "pre-release less than release",
			current:     "v1.0.0-alpha",
			target:      "v1.0.0",
			needsUpdate: true,
		},
		{
			name:        "pre-release less than other pre-release",
			current:     "v1.0.0-alpha",
			target:      "v1.0.0-beta",
			needsUpdate: true,
		},
		{
			name:        "pre-release equals pre-release",
			current:     "v1.0.0-alpha.1",
			target:      "v1.0.0-alpha.1",
			needsUpdate: false,
		},
		{
			name:        "release greater than pre-release",
			current:     "v1.0.0",
			target:      "v1.0.0-beta",
			needsUpdate: false,
		},
		{
			name:        "pre-release with numbers",
			current:     "v1.0.0-rc.1",
			target:      "v1.0.0-rc.2",
			needsUpdate: true,
		},

		// Build metadata (should be ignored in comparison)
		{
			name:        "build metadata ignored - equal",
			current:     "v1.0.0+build1",
			target:      "v1.0.0+build2",
			needsUpdate: false,
		},
		{
			name:        "build metadata ignored - current less",
			current:     "v1.0.0+build1",
			target:      "v1.0.1+build2",
			needsUpdate: true,
		},
		{
			name:        "build metadata with pre-release",
			current:     "v1.0.0-alpha+build1",
			target:      "v1.0.0-alpha+build2",
			needsUpdate: false,
		},

		// Pseudo-versions
		{
			name:        "pseudo-version less than release",
			current:     "v0.0.0-20230101120000-abcdef123456",
			target:      "v1.0.0",
			needsUpdate: true,
		},
		{
			name:        "pseudo-version comparison by timestamp",
			current:     "v0.0.0-20230101120000-abcdef123456",
			target:      "v0.0.0-20230201120000-abcdef123456",
			needsUpdate: true,
		},
		{
			name:        "pseudo-version equals pseudo-version",
			current:     "v0.0.0-20230101120000-abcdef123456",
			target:      "v0.0.0-20230101120000-abcdef123456",
			needsUpdate: false,
		},
		{
			name:        "pseudo-version greater by timestamp",
			current:     "v0.0.0-20230301120000-abcdef123456",
			target:      "v0.0.0-20230201120000-abcdef123456",
			needsUpdate: false,
		},

		// Error cases
		{
			name:        "invalid current version",
			current:     "not-a-version",
			target:      "v1.0.0",
			expectError: true,
		},
		{
			name:        "invalid target version",
			current:     "v1.0.0",
			target:      "not-a-version",
			expectError: true,
		},
		{
			name:        "empty current version",
			current:     "",
			target:      "v1.0.0",
			expectError: true,
		},
		{
			name:        "empty target version",
			current:     "v1.0.0",
			target:      "",
			expectError: true,
		},
		{
			name:        "malformed version - invalid format",
			current:     "v1.x.0",
			target:      "v1.0.0",
			expectError: true,
		},
		{
			name:        "malformed version - missing version",
			current:     "version",
			target:      "v1.0.0",
			expectError: true,
		},

		// Edge cases
		{
			name:        "v0.0.0 versions",
			current:     "v0.0.0",
			target:      "v0.0.1",
			needsUpdate: true,
		},
		{
			name:        "large version numbers",
			current:     "v99.99.99",
			target:      "v100.0.0",
			needsUpdate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			needsUpdate, err := CompareVersions(tt.current, tt.target)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if needsUpdate != tt.needsUpdate {
				t.Errorf("CompareVersions(%q, %q) = %v, want %v",
					tt.current, tt.target, needsUpdate, tt.needsUpdate)
			}
		})
	}
}

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"v1.2.3", "1.2.3"},
		{"1.2.3", "1.2.3"},
		{"v0.0.0", "0.0.0"},
		{"v1.0.0-alpha", "1.0.0-alpha"},
		{"v1.0.0+build", "1.0.0+build"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeVersion(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeVersion(%q) = %q, want %q",
					tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsPseudoVersion(t *testing.T) {
	tests := []struct {
		version  string
		expected bool
	}{
		// Valid pseudo-versions
		{"0.0.0-20230101120000-abcdef123456", true},
		{"0.0.0-20200101000000-123456789abc", true},
		{"1.2.3-20230101120000-abcdef123456", true},

		// Invalid pseudo-versions
		{"1.2.3", false},
		{"1.2.3-alpha", false},
		{"0.0.0-alpha-abcdef123456", false},           // Not a timestamp
		{"0.0.0-2023010112000-abcdef123456", false},   // Timestamp too short
		{"0.0.0-20230101120000-abcdef12345", false},   // Hash too short
		{"0.0.0-20230101120000-abcdef1234567", false}, // Hash too long
		{"0.0.0-20230101120000-ABCDEF123456", false},  // Hash uppercase
		{"0.0.0-20230101120000-ghijkl123456", false},  // Hash invalid chars
		{"", false},
		{"v1.0.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			result := isPseudoVersion(tt.version)
			if result != tt.expected {
				t.Errorf("isPseudoVersion(%q) = %v, want %v",
					tt.version, result, tt.expected)
			}
		})
	}
}

// BenchmarkCompareVersions measures the performance of version comparison
func BenchmarkCompareVersions(b *testing.B) {
	testCases := []struct {
		name    string
		current string
		target  string
	}{
		{"standard", "v1.0.0", "v1.0.1"},
		{"prerelease", "v1.0.0-alpha", "v1.0.0"},
		{"build", "v1.0.0+build1", "v1.0.0+build2"},
		{"pseudo", "v0.0.0-20230101120000-abcdef123456", "v1.0.0"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _ = CompareVersions(tc.current, tc.target)
			}
		})
	}
}
