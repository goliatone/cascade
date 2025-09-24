package planner

import (
	"reflect"
	"testing"

	"github.com/goliatone/cascade/internal/manifest"
)

func TestFilterSkipped(t *testing.T) {
	tests := []struct {
		name     string
		input    []manifest.Dependent
		expected []manifest.Dependent
	}{
		{
			name:     "empty slice",
			input:    []manifest.Dependent{},
			expected: nil,
		},
		{
			name:     "nil slice",
			input:    nil,
			expected: nil,
		},
		{
			name: "no skipped dependents",
			input: []manifest.Dependent{
				{Repo: "org/repo1", Skip: false},
				{Repo: "org/repo2", Skip: false},
			},
			expected: []manifest.Dependent{
				{Repo: "org/repo1", Skip: false},
				{Repo: "org/repo2", Skip: false},
			},
		},
		{
			name: "all skipped dependents",
			input: []manifest.Dependent{
				{Repo: "org/repo1", Skip: true},
				{Repo: "org/repo2", Skip: true},
			},
			expected: []manifest.Dependent{},
		},
		{
			name: "mixed skipped and non-skipped",
			input: []manifest.Dependent{
				{Repo: "org/repo1", Skip: false},
				{Repo: "org/repo2", Skip: true},
				{Repo: "org/repo3", Skip: false},
			},
			expected: []manifest.Dependent{
				{Repo: "org/repo1", Skip: false},
				{Repo: "org/repo3", Skip: false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterSkipped(tt.input)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("Expected nil, got %v", result)
				}
				return
			}

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Expected %+v, got %+v", tt.expected, result)
			}

			// Ensure input slice is not modified
			if tt.input != nil {
				for i, dep := range tt.input {
					originalSkip := (tt.name == "all skipped dependents") || (tt.name == "mixed skipped and non-skipped" && i == 1)
					if dep.Skip != originalSkip && (tt.name == "mixed skipped and non-skipped" || tt.name == "all skipped dependents") {
						// Skip this check for other test cases
						if originalSkip && !dep.Skip {
							t.Errorf("Input slice was modified at index %d", i)
						}
					}
				}
			}
		})
	}
}

func TestSortDependents(t *testing.T) {
	tests := []struct {
		name     string
		input    []manifest.Dependent
		expected []manifest.Dependent
	}{
		{
			name:     "empty slice",
			input:    []manifest.Dependent{},
			expected: nil,
		},
		{
			name:     "nil slice",
			input:    nil,
			expected: nil,
		},
		{
			name: "single dependent",
			input: []manifest.Dependent{
				{Repo: "org/repo1"},
			},
			expected: []manifest.Dependent{
				{Repo: "org/repo1"},
			},
		},
		{
			name: "already sorted",
			input: []manifest.Dependent{
				{Repo: "org/aaa"},
				{Repo: "org/bbb"},
				{Repo: "org/ccc"},
			},
			expected: []manifest.Dependent{
				{Repo: "org/aaa"},
				{Repo: "org/bbb"},
				{Repo: "org/ccc"},
			},
		},
		{
			name: "reverse sorted",
			input: []manifest.Dependent{
				{Repo: "org/zzz"},
				{Repo: "org/bbb"},
				{Repo: "org/aaa"},
			},
			expected: []manifest.Dependent{
				{Repo: "org/aaa"},
				{Repo: "org/bbb"},
				{Repo: "org/zzz"},
			},
		},
		{
			name: "random order",
			input: []manifest.Dependent{
				{Repo: "zebra/repo"},
				{Repo: "alpha/repo"},
				{Repo: "beta/repo"},
				{Repo: "gamma/repo"},
			},
			expected: []manifest.Dependent{
				{Repo: "alpha/repo"},
				{Repo: "beta/repo"},
				{Repo: "gamma/repo"},
				{Repo: "zebra/repo"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Keep a copy of the original input to verify it's not modified
			original := make([]manifest.Dependent, len(tt.input))
			copy(original, tt.input)

			result := SortDependents(tt.input)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("Expected nil, got %v", result)
				}
				return
			}

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Expected %+v, got %+v", tt.expected, result)
			}

			// Ensure input slice is not modified
			if !reflect.DeepEqual(tt.input, original) {
				t.Errorf("Input slice was modified: original %+v, current %+v", original, tt.input)
			}

			// Verify ordering is deterministic - run multiple times
			for i := 0; i < 3; i++ {
				result2 := SortDependents(tt.input)
				if !reflect.DeepEqual(result, result2) {
					t.Errorf("Sort is not deterministic: first=%+v, iteration %d=%+v", result, i, result2)
				}
			}
		})
	}
}

func TestSelectCanaries(t *testing.T) {
	tests := []struct {
		name     string
		input    []manifest.Dependent
		expected []manifest.Dependent
	}{
		{
			name:     "empty slice",
			input:    []manifest.Dependent{},
			expected: nil,
		},
		{
			name:     "nil slice",
			input:    nil,
			expected: nil,
		},
		{
			name: "single dependent",
			input: []manifest.Dependent{
				{Repo: "org/repo1", Canary: true},
			},
			expected: []manifest.Dependent{
				{Repo: "org/repo1", Canary: true},
			},
		},
		{
			name: "mixed canary flags",
			input: []manifest.Dependent{
				{Repo: "org/repo1", Canary: true},
				{Repo: "org/repo2", Canary: false},
				{Repo: "org/repo3", Canary: true},
			},
			expected: []manifest.Dependent{
				{Repo: "org/repo1", Canary: true},
				{Repo: "org/repo2", Canary: false},
				{Repo: "org/repo3", Canary: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Keep a copy of the original input to verify it's not modified
			original := make([]manifest.Dependent, len(tt.input))
			copy(original, tt.input)

			result := SelectCanaries(tt.input)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("Expected nil, got %v", result)
				}
				return
			}

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Expected %+v, got %+v", tt.expected, result)
			}

			// Ensure input slice is not modified
			if !reflect.DeepEqual(tt.input, original) {
				t.Errorf("Input slice was modified: original %+v, current %+v", original, tt.input)
			}

			// Verify the result is a copy, not the same slice
			if len(tt.input) > 0 && &result[0] == &tt.input[0] {
				t.Error("Result slice shares the same backing array as input")
			}
		})
	}
}
