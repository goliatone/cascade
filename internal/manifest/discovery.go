package manifest

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/mod/semver"
)

// WorkspaceDiscovery provides functionality to discover Go modules and their dependencies
// within a workspace directory.
type WorkspaceDiscovery interface {
	// DiscoverDependents finds Go modules in the workspace that depend on the target module.
	DiscoverDependents(ctx context.Context, options DiscoveryOptions) ([]DependentOptions, error)

	// ResolveVersion attempts to resolve the current version of a module within the workspace.
	// It returns the resolved version or an error if resolution fails.
	ResolveVersion(ctx context.Context, options VersionResolutionOptions) (*VersionResolution, error)
}

// DiscoveryOptions configures workspace discovery behavior.
type DiscoveryOptions struct {
	// WorkspaceDir is the root directory to scan for Go modules
	WorkspaceDir string

	// TargetModule is the module path we're looking for dependents of
	TargetModule string

	// TargetVersion is the version we're updating to (optional - if set, filters out modules already at this version)
	TargetVersion string

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

// VersionResolutionOptions configures version resolution behavior.
type VersionResolutionOptions struct {
	// WorkspaceDir is the root directory to scan for Go modules
	WorkspaceDir string

	// TargetModule is the module path we're trying to resolve the version for
	TargetModule string

	// Strategy determines how to resolve the version
	Strategy VersionResolutionStrategy

	// AllowNetworkAccess enables network-based resolution methods
	AllowNetworkAccess bool
}

// VersionResolutionStrategy defines how to resolve module versions.
type VersionResolutionStrategy string

const (
	// VersionResolutionLocal resolves version from local workspace dependencies
	VersionResolutionLocal VersionResolutionStrategy = "local"

	// VersionResolutionLatest resolves to the latest available version
	VersionResolutionLatest VersionResolutionStrategy = "latest"

	// VersionResolutionAuto tries local first, then latest if network access is allowed
	VersionResolutionAuto VersionResolutionStrategy = "auto"
)

// VersionResolution contains the result of version resolution.
type VersionResolution struct {
	// Version is the resolved version
	Version string

	// Source indicates how the version was resolved
	Source VersionResolutionSource

	// SourcePath is the path where the version was found (for local resolutions)
	SourcePath string

	// Warnings contains any warnings generated during resolution
	Warnings []string
}

// VersionResolutionSource indicates where a version was resolved from.
type VersionResolutionSource string

const (
	// VersionSourceLocal indicates the version was found in local workspace
	VersionSourceLocal VersionResolutionSource = "local"

	// VersionSourceNetwork indicates the version was retrieved from network
	VersionSourceNetwork VersionResolutionSource = "network"

	// VersionSourceFallback indicates a fallback/default version was used
	VersionSourceFallback VersionResolutionSource = "fallback"
)

// moduleInfo represents go list -m -json output
type moduleInfo struct {
	Path     string      `json:"Path"`
	Version  string      `json:"Version"`
	Replace  *moduleInfo `json:"Replace,omitempty"`
	Main     bool        `json:"Main"`
	Indirect bool        `json:"Indirect"`
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
		// Skip if this module IS the target module (prevent self-inclusion)
		if module.ModulePath == options.TargetModule {
			continue
		}

		depends, err := w.moduleHasDependency(ctx, module.Path, options.TargetModule)
		if err != nil {
			// Log warning but continue with other modules
			continue
		}

		if depends {
			// If target version is specified, check if module needs update
			if options.TargetVersion != "" {
				currentVersion := w.getDependencyVersion(ctx, module.Path, options.TargetModule)
				if currentVersion != "" {
					// Normalize versions for comparison
					normalizedCurrent := currentVersion
					normalizedTarget := options.TargetVersion

					// Ensure versions have 'v' prefix for semver comparison
					if !strings.HasPrefix(normalizedCurrent, "v") {
						normalizedCurrent = "v" + normalizedCurrent
					}
					if !strings.HasPrefix(normalizedTarget, "v") {
						normalizedTarget = "v" + normalizedTarget
					}

					// Skip if already at target version or newer
					if semver.Compare(normalizedCurrent, normalizedTarget) >= 0 {
						continue
					}
				}
			}

			repository := w.inferRepository(module.ModulePath)
			dependent := DependentOptions{
				Repository:      repository,
				CloneURL:        w.buildCloneURL(repository),
				ModulePath:      module.ModulePath,
				LocalModulePath: w.inferLocalModulePath(module.ModulePath),
			}
			dependents = append(dependents, dependent)
		}
	}

	return dependents, nil
}

