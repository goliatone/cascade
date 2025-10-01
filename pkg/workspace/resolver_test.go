package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goliatone/cascade/pkg/config"
)

func TestResolve_ExplicitPath(t *testing.T) {
	tmp := t.TempDir()
	rel := "subdir"
	path := filepath.Join(tmp, rel)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	resolved := Resolve(path, nil, "", "")
	if resolved != path {
		t.Fatalf("expected %s, got %s", path, resolved)
	}
}

func TestResolve_TargetModuleWorkspace(t *testing.T) {
	tmp := t.TempDir()
	orgDir := filepath.Join(tmp, "github.com", "acme")
	targetDir := filepath.Join(orgDir, "module")
	siblingDir := filepath.Join(orgDir, "another")

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("failed to create target dir: %v", err)
	}
	if err := os.MkdirAll(siblingDir, 0o755); err != nil {
		t.Fatalf("failed to create sibling dir: %v", err)
	}

	mustWrite := func(path string) {
		if err := os.WriteFile(path, []byte("module example"), 0o644); err != nil {
			t.Fatalf("write go.mod: %v", err)
		}
	}

	mustWrite(filepath.Join(targetDir, "go.mod"))
	mustWrite(filepath.Join(siblingDir, "go.mod"))

	modulePath := "github.com/acme/module"
	workspace := Resolve("", nil, modulePath, targetDir)
	expected := orgDir
	if workspace != expected {
		t.Fatalf("expected workspace %s, got %s", expected, workspace)
	}
}

func TestResolve_ConfigFallback(t *testing.T) {
	cfg := &config.Config{
		Workspace: config.WorkspaceConfig{Path: "/tmp/workspace"},
	}
	if got := Resolve("", cfg, "example.com/unused", "/nonexistent"); got != "/tmp/workspace" {
		t.Fatalf("expected config workspace, got %s", got)
	}
}

func TestResolve_DefaultWorkspace(t *testing.T) {
	cfg := &config.Config{
		ManifestGenerator: config.ManifestGeneratorConfig{DefaultWorkspace: "/tmp/manifest-workspace"},
	}
	if got := Resolve("", cfg, "example.com/unused", "/nonexistent"); got != "/tmp/manifest-workspace" {
		t.Fatalf("expected manifest workspace, got %s", got)
	}
}

func TestDeriveModuleDirFromPath_UsesGOPATH(t *testing.T) {
	tmp := t.TempDir()
	modulePath := "github.com/acme/widget"
	moduleDir := filepath.Join(tmp, "src", modulePath)
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatalf("failed to create module dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "go.mod"), []byte("module "+modulePath), 0o644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	t.Setenv("GOPATH", tmp)

	if got := DeriveModuleDirFromPath(modulePath); got != moduleDir {
		t.Fatalf("expected %s, got %s", moduleDir, got)
	}
}
