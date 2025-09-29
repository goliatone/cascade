package planner_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/planner"
)

// BenchmarkPlanner_WithSkipChecking benchmarks planning performance with dependency checking
func BenchmarkPlanner_WithSkipChecking(b *testing.B) {
	sizes := []int{10, 50, 100}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("dependents_%d", size), func(b *testing.B) {
			// Create a manifest with N dependents
			m := createLargeManifest(size)
			target := planner.Target{
				Module:  "github.com/example/lib",
				Version: "v1.2.3",
			}

			// Create mock checker that says all are up-to-date (worst case for checking)
			checker := &mockDependencyChecker{
				needsUpdateFunc: func(ctx context.Context, dependent manifest.Dependent, target planner.Target, workspace string) (bool, error) {
					return false, nil // all up-to-date
				},
			}

			p := planner.New(
				planner.WithDependencyChecker(checker),
				planner.WithWorkspace("/tmp/workspace"),
			)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := p.Plan(context.Background(), m, target)
				if err != nil {
					b.Fatalf("Plan failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkPlanner_WithoutSkipChecking benchmarks planning performance without dependency checking
func BenchmarkPlanner_WithoutSkipChecking(b *testing.B) {
	sizes := []int{10, 50, 100}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("dependents_%d", size), func(b *testing.B) {
			// Create a manifest with N dependents
			m := createLargeManifest(size)
			target := planner.Target{
				Module:  "github.com/example/lib",
				Version: "v1.2.3",
			}

			// No checker configured
			p := planner.New()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := p.Plan(context.Background(), m, target)
				if err != nil {
					b.Fatalf("Plan failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkPlanner_SkipEfficiency compares planning with and without dependency checking
func BenchmarkPlanner_SkipEfficiency(b *testing.B) {
	// Create a test workspace with real repositories
	tempWorkspace := b.TempDir()

	// Create 50 repositories - 40 up-to-date, 10 outdated
	manifestMod := &manifest.Module{
		Module: "github.com/example/lib",
	}

	for i := 0; i < 50; i++ {
		repoName := fmt.Sprintf("repo-%d", i)
		repoPath := filepath.Join(tempWorkspace, repoName)
		if err := os.MkdirAll(repoPath, 0755); err != nil {
			b.Fatalf("failed to create repo: %v", err)
		}

		// 80% are up-to-date (realistic scenario)
		version := "v1.0.0" // outdated
		if i < 40 {
			version = "v1.2.3" // up-to-date
		}

		goModContent := fmt.Sprintf(`module github.com/example/%s

go 1.21

require github.com/example/lib %s
`, repoName, version)

		if err := os.WriteFile(filepath.Join(repoPath, "go.mod"), []byte(goModContent), 0644); err != nil {
			b.Fatalf("failed to write go.mod: %v", err)
		}

		manifestMod.Dependents = append(manifestMod.Dependents, manifest.Dependent{
			Repo:       fmt.Sprintf("example/%s", repoName),
			Module:     fmt.Sprintf("github.com/example/%s", repoName),
			ModulePath: ".",
			Branch:     "main",
		})
	}

	m := &manifest.Manifest{
		ManifestVersion: 1,
		Defaults: manifest.Defaults{
			Branch:         "main",
			CommitTemplate: "chore: bump {{module}} to {{version}}",
		},
		Modules: []manifest.Module{*manifestMod},
	}

	target := planner.Target{
		Module:  "github.com/example/lib",
		Version: "v1.2.3",
	}

	b.Run("with_skip_checking", func(b *testing.B) {
		logger := &mockLogger{}
		checker := planner.NewDependencyChecker(logger)
		p := planner.New(
			planner.WithDependencyChecker(checker),
			planner.WithWorkspace(tempWorkspace),
		)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			plan, err := p.Plan(context.Background(), m, target)
			if err != nil {
				b.Fatalf("Plan failed: %v", err)
			}
			// Should only have 10 items (outdated ones)
			if i == 0 && len(plan.Items) != 10 {
				b.Fatalf("expected 10 items (outdated), got %d", len(plan.Items))
			}
		}
	})

	b.Run("without_skip_checking", func(b *testing.B) {
		p := planner.New()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			plan, err := p.Plan(context.Background(), m, target)
			if err != nil {
				b.Fatalf("Plan failed: %v", err)
			}
			// Should have all 50 items
			if i == 0 && len(plan.Items) != 50 {
				b.Fatalf("expected 50 items (all), got %d", len(plan.Items))
			}
		}
	})
}

// createLargeManifest creates a manifest with N dependents for benchmarking
func createLargeManifest(size int) *manifest.Manifest {
	mod := manifest.Module{
		Module:     "github.com/example/lib",
		Dependents: make([]manifest.Dependent, size),
	}

	for i := 0; i < size; i++ {
		mod.Dependents[i] = manifest.Dependent{
			Repo:       fmt.Sprintf("example/repo-%d", i),
			Module:     fmt.Sprintf("github.com/example/repo-%d", i),
			ModulePath: ".",
			Branch:     "main",
		}
	}

	return &manifest.Manifest{
		ManifestVersion: 1,
		Defaults: manifest.Defaults{
			Branch:         "main",
			CommitTemplate: "chore: bump {{module}} to {{version}}",
		},
		Modules: []manifest.Module{mod},
	}
}

// mockLogger is a simple logger for benchmarks
type mockLogger struct{}

func (m *mockLogger) Debug(msg string, args ...any) {}
func (m *mockLogger) Info(msg string, args ...any)  {}
func (m *mockLogger) Warn(msg string, args ...any)  {}
func (m *mockLogger) Error(msg string, args ...any) {}