// ResolveVersion attempts to resolve the current version of a module within the workspace.
func (w *workspaceDiscovery) ResolveVersion(ctx context.Context, options VersionResolutionOptions) (*VersionResolution, error) {
	if options.TargetModule == "" {
		return nil, fmt.Errorf("target module is required")
	}
	if options.WorkspaceDir == "" {
		return nil, fmt.Errorf("workspace directory is required")
	}

	resolution := &VersionResolution{
		Warnings: []string{},
	}

	switch options.Strategy {
	case VersionResolutionLocal:
		return w.resolveLocalVersion(ctx, options.WorkspaceDir, options.TargetModule, resolution)
	case VersionResolutionLatest:
		if !options.AllowNetworkAccess {
			return nil, fmt.Errorf("latest version resolution requires network access")
		}
		return w.resolveLatestVersion(ctx, options.TargetModule, resolution)
	case VersionResolutionAuto:
		// Try local first
		localRes, localErr := w.resolveLocalVersion(ctx, options.WorkspaceDir, options.TargetModule, resolution)
		if localErr == nil && localRes.Version != "" {
			return localRes, nil
		}

		// Add warning about local resolution failure
		if localErr != nil {
			resolution.Warnings = append(resolution.Warnings, fmt.Sprintf("Local resolution failed: %v", localErr))
		}

		// Try network if allowed
		if options.AllowNetworkAccess {
			netRes, netErr := w.resolveLatestVersion(ctx, options.TargetModule, resolution)
			if netErr == nil {
				return netRes, nil
			}
			resolution.Warnings = append(resolution.Warnings, fmt.Sprintf("Network resolution failed: %v", netErr))
		} else {
			resolution.Warnings = append(resolution.Warnings, "Network access not allowed, cannot resolve latest version")
		}

		return nil, fmt.Errorf("failed to resolve version using auto strategy")
	default:
		return nil, fmt.Errorf("unsupported version resolution strategy: %s", options.Strategy)
	}
}

// resolveLocalVersion attempts to find the module version from local workspace modules.
func (w *workspaceDiscovery) resolveLocalVersion(ctx context.Context, workspaceDir, targetModule string, resolution *VersionResolution) (*VersionResolution, error) {
	// Find all Go modules in the workspace
	modules, err := w.findGoModules(ctx, DiscoveryOptions{
		WorkspaceDir: workspaceDir,
		TargetModule: targetModule,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to find Go modules in workspace: %w", err)
	}

	// Check each module for the target dependency
	for _, module := range modules {
		version, err := w.getModuleVersionFromPath(ctx, module.Path, targetModule)
		if err != nil {
			continue // Skip modules where we can't resolve the version
		}
		if version != "" {
			resolution.Version = version
			resolution.Source = VersionSourceLocal
			resolution.SourcePath = module.Path
			return resolution, nil
		}
	}

	return nil, fmt.Errorf("module %s not found in any workspace dependencies", targetModule)
}

// resolveLatestVersion attempts to get the latest version from the Go module proxy.
func (w *workspaceDiscovery) resolveLatestVersion(ctx context.Context, targetModule string, resolution *VersionResolution) (*VersionResolution, error) {
	// Use go list -m -versions to get available versions
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "-versions", targetModule)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list module versions: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("no version information available for module %s", targetModule)
	}

	// The first line should contain: module_path version1 version2 ...
	parts := strings.Fields(lines[0])
	if len(parts) < 2 {
		return nil, fmt.Errorf("unexpected go list output format")
	}

	// Skip the module path and collect versions
	versions := parts[1:]
	if len(versions) == 0 {
		return nil, fmt.Errorf("no versions available for module %s", targetModule)
	}

	// Sort versions using semver and get the latest
	sort.Slice(versions, func(i, j int) bool {
		return semver.Compare(versions[i], versions[j]) < 0
	})

	latestVersion := versions[len(versions)-1]
	resolution.Version = latestVersion
	resolution.Source = VersionSourceNetwork
	return resolution, nil
}

