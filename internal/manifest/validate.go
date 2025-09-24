package manifest

import (
	"fmt"
	"strings"
)

// Validate performs schema and dependency checks on a manifest.
func Validate(m *Manifest) error {
	if m == nil {
		return &ValidationError{Issues: []string{"manifest cannot be nil"}}
	}

	var issues []string

	if m.ManifestVersion != 1 {
		issues = append(issues, fmt.Sprintf("unsupported manifest version: %d (expected 1)", m.ManifestVersion))
	}

	if m.Modules == nil {
		issues = append(issues, "modules cannot be nil")
	} else {
		// check for duplicate module names and build moduleByPath map
		moduleNames := make(map[string]bool)
		moduleByPath := make(map[string]string) // modulePath -> name
		for i, module := range m.Modules {
			if module.Name == "" {
				issues = append(issues, fmt.Sprintf("module[%d] name cannot be empty", i))
			} else if moduleNames[module.Name] {
				issues = append(issues, fmt.Sprintf("duplicate module name: %s", module.Name))
			} else {
				moduleNames[module.Name] = true
			}

			// validate required module fields
			if module.Module == "" {
				issues = append(issues, fmt.Sprintf("module[%d] (%s) module path cannot be empty", i, module.Name))
			} else {
				// Build moduleByPath map for later validation
				moduleByPath[module.Module] = module.Name
			}
			if module.Repo == "" {
				issues = append(issues, fmt.Sprintf("module[%d] (%s) repo cannot be empty", i, module.Name))
			}

			// dependents are not nil
			if module.Dependents == nil {
				issues = append(issues, fmt.Sprintf("module[%d] (%s) dependents cannot be nil", i, module.Name))
			} else {
				// check for duplicate dependents within a module
				dependentRepos := make(map[string]bool)
				for j, dep := range module.Dependents {
					if dep.Repo == "" {
						issues = append(issues, fmt.Sprintf("module[%d] (%s) dependent[%d] repo cannot be empty", i, module.Name, j))
					} else if dependentRepos[dep.Repo] {
						issues = append(issues, fmt.Sprintf("module[%d] (%s) has duplicate dependent repo: %s", i, module.Name, dep.Repo))
					} else {
						dependentRepos[dep.Repo] = true
					}

					if dep.Module == "" {
						issues = append(issues, fmt.Sprintf("module[%d] (%s) dependent[%d] (%s) module cannot be empty", i, module.Name, j, dep.Repo))
					}
					if dep.ModulePath == "" {
						issues = append(issues, fmt.Sprintf("module[%d] (%s) dependent[%d] (%s) module_path cannot be empty", i, module.Name, j, dep.Repo))
					}
				}
			}
		}

		// check for dependency cycles using the moduleByPath map
		if cycleIssues := detectCycles(m.Modules, moduleByPath); len(cycleIssues) > 0 {
			issues = append(issues, cycleIssues...)
		}
	}

	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}

	return nil
}

// detectCycles uses DFS to find dependency cycles in the module graph.
func detectCycles(modules []Module, moduleByPath map[string]string) []string {
	var issues []string

	moduleMap := make(map[string]*Module)
	for i := range modules {
		moduleMap[modules[i].Name] = &modules[i]
	}

	// track visited states 0=unvisited, 1=visiting, 2=visited
	visited := make(map[string]int)

	for _, module := range modules {
		if visited[module.Name] == 0 {
			if cycle := dfs(module.Name, moduleMap, moduleByPath, visited, []string{}); len(cycle) > 0 {
				issues = append(issues, fmt.Sprintf("dependency cycle detected: %s", strings.Join(cycle, " -> ")))
			}
		}
	}

	return issues
}

// dfs performs depth-first search to detect cycles.
// Returns the cycle path if found, empty slice otherwise.
func dfs(moduleName string, moduleMap map[string]*Module, moduleByPath map[string]string, visited map[string]int, path []string) []string {
	if visited[moduleName] == 1 {
		// found a cycle, find where it starts
		cycleStart := -1
		for i, name := range path {
			if name == moduleName {
				cycleStart = i
				break
			}
		}
		if cycleStart >= 0 {
			cycle := append(path[cycleStart:], moduleName)
			return cycle
		}
	}

	if visited[moduleName] == 2 {
		return nil // Already processed
	}

	visited[moduleName] = 1 // Mark as visiting
	path = append(path, moduleName)

	module := moduleMap[moduleName]
	if module != nil {
		// Check dependencies through dependents - find modules that this dependent's module matches
		for _, dep := range module.Dependents {
			// Look for modules that this dependent depends on by matching Module field
			if referencedModuleName, exists := moduleByPath[dep.Module]; exists {
				if referencedModuleName != moduleName { // Skip self
					if cycle := dfs(referencedModuleName, moduleMap, moduleByPath, visited, path); len(cycle) > 0 {
						return cycle
					}
				}
			}
			// If dep.Module doesn't match any module in our manifest, that's okay
			// It means this dependent uses external modules not managed by cascade
		}
	}

	visited[moduleName] = 2 // Mark as visited
	return nil
}
