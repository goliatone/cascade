package localupdate

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"golang.org/x/mod/modfile"
)

var repositoryScanExcludedDirs = map[string]bool{
	".cache":       true,
	".git":         true,
	".gocache":     true,
	".gomodcache":  true,
	".gopath":      true,
	".tmp":         true,
	"fixtures":     true,
	"node_modules": true,
	"testdata":     true,
	"vendor":       true,
}

type DiscoveryOptions struct {
	RespectGitIgnore bool
}

func DiscoverRepository(startDir string, options DiscoveryOptions) (Repository, error) {
	startDir = strings.TrimSpace(startDir)
	if startDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return Repository{}, fmt.Errorf("determine current directory: %w", err)
		}
		startDir = cwd
	}

	startDir, err := canonicalExistingDir(startDir)
	if err != nil {
		return Repository{}, fmt.Errorf("resolve start directory: %w", err)
	}
	repositoryRoot, err := findRepositoryRoot(startDir)
	if err != nil {
		return Repository{}, err
	}

	repository := Repository{Root: repositoryRoot}
	if workFile := repositoryWorkFile(repositoryRoot); workFile != "" {
		repository.WorkFile = workFile
		repository.Modules, repository.ExternalUses, err = modulesFromWorkFile(repositoryRoot, workFile)
	} else {
		repository.Modules, err = modulesFromRepositoryTree(repositoryRoot, options)
	}
	if err != nil {
		return Repository{}, err
	}
	if len(repository.Modules) == 0 {
		return Repository{}, fmt.Errorf("no Go modules found in repository %s", repositoryRoot)
	}
	sortModuleTargets(repositoryRoot, repository.Modules)
	return repository, nil
}

func repositoryWorkFile(repositoryRoot string) string {
	path := filepath.Join(repositoryRoot, "go.work")
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return path
	}
	return ""
}

func findRepositoryRoot(startDir string) (string, error) {
	for dir := startDir; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return canonicalExistingDir(dir)
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect repository metadata at %s: %w", dir, err)
		}
		if parent := filepath.Dir(dir); parent == dir {
			break
		}
	}

	if workFile := findUpwardFile(startDir, "", "go.work"); workFile != "" {
		return canonicalExistingDir(filepath.Dir(workFile))
	}
	if goMod := findUpwardFile(startDir, "", "go.mod"); goMod != "" {
		return canonicalExistingDir(filepath.Dir(goMod))
	}
	return "", fmt.Errorf("repository does not contain an enclosing .git, go.work, or go.mod")
}

func findUpwardFile(startDir, stopDir, name string) string {
	for dir := startDir; ; dir = filepath.Dir(dir) {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
		if stopDir != "" && samePath(dir, stopDir) {
			break
		}
		if parent := filepath.Dir(dir); parent == dir {
			break
		}
	}
	return ""
}

func modulesFromWorkFile(repositoryRoot, workFilePath string) ([]ModuleTarget, []string, error) {
	data, err := os.ReadFile(workFilePath)
	if err != nil {
		return nil, nil, fmt.Errorf("read go.work: %w", err)
	}
	workFile, err := modfile.ParseWork(workFilePath, data, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("parse go.work: %w", err)
	}

	seen := map[string]bool{}
	modules := make([]ModuleTarget, 0, len(workFile.Use))
	external := make([]string, 0)
	for _, use := range workFile.Use {
		moduleDir := use.Path
		if !filepath.IsAbs(moduleDir) {
			moduleDir = filepath.Join(filepath.Dir(workFilePath), moduleDir)
		}
		moduleDir, err = filepath.Abs(moduleDir)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve go.work use %q: %w", use.Path, err)
		}
		moduleDir = filepath.Clean(moduleDir)
		if !pathWithin(repositoryRoot, moduleDir) {
			external = append(external, moduleDir)
			continue
		}
		moduleDir, err = canonicalExistingDir(moduleDir)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve go.work use %q: %w", use.Path, err)
		}
		moduleKey := physicalPath(moduleDir)
		if seen[moduleKey] {
			continue
		}
		target, err := readModuleTarget(moduleDir)
		if err != nil {
			return nil, nil, fmt.Errorf("validate go.work use %q: %w", use.Path, err)
		}
		seen[moduleKey] = true
		modules = append(modules, target)
	}
	sort.Strings(external)
	return modules, external, nil
}