// getModuleVersionFromPath extracts the version of a specific module from a Go module path.
func (w *workspaceDiscovery) getModuleVersionFromPath(ctx context.Context, modulePath, targetModule string) (string, error) {
	// Use go list -m -json to get module information
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "-json", "all")
	cmd.Dir = modulePath

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to list modules: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(output))
	for decoder.More() {
		var module moduleInfo
		if err := decoder.Decode(&module); err != nil {
			return "", fmt.Errorf("failed to decode module info: %w", err)
		}

		if module.Path == targetModule {
			if module.Replace != nil && module.Replace.Version != "" {
				return module.Replace.Version, nil
			}
			return module.Version, nil
		}
	}

	return "", nil // Module not found locally
}

// findGoModules discovers all Go modules within the workspace directory.
func (w *workspaceDiscovery) findGoModules(ctx context.Context, options DiscoveryOptions) ([]DiscoveredModule, error) {
	var allModules []DiscoveredModule

	// First pass: find all go.mod files
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
			moduleDir := filepath.Dir(path)
			// Check include/exclude patterns
			if w.shouldIncludeDirectory(moduleDir, options) {
				modulePath, err := w.extractModulePath(path)
				if err != nil {
					return nil // Skip modules with invalid go.mod files
				}

				module := DiscoveredModule{
					Path:       moduleDir,
					ModulePath: modulePath,
					Repository: w.inferRepository(modulePath),
				}
				allModules = append(allModules, module)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Second pass: filter out modules that are subdirectories of other modules
	var filteredModules []DiscoveredModule
	for _, module := range allModules {
		isSubmodule := false
		for _, other := range allModules {
			if module.Path != other.Path {
				// Check if module.Path is a subdirectory of other.Path
				rel, err := filepath.Rel(other.Path, module.Path)
				if err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
					isSubmodule = true
					break
				}
			}
		}
		if !isSubmodule {
			filteredModules = append(filteredModules, module)
		}
	}

	return filteredModules, nil
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

// getDependencyVersion retrieves the version of a specific dependency from a module's go.mod.
// Returns empty string if the dependency is not found or on error.
func (w *workspaceDiscovery) getDependencyVersion(ctx context.Context, modulePath, targetModule string) string {
	// Try using go list first for accurate version info (handles replace directives)
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "-f", "{{.Version}}", targetModule)
	cmd.Dir = modulePath

	output, err := cmd.Output()
	if err == nil {
		version := strings.TrimSpace(string(output))
		if version != "" && version != "<nil>" {
			return version
		}
	}

	// Fall back to parsing go.mod directly
	goModPath := filepath.Join(modulePath, "go.mod")
	file, err := os.Open(goModPath)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	inRequireBlock := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Handle single-line require statements
		if strings.HasPrefix(line, "require ") {
			parts := strings.Fields(line)
			if len(parts) >= 3 && parts[1] == targetModule {
				return parts[2]
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
			if len(parts) >= 2 && parts[0] == targetModule {
				return parts[1]
			}
		}
	}

	return ""
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

	// For non-hosted URLs or unknown hosts, preserve the original module path
	// This prevents breaking URLs like go.uber.org/zap into invalid repository names
	return modulePath
}

// inferLocalModulePath calculates the relative path from repository root to module
func (w *workspaceDiscovery) inferLocalModulePath(modulePath string) string {
	parts := strings.Split(modulePath, "/")

	if len(parts) >= 4 {
		switch parts[0] {
		case "github.com", "gitlab.com", "bitbucket.org":
			// For hosted repos, everything after org/repo is the local path
			return strings.Join(parts[3:], "/")
		}
	}

	// For non-hosted URLs or short paths, default to root
	// This handles cases like go.uber.org/zap where the entire URL is the "repository"
	return "."
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

// buildCloneURL ensures the repo string is a valid cloneable URL.
// This mirrors the logic from internal/executor/git.go to maintain consistency.
func (w *workspaceDiscovery) buildCloneURL(repo string) string {
	// If it doesn't have a protocol or git@, and is in owner/repo format, assume it's a GitHub repo.
	if !strings.HasPrefix(repo, "https://") &&
		!strings.HasPrefix(repo, "http://") &&
		!strings.HasPrefix(repo, "git@") &&
		strings.Count(repo, "/") == 1 {
		return "https://github.com/" + repo
	}
	return repo
}
