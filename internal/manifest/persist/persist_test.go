package persist_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	manifestpkg "github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/manifest/persist"
	"gopkg.in/yaml.v3"
)

func TestPersistorSave_MergesSanitizesAndReports(t *testing.T) {
	t.Helper()

	existing := &manifestpkg.Manifest{
		ManifestVersion: 1,
		Modules: []manifestpkg.Module{
			{
				Name:   "example-module",
				Module: "github.com/example/module",
				Repo:   "example/module",
				Dependents: []manifestpkg.Dependent{
					{
						Repo:       "example/existing",
						Module:     "github.com/example/existing",
						ModulePath: "modules/service",
					},
				},
			},
		},
	}

	generated := &manifestpkg.Manifest{
		ManifestVersion: 0, // exercise ManifestVersion recalculation
		Modules: []manifestpkg.Module{
			{
				Name:   "example-module",
				Module: "github.com/example/module",
				Repo:   "example/module",
				Dependents: []manifestpkg.Dependent{
					{
						Repo:       "example/dup",
						Module:     "github.com/example/dup",
						ModulePath: "",
					},
					{
						Repo:       "example/dup",
						Module:     "github.com/example/dup",
						ModulePath: "",
					},
					{
						Repo:       "example/new",
						Module:     "github.com/example/new",
						ModulePath: "sub/module",
					},
					{
						Repo:       "",
						Module:     "",
						ModulePath: "",
					},
				},
			},
		},
	}

	tempDir := t.TempDir()
	manifestPath := filepath.Join(tempDir, "manifest.yaml")

	existingBytes, err := yaml.Marshal(existing)
	if err != nil {
		t.Fatalf("marshal existing manifest: %v", err)
	}
	if err := os.WriteFile(manifestPath, existingBytes, 0o644); err != nil {
		t.Fatalf("write existing manifest: %v", err)
	}

	persistor := persist.NewPersistor(manifestpkg.NewLoader())

	result, err := persistor.Save(generated, persist.Options{
		Path:         manifestPath,
		TargetModule: "github.com/example/module",
		DryRun:       true,
	})
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	if result.Manifest.ManifestVersion != 1 {
		t.Fatalf("expected manifest version 1, got %d", result.Manifest.ManifestVersion)
	}

	if len(result.Manifest.Modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(result.Manifest.Modules))
	}

	dependents := result.Manifest.Modules[0].Dependents
	if len(dependents) != 2 {
		t.Fatalf("expected 2 dependents after sanitization, got %d", len(dependents))
	}

	if dependents[0].Repo != "example/dup" {
		t.Fatalf("expected first dependent to be example/dup, got %s", dependents[0].Repo)
	}
	if dependents[0].ModulePath != "." {
		t.Fatalf("expected normalized module path '.', got %s", dependents[0].ModulePath)
	}
	if dependents[1].Repo != "example/new" {
		t.Fatalf("expected second dependent to be example/new, got %s", dependents[1].Repo)
	}

	if len(result.Report.DeduplicatedRepos) != 1 || result.Report.DeduplicatedRepos[0] != "example/dup" {
		t.Fatalf("expected deduplicated repo example/dup, got %#v", result.Report.DeduplicatedRepos)
	}
	if len(result.Report.DroppedDependents) != 1 || result.Report.DroppedDependents[0].Reason != persist.DropReasonInvalid {
		t.Fatalf("expected one invalid dropped dependent, got %#v", result.Report.DroppedDependents)
	}
	if !result.Report.ManifestVersionUpdated {
		t.Fatalf("expected manifest version to be marked updated")
	}

	yamlStr := string(result.YAML)
	dupIdx := strings.Index(yamlStr, "example/dup")
	newIdx := strings.Index(yamlStr, "example/new")
	if dupIdx == -1 || newIdx == -1 {
		t.Fatalf("expected repos to be present in YAML, got:\n%s", yamlStr)
	}
	if dupIdx > newIdx {
		t.Fatalf("expected example/dup to appear before example/new in YAML:\n%s", yamlStr)
	}
	if strings.Contains(yamlStr, "repo: \"\"") {
		t.Fatalf("expected invalid dependent to be removed from YAML:\n%s", yamlStr)
	}

	// Ensure generated manifest was not mutated by Save.
	if generated.ManifestVersion != 0 {
		t.Fatalf("expected generated manifest version to remain 0, got %d", generated.ManifestVersion)
	}
	if len(generated.Modules[0].Dependents) != 4 {
		t.Fatalf("expected generated manifest to retain 4 dependents, got %d", len(generated.Modules[0].Dependents))
	}
}

func TestPersistorSave_WritesFileWhenNotDryRun(t *testing.T) {
	generated := &manifestpkg.Manifest{
		ManifestVersion: 1,
		Modules: []manifestpkg.Module{
			{
				Name:   "example-module",
				Module: "github.com/example/module",
				Repo:   "example/module",
				Dependents: []manifestpkg.Dependent{
					{
						Repo:       "example/one",
						Module:     "github.com/example/one",
						ModulePath: ".",
					},
				},
			},
		},
	}

	tempDir := t.TempDir()
	manifestPath := filepath.Join(tempDir, "output.yaml")

	persistor := persist.NewPersistor(nil)
	result, err := persistor.Save(generated, persist.Options{
		Path:   manifestPath,
		DryRun: false,
	})
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	written, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("failed to read written manifest: %v", err)
	}

	if string(written) != string(result.YAML) {
		t.Fatalf("written manifest mismatch\nwant:\n%s\ngot:\n%s", string(result.YAML), string(written))
	}
}
