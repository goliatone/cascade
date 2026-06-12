package localupdate

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/mod/modfile"
)

type localModule struct {
	ModulePath string
	ModuleDir  string
	RepoRoot   string
	Checked    []string
}

type localModuleError struct {
	Status Status
	Reason string
}

func (e localModuleError) Error() string {
	return e.Reason
}

type moduleCandidate struct {
	dir           string
	repoRoot      string
	authoritative bool
}

func resolveLocalModule(workspacePath, modulePath string) (localModule, error) {
	workspacePath = filepath.Clean(workspacePath)
	candidates := localModuleCandidates(workspacePath, modulePath)
	checked := make([]string, 0, len(candidates))
	var authoritativeErr *localModuleError

	for _, candidate := range candidates {
		checked = append(checked, candidate.dir)
		foundPath, exists, err := readCandidateModulePath(candidate.dir)
		if err != nil {
			if candidate.authoritative {
				if authoritativeErr == nil {
					err := localModuleError{
						Status: StatusInvalidLocalModule,
						Reason: fmt.Sprintf("invalid local module candidate at %s: %v", candidate.dir, err),
					}
					authoritativeErr = &err
				}
				continue
			}
			continue
		}
		if !exists {
			continue
		}
		if foundPath != modulePath {
			if candidate.authoritative {
				if authoritativeErr == nil {
					err := localModuleError{
						Status: StatusInvalidLocalModule,
						Reason: fmt.Sprintf("local module candidate at %s declares %s, expected %s", candidate.dir, foundPath, modulePath),
					}
					authoritativeErr = &err
				}
				continue
			}
			continue
		}
		return localModule{
			ModulePath: modulePath,
			ModuleDir:  candidate.dir,
			RepoRoot:   candidate.repoRoot,
			Checked:    append([]string(nil), checked...),
		}, nil
	}

	if authoritativeErr != nil {
		return localModule{}, *authoritativeErr
	}

	matches, scannedChecked, err := scanForLocalModule(workspacePath, modulePath)
	checked = appendChecked(checked, scannedChecked...)
	if err != nil {
		return localModule{}, err
	}

	if len(matches) == 1 {
		matches[0].Checked = checked
		return matches[0], nil
	}
	if len(matches) > 1 {
		return localModule{}, localModuleError{
			Status: StatusAmbiguousLocalModule,
			Reason: fmt.Sprintf("multiple local modules match %s: %s", modulePath, joinModuleDirs(matches)),
		}
	}

	return localModule{}, localModuleError{
		Status: StatusMissingLocalRepo,
		Reason: fmt.Sprintf("local module not found; checked %s", strings.Join(checked, ", ")),
	}
}

func localModuleCandidates(workspacePath, modulePath string) []moduleCandidate {
	parts := splitModulePath(modulePath)
	candidates := make([]moduleCandidate, 0, 6)
	seen := map[string]bool{}
	workspaceBase := filepath.Base(workspacePath)

	add := func(dir, repoRoot string, authoritative bool) {
		if strings.TrimSpace(dir) == "" || seen[dir] {
			return
		}
		seen[dir] = true
		candidates = append(candidates, moduleCandidate{dir: dir, repoRoot: repoRoot, authoritative: authoritative})
	}
	addRelative := func(relative, rootParts []string, authoritative bool) {
		repoRoot := workspacePath
		if len(rootParts) > 0 {
			repoRoot = filepath.Join(append([]string{workspacePath}, rootParts...)...)
		}
		addMajorAware(add, workspacePath, relative, repoRoot, authoritative)
	}

	if len(parts) >= 3 && isKnownVCSHost(parts[0]) {
		repoRel := parts[2:]
		orgRel := parts[1:]
		fullRel := parts
		subRel := parts[3:]
		switch workspaceBase {
		case parts[2]:
			addRelative(subRel, nil, true)
			addRelative(repoRel, []string{parts[2]}, false)
			addRelative(orgRel, []string{parts[1], parts[2]}, false)
		case parts[1]:
			addRelative(repoRel, []string{parts[2]}, true)
			addRelative(orgRel, []string{parts[1], parts[2]}, false)
		case parts[0]:
			addRelative(orgRel, []string{parts[1], parts[2]}, true)
			addRelative(repoRel, []string{parts[2]}, false)
		default:
			addRelative(fullRel, []string{parts[0], parts[1], parts[2]}, true)
			addRelative(orgRel, []string{parts[1], parts[2]}, false)
			addRelative(repoRel, []string{parts[2]}, false)
		}
	} else if len(parts) >= 2 && strings.Contains(parts[0], ".") {
		repoRel := parts[1:]
		fullRel := parts
		subRel := parts[2:]
		switch workspaceBase {
		case parts[1]:
			addRelative(subRel, nil, true)
			addRelative(repoRel, []string{parts[1]}, false)
		case parts[0]:
			addRelative(repoRel, []string{parts[1]}, true)
		default:
			addRelative(fullRel, []string{parts[0], parts[1]}, true)
			addRelative(repoRel, []string{parts[1]}, false)
		}
	}

	if len(parts) > 0 {
		base := parts[len(parts)-1]
		if !isMajorVersionSuffix(base) {
			add(filepath.Join(workspacePath, base), filepath.Join(workspacePath, base), false)
		}
	}

	return candidates
}

