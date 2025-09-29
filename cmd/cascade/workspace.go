package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/goliatone/cascade/pkg/config"
)

// isValidWorkspace checks if a directory exists and is readable
func isValidWorkspace(dir string) bool {
	if dir == "" {
		return false
	}

	info, err := os.Stat(dir)
	if err != nil {
		return false
	}

	return info.IsDir()
}

// resolveWorkspaceDir determines the workspace directory for discovery with intelligent defaults
func resolveWorkspaceDir(workspace string, cfg *config.Config) string {
	return resolveWorkspaceDirWithTarget(workspace, cfg, "", "")
}

// resolveWorkspaceDirWithTarget determines the workspace directory for discovery with intelligent defaults
// using the provided target module information for smarter detection
func resolveWorkspaceDirWithTarget(workspace string, cfg *config.Config, targetModulePath, targetModuleDir string) string {
	// Debug output removed - function working correctly
	// Use explicit workspace if provided
	if workspace != "" {
		if !filepath.IsAbs(workspace) {
			if abs, err := filepath.Abs(workspace); err == nil {
				return abs
			}
		}
		return workspace
	}

	// workspace detection based on target module location if available, otherwise current module location
	if intelligentWorkspace := detectDefaultWorkspaceWithTarget(targetModulePath, targetModuleDir); intelligentWorkspace != "" {
		// TODO: Add logging here when logger is available
		return intelligentWorkspace
	}

	// Use config workspace path
	if cfg != nil && cfg.Workspace.Path != "" {
		return cfg.Workspace.Path
	}

	// Use manifest generator default workspace
	if cfg != nil && cfg.ManifestGenerator.DefaultWorkspace != "" {
		return cfg.ManifestGenerator.DefaultWorkspace
	}

	// Fallback to $HOME/.cache/cascade for isolation
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".cache", "cascade")
	}

	// Last resort: current working directory
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}

	return "."
}

// detectDefaultWorkspace attempts to detect a sensible workspace directory based on
// the current module's location and Go environment. It tries to find a directory that
// likely contains other Go modules that might depend on the current module.
func detectDefaultWorkspace() string {
	return detectDefaultWorkspaceWithTarget("", "")
}

// detectDefaultWorkspaceWithTarget attempts to detect a sensible workspace directory based on
// the target module's location and Go environment. If target module info is not provided,
// it falls back to the current module's location.
func detectDefaultWorkspaceWithTarget(targetModulePath, targetModuleDir string) string {
	// Use target module information if provided, otherwise detect current module
	modulePath := targetModulePath
	moduleDir := targetModuleDir

	if modulePath == "" || moduleDir == "" {
		// Get current module information as fallback
		detectedPath, detectedDir, err := detectModuleInfo()
		if err != nil {
			return ""
		}
		modulePath = detectedPath
		moduleDir = detectedDir
	}

	// 1) use parent directory of module if it contains multiple Go modules
	// e.g., ~/Development/GO/src/github.com/goliatone/go-errors -> ~/Development/GO/src/github.com/goliatone/
	if parentWorkspace := detectParentWorkspace(moduleDir, modulePath); parentWorkspace != "" {
		return parentWorkspace
	}

	// 2) check GOPATH/src/{hosting}/{org}/ directory
	// e.g., github.com/goliatone/go-errors -> $GOPATH/src/github.com/goliatone/
	if gopathOrgWorkspace := detectGopathOrgWorkspace(modulePath); gopathOrgWorkspace != "" {
		return gopathOrgWorkspace
	}

	// 3) check GOPATH/src/ directory for broader discovery
	if gopathWorkspace := detectGopathWorkspace(); gopathWorkspace != "" {
		return gopathWorkspace
	}

	return ""
}

// deriveModuleDirFromPath attempts to derive the local directory path for a given module path
// by checking GOPATH conventions and common workspace layouts
func deriveModuleDirFromPath(modulePath string) string {
	if modulePath == "" {
		return ""
	}

	// Try GOPATH/src/{module-path} structure
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		// Try default GOPATH
		if home, err := os.UserHomeDir(); err == nil {
			gopath = filepath.Join(home, "go")
		}
	}
	if gopath != "" {
		moduleDir := filepath.Join(gopath, "src", modulePath)
		if isValidWorkspace(moduleDir) {
			if goModPath := filepath.Join(moduleDir, "go.mod"); isValidModuleDir(goModPath) {
				return moduleDir
			}
		}
	}

	// Try common development patterns: ~/Development/GO/src/{module-path}
	if home, err := os.UserHomeDir(); err == nil {
		commonPaths := []string{
			filepath.Join(home, "Development", "GO", "src", modulePath),
			filepath.Join(home, "dev", "go", "src", modulePath),
			filepath.Join(home, "src", modulePath),
		}

		for _, moduleDir := range commonPaths {
			if isValidWorkspace(moduleDir) {
				if goModPath := filepath.Join(moduleDir, "go.mod"); isValidModuleDir(goModPath) {
					return moduleDir
				}
			}
		}
	}

	return ""
}

