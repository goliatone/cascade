package localupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverRepositoryUsesGoWorkModulesFromAnyRepositoryDirectory(t *testing.T) {
	parent := t.TempDir()
	repositoryRoot := filepath.Join(parent, "app")
	externalDir := filepath.Join(parent, "external")
	mustMkdirAll(t, filepath.Join(repositoryRoot, ".git"))
	mustWriteModuleFile(t, repositoryRoot, "github.com/goliatone/app", "")
	mustWriteModuleFile(t, filepath.Join(repositoryRoot, "adapters", "admin"), "github.com/goliatone/app/adapters/admin", "")
	mustWriteModuleFile(t, filepath.Join(repositoryRoot, "unlisted"), "github.com/goliatone/app/unlisted", "")
	mustWriteModuleFile(t, externalDir, "github.com/goliatone/external", "")
	mustWriteFile(t, filepath.Join(repositoryRoot, "go.work"), `go 1.24

use (
	.
	./adapters/admin
	../external
)
`)
	startDir := filepath.Join(repositoryRoot, "adapters", "admin", "pkg")
	mustMkdirAll(t, startDir)

	repository, err := DiscoverRepository(startDir)
	if err != nil {
		t.Fatalf("discover repository: %v", err)
	}
	if repository.Root != repositoryRoot {
		t.Fatalf("expected root %q, got %q", repositoryRoot, repository.Root)
	}
	if repository.WorkFile != filepath.Join(repositoryRoot, "go.work") {
		t.Fatalf("unexpected work file: %q", repository.WorkFile)
	}
	if len(repository.Modules) != 2 {
		t.Fatalf("expected two in-repository work modules, got %#v", repository.Modules)
	}
	if repository.Modules[0].ModulePath != "github.com/goliatone/app" || repository.Modules[1].ModulePath != "github.com/goliatone/app/adapters/admin" {
		t.Fatalf("unexpected module order: %#v", repository.Modules)
	}
	if len(repository.ExternalUses) != 1 || repository.ExternalUses[0] != externalDir {
		t.Fatalf("expected external use %q, got %#v", externalDir, repository.ExternalUses)
	}
}

func TestDiscoverRepositoryScansNestedModulesWithoutGoWork(t *testing.T) {
	repositoryRoot := filepath.Join(t.TempDir(), "app")
	mustMkdirAll(t, filepath.Join(repositoryRoot, ".git"))
	mustWriteModuleFile(t, repositoryRoot, "github.com/goliatone/app", "")
	mustWriteModuleFile(t, filepath.Join(repositoryRoot, "adapters", "admin"), "github.com/goliatone/app/adapters/admin", "")
	for _, excluded := range []string{"vendor", "node_modules", "testdata", "fixtures", ".cache"} {
		mustWriteModuleFile(t, filepath.Join(repositoryRoot, excluded, "ignored"), "github.com/goliatone/ignored/"+excluded, "")
	}

	repository, err := DiscoverRepository(repositoryRoot)
	if err != nil {
		t.Fatalf("discover repository: %v", err)
	}
	if len(repository.Modules) != 2 {
		t.Fatalf("expected root and nested module, got %#v", repository.Modules)
	}
	if repository.Modules[0].ModuleDir != repositoryRoot || repository.Modules[1].ModuleDir != filepath.Join(repositoryRoot, "adapters", "admin") {
		t.Fatalf("unexpected discovered modules: %#v", repository.Modules)
	}
}

func TestDiscoverRepositoryFallsBackToSingleEnclosingModuleWithoutGit(t *testing.T) {
	moduleDir := filepath.Join(t.TempDir(), "app")
	mustWriteModuleFile(t, moduleDir, "github.com/goliatone/app", "")
	startDir := filepath.Join(moduleDir, "pkg", "feature")
	mustMkdirAll(t, startDir)

	repository, err := DiscoverRepository(startDir)
	if err != nil {
		t.Fatalf("discover repository: %v", err)
	}
	if repository.Root != moduleDir || len(repository.Modules) != 1 || repository.Modules[0].ModuleDir != moduleDir {
		t.Fatalf("unexpected single-module repository: %#v", repository)
	}
}

func TestDiscoverRepositoryRejectsInvalidInRepositoryWorkModule(t *testing.T) {
	repositoryRoot := filepath.Join(t.TempDir(), "app")
	mustMkdirAll(t, filepath.Join(repositoryRoot, ".git"))
	mustWriteModuleFile(t, repositoryRoot, "github.com/goliatone/app", "")
	mustMkdirAll(t, filepath.Join(repositoryRoot, "broken"))
	mustWriteFile(t, filepath.Join(repositoryRoot, "go.work"), "go 1.24\nuse (\n.\n./broken\n)\n")

	if _, err := DiscoverRepository(repositoryRoot); err == nil {
		t.Fatal("expected invalid go.work module to fail discovery")
	}
}

func mustWriteModuleFile(t *testing.T, dir, modulePath, extra string) {
	t.Helper()
	mustMkdirAll(t, dir)
	mustWriteFile(t, filepath.Join(dir, "go.mod"), "module "+modulePath+"\n\ngo 1.24\n"+extra)
}

func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create directory %s: %v", dir, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