func modulesFromRepositoryTree(repositoryRoot string, options DiscoveryOptions) ([]ModuleTarget, error) {
	modules := make([]ModuleTarget, 0, 1)
	ignorePatterns := make([]gitignore.Pattern, 0)
	ignoreMatcher := gitignore.NewMatcher(ignorePatterns)
	err := filepath.WalkDir(repositoryRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relativePath, err := filepath.Rel(repositoryRoot, path)
		if err != nil {
			return fmt.Errorf("resolve repository-relative path for %s: %w", path, err)
		}
		pathParts := repositoryPathParts(relativePath)
		if entry.IsDir() {
			if path != repositoryRoot && repositoryScanExcludedDirs[entry.Name()] {
				return filepath.SkipDir
			}
			if path != repositoryRoot && options.RespectGitIgnore && ignoreMatcher.Match(pathParts, true) {
				return filepath.SkipDir
			}
			if options.RespectGitIgnore {
				patterns, err := readGitIgnorePatterns(path, pathParts)
				if err != nil {
					return err
				}
				if len(patterns) > 0 {
					ignorePatterns = append(ignorePatterns, patterns...)
					ignoreMatcher = gitignore.NewMatcher(ignorePatterns)
				}
			}
			return nil
		}
		if options.RespectGitIgnore && ignoreMatcher.Match(pathParts, false) {
			return nil
		}
		if entry.Name() != "go.mod" {
			return nil
		}
		target, err := readModuleTarget(filepath.Dir(path))
		if err != nil {
			return err
		}
		modules = append(modules, target)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover repository Go modules: %w", err)
	}
	return modules, nil
}

func readGitIgnorePatterns(directory string, domain []string) ([]gitignore.Pattern, error) {
	ignorePath := filepath.Join(directory, ".gitignore")
	file, err := os.Open(ignorePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", ignorePath, err)
	}
	defer file.Close()

	patterns := make([]gitignore.Pattern, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, gitignore.ParsePattern(line, domain))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", ignorePath, err)
	}
	return patterns, nil
}

func repositoryPathParts(relativePath string) []string {
	if relativePath == "." || relativePath == "" {
		return nil
	}
	return strings.Split(filepath.ToSlash(relativePath), "/")
}

func readModuleTarget(moduleDir string) (ModuleTarget, error) {
	goModPath := filepath.Join(moduleDir, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return ModuleTarget{}, fmt.Errorf("read %s: %w", goModPath, err)
	}
	file, err := modfile.Parse(goModPath, data, nil)
	if err != nil {
		return ModuleTarget{}, fmt.Errorf("parse %s: %w", goModPath, err)
	}
	if file.Module == nil || strings.TrimSpace(file.Module.Mod.Path) == "" {
		return ModuleTarget{}, fmt.Errorf("module declaration not found in %s", goModPath)
	}
	return ModuleTarget{ModulePath: file.Module.Mod.Path, ModuleDir: moduleDir}, nil
}

func canonicalExistingDir(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	cleaned := filepath.Clean(abs)
	info, err := os.Stat(cleaned)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", cleaned)
	}
	return cleaned, nil
}

func pathWithin(root, candidate string) bool {
	root = physicalPath(root)
	candidate = physicalPath(candidate)
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func physicalPath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(path)
}

func sortModuleTargets(repositoryRoot string, modules []ModuleTarget) {
	sort.Slice(modules, func(i, j int) bool {
		iRel, _ := filepath.Rel(repositoryRoot, modules[i].ModuleDir)
		jRel, _ := filepath.Rel(repositoryRoot, modules[j].ModuleDir)
		if iRel == "." {
			return true
		}
		if jRel == "." {
			return false
		}
		return filepath.ToSlash(iRel) < filepath.ToSlash(jRel)
	})
}
