package manifest

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// WorkspaceDiscovery provides functionality to discover Go modules and their dependencies
// within a workspace directory.
type WorkspaceDiscovery interface {
	// DiscoverDependents finds Go modules in the workspace that depend on the target module.
	DiscoverDependents(ctx context.Context, options DiscoveryOptions) ([]DependentOptions, error)
}

// DiscoveryOptions configures workspace discovery behavior.
type DiscoveryOptions struct {
	// WorkspaceDir is the root directory to scan for Go modules
	WorkspaceDir string

	// TargetModule is the module path we're looking for dependents of
	TargetModule string

	// MaxDepth limits how deep to scan in the directory tree (0 = no limit)
	MaxDepth int

	// IncludePatterns specifies directory patterns to include (empty = include all)
	IncludePatterns []string

	// ExcludePatterns specifies directory patterns to exclude
	ExcludePatterns []string
}

// DiscoveredModule represents a Go module found during workspace scanning.
type DiscoveredModule struct {
	Path       string // File system path to the module
	ModulePath string // Go module path from go.mod
	Repository string // Inferred repository path
}

// NewWorkspaceDiscovery creates a new workspace discovery instance.
func NewWorkspaceDiscovery() WorkspaceDiscovery {
	return &workspaceDiscovery{}
}

type workspaceDiscovery struct{}

// DiscoverDependents scans the workspace for Go modules that depend on the target module.
func (w *workspaceDiscovery) DiscoverDependents(ctx context.Context, options DiscoveryOptions) ([]DependentOptions, error) {
	if options.WorkspaceDir == "" {
		return nil, fmt.Errorf("workspace directory is required")
	}
	if options.TargetModule == "" {
		return nil, fmt.Errorf("target module is required")
	}

	// Find all Go modules in the workspace
	modules, err := w.findGoModules(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("failed to find Go modules: %w", err)
	}

	var dependents []DependentOptions

	// Check each module for dependencies on the target
	for _, module := range modules {
		depends, err := w.moduleHasDependency(ctx, module.Path, options.TargetModule)
		if err != nil {
			// Log warning but continue with other modules
			continue
		}

		if depends {
			dependent := DependentOptions{
				Repository:      w.inferRepository(module.ModulePath),
				ModulePath:      module.ModulePath,
				LocalModulePath: ".", // Default to root of repository
			}
			dependents = append(dependents, dependent)
		}
	}

	return dependents, nil
}

// findGoModules discovers all Go modules within the workspace directory.
func (w *workspaceDiscovery) findGoModules(ctx context.Context, options DiscoveryOptions) ([]DiscoveredModule, error) {
	var modules []DiscoveredModule

	err := filepath.Walk(options.WorkspaceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Check depth limit
		if options.MaxDepth > 0 {
			rel, err := filepath.Rel(options.WorkspaceDir, path)
			if err != nil {
				return err
			}
			depth := strings.Count(rel, string(filepath.Separator))
			if depth > options.MaxDepth {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		// Skip if not a go.mod file
		if !info.IsDir() && info.Name() == "go.mod" {
			// Check include/exclude patterns
			if w.shouldIncludeDirectory(filepath.Dir(path), options) {
				modulePath, err := w.extractModulePath(path)
				if err != nil {
					return nil // Skip modules with invalid go.mod files
				}

				module := DiscoveredModule{
					Path:       filepath.Dir(path),
					ModulePath: modulePath,
					Repository: w.inferRepository(modulePath),
				}
				modules = append(modules, module)
			}
		}

		return nil
	})

	return modules, err
}

// moduleHasDependency checks if a Go module depends on the target module.
func (w *workspaceDiscovery) moduleHasDependency(ctx context.Context, modulePath, targetModule string) (bool, error) {
	// First try using go list to get module dependencies
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "all")
	cmd.Dir = modulePath

	output, err := cmd.Output()
	if err != nil {
		// If go list fails, fall back to parsing go.mod directly
		return w.parseGoModForDependency(modulePath, targetModule)
	}

	// Scan output for the target module
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, targetModule+" ") || line == targetModule {
			return true, nil
		}
	}

	return false, scanner.Err()
}

// parseGoModForDependency parses the go.mod file directly to check for a dependency.
func (w *workspaceDiscovery) parseGoModForDependency(modulePath, targetModule string) (bool, error) {
	goModPath := filepath.Join(modulePath, "go.mod")
	file, err := os.Open(goModPath)
	if err != nil {
		return false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	inRequireBlock := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Handle single-line require statements
		if strings.HasPrefix(line, "require ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 && parts[1] == targetModule {
				return true, nil
			}
			if strings.HasSuffix(line, "(") {
				inRequireBlock = true
				continue
			}
		}

		// Handle multi-line require blocks
		if inRequireBlock {
			if strings.Contains(line, ")") {
				inRequireBlock = false
			}
			parts := strings.Fields(line)
			if len(parts) >= 1 && parts[0] == targetModule {
				return true, nil
			}
		}
	}

	return false, scanner.Err()
}

// extractModulePath reads the module path from a go.mod file.
func (w *workspaceDiscovery) extractModulePath(goModPath string) (string, error) {
	file, err := os.Open(goModPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1], nil
			}
		}
	}

	return "", fmt.Errorf("no module declaration found in %s", goModPath)
}

// inferRepository attempts to infer the repository path from a Go module path.
func (w *workspaceDiscovery) inferRepository(modulePath string) string {
	// For common hosting providers, extract the repository part
	parts := strings.Split(modulePath, "/")

	if len(parts) >= 3 {
		switch parts[0] {
		case "github.com", "gitlab.com", "bitbucket.org":
			return strings.Join(parts[1:3], "/")
		}
	}

	// Fallback: use the full module path as repository
	return modulePath
}

// shouldIncludeDirectory checks if a directory should be included based on patterns.
func (w *workspaceDiscovery) shouldIncludeDirectory(dirPath string, options DiscoveryOptions) bool {
	relPath, err := filepath.Rel(options.WorkspaceDir, dirPath)
	if err != nil {
		return false
	}

	// Check exclude patterns first
	for _, pattern := range options.ExcludePatterns {
		if matched, _ := filepath.Match(pattern, relPath); matched {
			return false
		}
		// Also check if any parent directory matches
		parts := strings.Split(relPath, string(filepath.Separator))
		for i := 1; i <= len(parts); i++ {
			parentPath := strings.Join(parts[:i], string(filepath.Separator))
			if matched, _ := filepath.Match(pattern, parentPath); matched {
				return false
			}
		}
	}

	// If no include patterns specified, include by default
	if len(options.IncludePatterns) == 0 {
		return true
	}

	// Check include patterns
	for _, pattern := range options.IncludePatterns {
		if matched, _ := filepath.Match(pattern, relPath); matched {
			return true
		}
		// Also check if any parent directory matches
		parts := strings.Split(relPath, string(filepath.Separator))
		for i := 1; i <= len(parts); i++ {
			parentPath := strings.Join(parts[:i], string(filepath.Separator))
			if matched, _ := filepath.Match(pattern, parentPath); matched {
				return true
			}
		}
	}

	return false
}