func addMajorAware(add func(dir, repoRoot string, authoritative bool), workspacePath string, relative []string, repoRoot string, authoritative bool) {
	if len(relative) == 0 {
		add(workspacePath, repoRoot, authoritative)
		return
	}
	if trimmed := trimMajorSuffix(relative); len(trimmed) != len(relative) {
		add(filepath.Join(append([]string{workspacePath}, trimmed...)...), repoRoot, authoritative)
		add(filepath.Join(append([]string{workspacePath}, relative...)...), repoRoot, authoritative)
		return
	}
	add(filepath.Join(append([]string{workspacePath}, relative...)...), repoRoot, authoritative)
}

func isKnownVCSHost(host string) bool {
	switch host {
	case "github.com", "gitlab.com", "bitbucket.org":
		return true
	default:
		return false
	}
}

func scanForLocalModule(workspacePath, modulePath string) ([]localModule, []string, error) {
	roots := scanRoots(workspacePath, modulePath)
	checked := make([]string, 0, len(roots))
	matches := make([]localModule, 0, 1)
	for _, root := range roots {
		checked = append(checked, root)
		found, err := scanRootForModule(workspacePath, root, modulePath)
		if err != nil {
			return nil, checked, err
		}
		matches = append(matches, found...)
	}
	return matches, checked, nil
}

func scanRoots(workspacePath, modulePath string) []string {
	roots := []string{}
	seen := map[string]bool{}
	add := func(root string) {
		if strings.TrimSpace(root) == "" || seen[root] {
			return
		}
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			seen[root] = true
			roots = append(roots, root)
		}
	}

	for _, candidate := range localModuleCandidates(workspacePath, modulePath) {
		if candidate.authoritative {
			add(candidate.repoRoot)
		}
	}
	add(workspacePath)
	return roots
}

func scanRootForModule(workspacePath, root, modulePath string) ([]localModule, error) {
	const maxDepth = 5
	matches := []localModule{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "vendor", "node_modules", ".cache":
				return filepath.SkipDir
			}
			if depth(root, path) > maxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() != "go.mod" {
			return nil
		}
		moduleDir := filepath.Dir(path)
		foundPath, exists, err := readCandidateModulePath(moduleDir)
		if err != nil {
			return nil
		}
		if !exists || foundPath != modulePath {
			return nil
		}
		matches = append(matches, localModule{
			ModulePath: modulePath,
			ModuleDir:  moduleDir,
			RepoRoot:   repoRootForModule(workspacePath, root, moduleDir, modulePath),
		})
		return nil
	})
	return matches, err
}

func readCandidateModulePath(dir string) (string, bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", true, err
	}
	file, err := modfile.Parse(filepath.Join(dir, "go.mod"), data, nil)
	if err != nil {
		return "", true, err
	}
	if file.Module == nil || strings.TrimSpace(file.Module.Mod.Path) == "" {
		return "", true, fmt.Errorf("module declaration not found")
	}
	return file.Module.Mod.Path, true, nil
}

func repoRootForModule(workspacePath, scanRoot, moduleDir, modulePath string) string {
	parts := splitModulePath(modulePath)
	repoName := ""
	if len(parts) >= 3 && isKnownVCSHost(parts[0]) {
		repoName = parts[2]
	} else if len(parts) >= 2 && strings.Contains(parts[0], ".") {
		repoName = parts[1]
	}
	if repoName != "" && filepath.Base(workspacePath) == repoName {
		return workspacePath
	}
	if scanRoot != workspacePath {
		return scanRoot
	}
	rel, err := filepath.Rel(workspacePath, moduleDir)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return moduleDir
	}
	first := strings.Split(rel, string(filepath.Separator))[0]
	if repoName != "" && first == repoName {
		return filepath.Join(workspacePath, first)
	}
	return moduleDir
}

func appendChecked(base []string, values ...string) []string {
	seen := map[string]bool{}
	for _, value := range base {
		seen[value] = true
	}
	for _, value := range values {
		if !seen[value] {
			base = append(base, value)
			seen[value] = true
		}
	}
	return base
}

func splitModulePath(modulePath string) []string {
	parts := strings.Split(strings.Trim(modulePath, "/"), "/")
	out := parts[:0]
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func trimMajorSuffix(parts []string) []string {
	if len(parts) == 0 || !isMajorVersionSuffix(parts[len(parts)-1]) {
		return parts
	}
	return parts[:len(parts)-1]
}

func isMajorVersionSuffix(value string) bool {
	if len(value) < 2 || value[0] != 'v' {
		return false
	}
	if len(value) > 2 && value[1] == '0' {
		return false
	}
	for _, r := range value[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	major, err := strconv.Atoi(value[1:])
	return err == nil && major >= 2
}

func depth(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return strings.Count(rel, string(filepath.Separator)) + 1
}

func joinModuleDirs(modules []localModule) string {
	dirs := make([]string, 0, len(modules))
	for _, module := range modules {
		dirs = append(dirs, module.ModuleDir)
	}
	return strings.Join(dirs, ", ")
}