// isValidModuleDir checks if a directory contains a go.mod file
func isValidModuleDir(goModPath string) bool {
	info, err := os.Stat(goModPath)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// detectParentWorkspace checks if the parent directories of the current module
// contain other Go modules, indicating this is a multi-module workspace
func detectParentWorkspace(moduleDir, modulePath string) string {
	if moduleDir == "" {
		return ""
	}

	// Extract organization from module path (e.g., "goliatone" from "github.com/goliatone/go-errors")
	org := extractOrgFromModulePath(modulePath)
	if org == "" {
		return ""
	}

	// Walk up the directory tree looking for a directory that contains multiple Go modules
	current := moduleDir
	var potentialWorkspace string // To store a candidate workspace

	for i := 0; i < 5; i++ { // Limit traversal to avoid going too far up
		parent := filepath.Dir(current)
		if parent == current || parent == "/" || parent == "." {
			break
		}

		// A valid workspace MUST contain multiple go modules.
		if containsMultipleModules(parent) {
			// If the parent is named after the org, we are confident this is the workspace.
			if filepath.Base(parent) == org {
				return parent // This is our ideal workspace, return immediately.
			}
			// Otherwise, keep it as a potential candidate and keep searching for a better one.
			if potentialWorkspace == "" {
				potentialWorkspace = parent
			}
		}
		current = parent
	}

	return potentialWorkspace
}

// extractOrgFromModulePath extracts the organization from a module path
func extractOrgFromModulePath(modulePath string) string {
	parts := strings.Split(modulePath, "/")
	if len(parts) >= 2 {
		switch parts[0] {
		case "github.com", "gitlab.com", "bitbucket.org":
			return parts[1]
		}
	}
	return ""
}

// containsMultipleModules checks if a directory contains multiple Go modules
func containsMultipleModules(dir string) bool {
	moduleCount := 0
	maxCheck := 50 // Limit to avoid scanning huge directories

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue on errors
		}

		// Stop if we've checked too many entries
		if moduleCount >= maxCheck {
			return filepath.SkipDir
		}

		// Skip deep nested directories
		if strings.Count(strings.TrimPrefix(path, dir), string(filepath.Separator)) > 3 {
			return filepath.SkipDir
		}

		// Skip common non-module directories
		base := filepath.Base(path)
		if base == ".git" || base == "vendor" || base == "node_modules" || base == ".cache" {
			return filepath.SkipDir
		}

		if info.Name() == "go.mod" {
			moduleCount++
			if moduleCount >= 2 {
				return filepath.SkipAll // Found multiple modules, we can stop
			}
		}

		return nil
	})

	if err != nil {
		return false
	}

	return moduleCount >= 2
}

// detectGopathOrgWorkspace checks $GOPATH/src/{hosting}/{org}/ for a workspace
func detectGopathOrgWorkspace(modulePath string) string {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		// Try default GOPATH
		if home, err := os.UserHomeDir(); err == nil {
			gopath = filepath.Join(home, "go")
		}
	}
	if gopath == "" {
		return ""
	}

	// Parse module path to extract hosting and org
	// e.g., github.com/goliatone/go-errors -> hosting=github.com, org=goliatone
	parts := strings.Split(modulePath, "/")
	if len(parts) < 3 {
		return ""
	}

	hosting := parts[0]
	org := parts[1]

	// Check $GOPATH/src/{hosting}/{org}/
	orgPath := filepath.Join(gopath, "src", hosting, org)
	if isValidWorkspace(orgPath) {
		return orgPath
	}

	return ""
}

// detectGopathWorkspace checks $GOPATH/src/ as a broader workspace
func detectGopathWorkspace() string {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		// Try default GOPATH
		if home, err := os.UserHomeDir(); err == nil {
			gopath = filepath.Join(home, "go")
		}
	}
	if gopath == "" {
		return ""
	}

	srcPath := filepath.Join(gopath, "src")
	if isValidWorkspace(srcPath) && containsMultipleModules(srcPath) {
		return srcPath
	}

	return ""
}
