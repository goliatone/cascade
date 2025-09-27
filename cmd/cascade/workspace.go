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
	// Use explicit workspace if provided
	if workspace != "" {
		if !filepath.IsAbs(workspace) {
			if abs, err := filepath.Abs(workspace); err == nil {
				return abs
			}
		}
		return workspace
	}

	// Use config workspace path
	if cfg != nil && cfg.Workspace.Path != "" {
		return cfg.Workspace.Path
	}

	// Use manifest generator default workspace
	if cfg != nil && cfg.ManifestGenerator.DefaultWorkspace != "" {
		return cfg.ManifestGenerator.DefaultWorkspace
	}

	// workspace detection based on current module location
	if intelligentWorkspace := detectDefaultWorkspace(); intelligentWorkspace != "" {
		// TODO: Add logging here when logger is available
		return intelligentWorkspace
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
	// Get current module information
	modulePath, moduleDir, err := detectModuleInfo()
	if err != nil {
		return ""
	}

	// 1) use parent directory of current module if it contains multiple Go modules
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
	for i := 0; i < 5; i++ { // Limit traversal to avoid going too far up
		parent := filepath.Dir(current)
		if parent == current || parent == "/" || parent == "." {
			break
		}

		// Check if this directory name matches the organization
		if filepath.Base(parent) == org {
			// Validate this directory contains multiple Go modules
			if isValidWorkspace(parent) {
				return parent
			}
		}

		// Also check if parent contains multiple modules (even if not named after org)
		if isValidWorkspace(parent) && containsMultipleModules(parent) {
			return parent
		}

		current = parent
	}

	return ""
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
