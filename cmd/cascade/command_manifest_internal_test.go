package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goliatone/cascade/internal/manifest"
)

func TestFilterDiscoveredDependents_DropsSelfModule(t *testing.T) {
	deps := []manifest.DependentOptions{
		{
			Repository:      "goliatone/go-errors",
			ModulePath:      "github.com/goliatone/go-errors",
			DiscoverySource: "workspace",
			LocalModulePath: ".",
			CloneURL:        "https://github.com/goliatone/go-errors",
		},
		{
			Repository:      "goliatone/go-auth",
			ModulePath:      "github.com/goliatone/go-auth",
			DiscoverySource: "workspace",
		},
	}

	filtered, skipped := filterDiscoveredDependents(deps, "github.com/goliatone/go-errors", "v0.9.0", "/workspace", nil)

	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered dependent, got %d", len(filtered))
	}
	if filtered[0].Repository != "goliatone/go-auth" {
		t.Fatalf("expected remaining repo to be go-auth, got %s", filtered[0].Repository)
	}
	if len(skipped) != 1 {
		t.Fatalf("expected 1 skipped dependent, got %d", len(skipped))
	}
}

func TestFilterDiscoveredDependents_DropsUpToDateWorkspace(t *testing.T) {
	tempDir := t.TempDir()
	repoDir := filepath.Join(tempDir, "up-to-date")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	goModPath := filepath.Join(repoDir, "go.mod")
	goModContent := "module github.com/goliatone/up-to-date\n\nrequire github.com/goliatone/go-errors v0.9.0\n"
	if err := os.WriteFile(goModPath, []byte(goModContent), 0o644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	deps := []manifest.DependentOptions{
		{
			Repository:      "goliatone/up-to-date",
			ModulePath:      "github.com/goliatone/up-to-date",
			DiscoverySource: "workspace",
			LocalModulePath: ".",
		},
		{
			Repository:      "goliatone/outdated",
			ModulePath:      "github.com/goliatone/outdated",
			DiscoverySource: "github",
		},
	}

	filtered, skipped := filterDiscoveredDependents(deps, "github.com/goliatone/go-errors", "v0.9.0", tempDir, nil)

	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered dependent, got %d", len(filtered))
	}
	if filtered[0].Repository != "goliatone/outdated" {
		t.Fatalf("expected remaining repo to be go/outdated, got %s", filtered[0].Repository)
	}
	if len(skipped) != 1 {
		t.Fatalf("expected 1 skipped dependent, got %d", len(skipped))
	}
	if skipped[0].Repository != "goliatone/up-to-date" {
		t.Fatalf("expected skipped repo to be go/up-to-date, got %s", skipped[0].Repository)
	}
}
