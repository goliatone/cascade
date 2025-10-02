package executor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCaptureDependencyVersion(t *testing.T) {
	dir := t.TempDir()
	goModPath := filepath.Join(dir, "go.mod")

	initial := `module example.com/app

require example.com/pkg v1.1.0
`
	if err := os.WriteFile(goModPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("write initial go.mod: %v", err)
	}

	impact := &DependencyImpact{
		Module:        "example.com/pkg",
		TargetVersion: "v1.2.0",
	}

	captureOldDependencyVersion(impact, dir)

	if !impact.OldVersionDetected {
		t.Fatalf("expected old version to be detected")
	}

	if impact.OldVersion != "v1.1.0" {
		t.Fatalf("expected old version v1.1.0, got %q", impact.OldVersion)
	}

	updated := `module example.com/app

require example.com/pkg v1.2.0
`
	if err := os.WriteFile(goModPath, []byte(updated), 0o644); err != nil {
		t.Fatalf("write updated go.mod: %v", err)
	}

	captureNewDependencyVersion(impact, dir, "after go get")

	if !impact.NewVersionDetected {
		t.Fatalf("expected new version to be detected")
	}

	if impact.NewVersion != "v1.2.0" {
		t.Fatalf("expected new version v1.2.0, got %q", impact.NewVersion)
	}

	if !impact.Applied {
		t.Fatalf("expected dependency update to be marked as applied")
	}
}

func TestCaptureDependencyVersionMissingModule(t *testing.T) {
	dir := t.TempDir()
	goModPath := filepath.Join(dir, "go.mod")

	contents := `module example.com/app

require other.com/dep v0.1.0
`
	if err := os.WriteFile(goModPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	impact := &DependencyImpact{Module: "example.com/pkg"}
	captureOldDependencyVersion(impact, dir)

	if impact.OldVersionDetected {
		t.Fatalf("expected module to be absent")
	}

	if impact.OldVersion != "" {
		t.Fatalf("expected empty old version, got %q", impact.OldVersion)
	}
}
