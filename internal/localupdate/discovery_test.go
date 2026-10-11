package localupdate

import (
	"errors"
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

	repository, err := DiscoverRepository(startDir, DiscoveryOptions{})
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
	for _, excluded := range []string{"vendor", "node_modules", "testdata", "fixtures", ".cache", ".gomodcache", ".tmp"} {
		mustWriteModuleFile(t, filepath.Join(repositoryRoot, excluded, "ignored"), "github.com/goliatone/ignored/"+excluded, "")
	}

	repository, err := DiscoverRepository(repositoryRoot, DiscoveryOptions{})
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

func TestDiscoverRepositoryRespectsGitIgnoreWhenEnabled(t *testing.T) {
	repositoryRoot := filepath.Join(t.TempDir(), "app")
	mustMkdirAll(t, filepath.Join(repositoryRoot, ".git"))
	mustWriteModuleFile(t, repositoryRoot, "github.com/goliatone/app", "")
	mustWriteModuleFile(t, filepath.Join(repositoryRoot, "ignored", "module"), "github.com/goliatone/ignored", "")
	mustWriteModuleFile(t, filepath.Join(repositoryRoot, "custom", "dropped"), "github.com/goliatone/dropped", "")
	mustWriteModuleFile(t, filepath.Join(repositoryRoot, "custom", "kept"), "github.com/goliatone/kept", "")
	mustWriteModuleFile(t, filepath.Join(repositoryRoot, "custom", "kept", "nested-ignored"), "github.com/goliatone/nested-ignored", "")
	mustWriteFile(t, filepath.Join(repositoryRoot, ".gitignore"), "ignored/\ncustom/*\n!custom/kept/\n")
	mustWriteFile(t, filepath.Join(repositoryRoot, "custom", "kept", ".gitignore"), "nested-ignored/\n")

	withoutIgnore, err := DiscoverRepository(repositoryRoot, DiscoveryOptions{})
	if err != nil {
		t.Fatalf("discover repository without gitignore: %v", err)
	}
	if len(withoutIgnore.Modules) != 5 {
		t.Fatalf("expected every module without gitignore matching, got %#v", withoutIgnore.Modules)
	}

	withIgnore, err := DiscoverRepository(repositoryRoot, DiscoveryOptions{RespectGitIgnore: true})
	if err != nil {
		t.Fatalf("discover repository with gitignore: %v", err)
	}
	if len(withIgnore.Modules) != 2 {
		t.Fatalf("expected root and negated module, got %#v", withIgnore.Modules)
	}
	if withIgnore.Modules[0].ModulePath != "github.com/goliatone/app" || withIgnore.Modules[1].ModulePath != "github.com/goliatone/kept" {
		t.Fatalf("unexpected modules after gitignore matching: %#v", withIgnore.Modules)
	}
}

func TestDiscoverRepositoryPrunesIgnoredDirectoriesBeforeInspectingMetadata(t *testing.T) {
	for _, parent := range []string{".", "custom"} {
		t.Run(parent, func(t *testing.T) {
			root := t.TempDir()
			mustMkdirAll(t, filepath.Join(root, ".git"))
			mustWriteModuleFile(t, root, "github.com/goliatone/app", "")
			nested := filepath.Join(root, "adapter")
			mustWriteModuleFile(t, nested, "github.com/goliatone/app/adapter", "")
			blocked := filepath.Join(root, parent, "private-cache")
			mustMkdirAll(t, blocked)
			ignoreFile := filepath.Join(root, parent, ".gitignore")
			mustWriteFile(t, ignoreFile, "private-cache/\n")
			t.Cleanup(func() {
				if err := os.Chmod(blocked, 0o755); err != nil {
					t.Errorf("restore directory permissions: %v", err)
				}
			})
			if err := os.Chmod(blocked, 0); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Lstat(filepath.Join(blocked, ".git")); !errors.Is(err, os.ErrPermission) {
				if err == nil || errors.Is(err, os.ErrNotExist) {
					t.Skip("host does not enforce directory search permissions")
				}
				t.Fatalf("probe blocked directory: %v", err)
			}
			repository, err := DiscoverRepository(root, DiscoveryOptions{RespectGitIgnore: true})
			if err != nil {
				t.Fatalf("ignored directory must be pruned without inspecting it: %v", err)
			}
			if len(repository.Modules) != 2 || repository.Modules[0].ModuleDir != root || repository.Modules[1].ModuleDir != nested {
				t.Fatalf("expected root and accessible nested module, got %#v", repository.Modules)
			}
			if _, err := DiscoverRepository(root, DiscoveryOptions{}); !errors.Is(err, os.ErrPermission) {
				t.Fatalf("disabled ignore matching must retain permission errors: %v", err)
			}
			mustWriteFile(t, ignoreFile, "")
			if _, err := DiscoverRepository(root, DiscoveryOptions{RespectGitIgnore: true}); !errors.Is(err, os.ErrPermission) {
				t.Fatalf("nonignored directories must retain permission errors: %v", err)
			}
		})
	}
}

func TestDiscoverRepositoryStopsAtNestedGitCheckouts(t *testing.T) {
	for _, gitFile := range []bool{false, true} {
		name := "clone"
		if gitFile {
			name = "worktree"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			mustMkdirAll(t, filepath.Join(root, ".git"))
			mustWriteModuleFile(t, root, "github.com/goliatone/app", "")
			nested := filepath.Join(root, "adapter")
			mustWriteModuleFile(t, nested, "github.com/goliatone/app/adapter", "")
			checkout := filepath.Join(root, "checkouts", name)
			moduleDir := filepath.Join(checkout, "module")
			mustWriteModuleFile(t, moduleDir, "github.com/goliatone/app", "")
			if gitFile {
				mustWriteFile(t, filepath.Join(checkout, ".git"), "gitdir: /unavailable/metadata\n")
			} else {
				mustMkdirAll(t, filepath.Join(checkout, ".git"))
			}
			for _, respectIgnore := range []bool{false, true} {
				repository, err := DiscoverRepository(root, DiscoveryOptions{RespectGitIgnore: respectIgnore})
				if err != nil {
					t.Fatal(err)
				}
				if len(repository.Modules) != 2 || repository.Modules[0].ModuleDir != root || repository.Modules[1].ModuleDir != nested {
					t.Fatalf("expected only root and ordinary nested module, got %#v", repository.Modules)
				}
			}
			repository, err := DiscoverRepository(moduleDir, DiscoveryOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if repository.Root != checkout || len(repository.Modules) != 1 || repository.Modules[0].ModuleDir != moduleDir {
				t.Fatalf("direct invocation must target the nested checkout: %#v", repository)
			}
			mustWriteFile(t, filepath.Join(moduleDir, "go.mod"), "invalid go.mod\n")
			if _, err := DiscoverRepository(root, DiscoveryOptions{}); err != nil {
				t.Fatalf("must not parse modules inside a foreign checkout: %v", err)
			}
		})
	}
}

func TestDiscoverRepositoryKeepsGoWorkModulesAuthoritativeWithGitIgnore(t *testing.T) {
	repositoryRoot := filepath.Join(t.TempDir(), "app")
	mustMkdirAll(t, filepath.Join(repositoryRoot, ".git"))
	mustWriteModuleFile(t, repositoryRoot, "github.com/goliatone/app", "")
	mustWriteModuleFile(t, filepath.Join(repositoryRoot, "ignored"), "github.com/goliatone/ignored", "")
	mustWriteFile(t, filepath.Join(repositoryRoot, "ignored", ".git"), "gitdir: ../.git/worktrees/ignored\n")
	mustWriteFile(t, filepath.Join(repositoryRoot, ".gitignore"), "ignored/\n")
	mustWriteFile(t, filepath.Join(repositoryRoot, "go.work"), "go 1.24\nuse (\n.\n./ignored\n)\n")

	repository, err := DiscoverRepository(repositoryRoot, DiscoveryOptions{RespectGitIgnore: true})
	if err != nil {
		t.Fatalf("discover repository: %v", err)
	}
	if len(repository.Modules) != 2 || repository.Modules[1].ModulePath != "github.com/goliatone/ignored" {
		t.Fatalf("expected explicit go.work module to remain authoritative, got %#v", repository.Modules)
	}
}

func TestDiscoverRepositoryFallsBackToSingleEnclosingModuleWithoutGit(t *testing.T) {
	moduleDir := filepath.Join(t.TempDir(), "app")
	mustWriteModuleFile(t, moduleDir, "github.com/goliatone/app", "")
	startDir := filepath.Join(moduleDir, "pkg", "feature")
	mustMkdirAll(t, startDir)

	repository, err := DiscoverRepository(startDir, DiscoveryOptions{})
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

	if _, err := DiscoverRepository(repositoryRoot, DiscoveryOptions{}); err == nil {
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
