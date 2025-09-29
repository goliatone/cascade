package planner

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

// ModuleInfo represents parsed information from a go.mod file
type ModuleInfo struct {
	Module   string
	File     *modfile.File
	FilePath string
}

// ParseGoMod parses a go.mod file and returns module information
func ParseGoMod(path string) (*ModuleInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read go.mod file: %w", err)
	}

	f, err := modfile.Parse(path, data, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to parse go.mod file: %w", err)
	}

	if f.Module == nil {
		return nil, fmt.Errorf("go.mod missing module directive")
	}

	return &ModuleInfo{
		Module:   f.Module.Mod.Path,
		File:     f,
		FilePath: path,
	}, nil
}

// ExtractDependency extracts the version of a specific dependency from parsed module info
func ExtractDependency(modInfo *ModuleInfo, modulePath string) (string, error) {
	if modInfo == nil || modInfo.File == nil {
		return "", fmt.Errorf("invalid module info")
	}

	// Check if there's a replace directive for this module
	for _, r := range modInfo.File.Replace {
		if r.Old.Path == modulePath {
			// If replaced with a local path, we cannot determine version
			if r.New.Version == "" {
				return "", fmt.Errorf("dependency %s has local replace directive", modulePath)
			}
			// Return the replaced version
			return r.New.Version, nil
		}
	}

	// Search in require directives
	for _, req := range modInfo.File.Require {
		if req.Mod.Path == modulePath {
			if req.Mod.Version == "" {
				return "", fmt.Errorf("dependency %s has no version", modulePath)
			}
			return req.Mod.Version, nil
		}
	}

	// Dependency not found
	return "", fmt.Errorf("dependency %s not found in go.mod", modulePath)
}

// findGoModFile locates the go.mod file in a repository path
func findGoModFile(repoPath string) (string, error) {
	goModPath := filepath.Join(repoPath, "go.mod")

	// Check if go.mod exists
	info, err := os.Stat(goModPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("go.mod not found in %s", repoPath)
		}
		return "", fmt.Errorf("failed to check go.mod: %w", err)
	}

	// Ensure it's a file, not a directory
	if info.IsDir() {
		return "", fmt.Errorf("go.mod is a directory, not a file")
	}

	return goModPath, nil
}
