package main

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"

	"github.com/goliatone/cascade/internal/planner"
)

func TestShowPerformanceWarnings(t *testing.T) {
	tests := []struct {
		name               string
		stats              planner.PlanStats
		configuredParallel int
		wantWarnings       []string
		wantNoWarnings     bool
	}{
		{
			name: "slow checks without parallelism",
			stats: planner.PlanStats{
				CheckDuration:  35 * time.Second,
				CheckStrategy:  "remote",
				ParallelChecks: false,
			},
			configuredParallel: 1,
			wantWarnings: []string{
				"Warning: Dependency checks took 35.0s (>30s)",
				"Consider increasing parallelism with --check-parallel=8",
			},
		},
		{
			name: "slow checks with low parallelism",
			stats: planner.PlanStats{
				CheckDuration:  40 * time.Second,
				CheckStrategy:  "remote",
				ParallelChecks: true,
			},
			configuredParallel: 2,
			wantWarnings: []string{
				"Warning: Dependency checks took 40.0s (>30s)",
				"Consider increasing parallelism with --check-parallel=8",
			},
		},
		{
			name: "slow checks with good parallelism",
			stats: planner.PlanStats{
				CheckDuration:  35 * time.Second,
				CheckStrategy:  "remote",
				ParallelChecks: true,
			},
			configuredParallel: 8,
			wantWarnings: []string{
				"Warning: Dependency checks took 35.0s (>30s)",
			},
		},
		{
			name: "fast checks",
			stats: planner.PlanStats{
				CheckDuration:  10 * time.Second,
				CheckStrategy:  "remote",
				ParallelChecks: true,
			},
			configuredParallel: 4,
			wantNoWarnings:     true,
		},
		{
			name: "low cache hit rate",
			stats: planner.PlanStats{
				CheckDuration: 5 * time.Second,
				CheckStrategy: "remote",
				CacheHits:     2,
				CacheMisses:   8,
			},
			configuredParallel: 4,
			wantWarnings: []string{
				"Low cache hit rate (20%). Repeated runs may be slower than expected.",
			},
		},
		{
			name: "good cache hit rate",
			stats: planner.PlanStats{
				CheckDuration: 5 * time.Second,
				CheckStrategy: "remote",
				CacheHits:     8,
				CacheMisses:   2,
			},
			configuredParallel: 4,
			wantNoWarnings:     true,
		},
		{
			name: "low cache hit rate but too few checks",
			stats: planner.PlanStats{
				CheckDuration: 5 * time.Second,
				CheckStrategy: "remote",
				CacheHits:     1,
				CacheMisses:   2,
			},
			configuredParallel: 4,
			wantNoWarnings:     true, // Too few samples to warn
		},
		{
			name: "local strategy no cache warnings",
			stats: planner.PlanStats{
				CheckDuration: 5 * time.Second,
				CheckStrategy: "local",
				CacheHits:     1,
				CacheMisses:   9,
			},
			configuredParallel: 4,
			wantNoWarnings:     true, // Cache warnings only for remote/auto
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			showPerformanceWarnings(&tt.stats, tt.configuredParallel)

			w.Close()
			os.Stdout = old

			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := buf.String()

			if tt.wantNoWarnings {
				if output != "" {
					t.Errorf("expected no warnings, got: %s", output)
				}
				return
			}

			for _, wantWarning := range tt.wantWarnings {
				if !bytes.Contains([]byte(output), []byte(wantWarning)) {
					t.Errorf("expected warning %q not found in output:\n%s", wantWarning, output)
				}
			}
		})
	}
}

func TestShowPerformanceWarnings_CombinedWarnings(t *testing.T) {
	stats := planner.PlanStats{
		CheckDuration:  40 * time.Second,
		CheckStrategy:  "remote",
		ParallelChecks: false,
		CacheHits:      2,
		CacheMisses:    10,
	}

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	showPerformanceWarnings(&stats, 1)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	expectedWarnings := []string{
		"Warning: Dependency checks took 40.0s (>30s)",
		"Consider increasing parallelism with --check-parallel=8",
		"Low cache hit rate (17%). Repeated runs may be slower than expected.",
	}

	for _, expected := range expectedWarnings {
		if !bytes.Contains([]byte(output), []byte(expected)) {
			t.Errorf("expected warning %q not found in output:\n%s", expected, output)
		}
	}
}
