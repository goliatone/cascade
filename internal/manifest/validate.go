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

	// Schema validation
	if m.ManifestVersion != 1 {
		issues = append(issues, fmt.Sprintf("unsupported manifest version: %d (expected 1)", m.ManifestVersion))
	}

	// Validate modules are not nil
	if m.Modules == nil {
		issues = append(issues, "modules cannot be nil")
	} else {
		// Check for duplicate module names
		moduleNames := make(map[string]bool)
		for i, module := range m.Modules {
			if module.Name == "" {
				issues = append(issues, fmt.Sprintf("module[%d] name cannot be empty", i))
			} else if moduleNames[module.Name] {
				issues = append(issues, fmt.Sprintf("duplicate module name: %s", module.Name))
			} else {
				moduleNames[module.Name] = true
			}

			// Validate required module fields
			if module.Module == "" {
				issues = append(issues, fmt.Sprintf("module[%d] (%s) module path cannot be empty", i, module.Name))
			}
			if module.Repo == "" {
				issues = append(issues, fmt.Sprintf("module[%d] (%s) repo cannot be empty", i, module.Name))
			}

			// Validate dependents are not nil
			if module.Dependents == nil {
				issues = append(issues, fmt.Sprintf("module[%d] (%s) dependents cannot be nil", i, module.Name))
			} else {
				// Check for duplicate dependents within a module
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

		// Check for dependency cycles
		if cycleIssues := detectCycles(m.Modules); len(cycleIssues) > 0 {
			issues = append(issues, cycleIssues...)
		}
	}

	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}

	return nil
}

// detectCycles uses DFS to find dependency cycles in the module graph.
func detectCycles(modules []Module) []string {
	var issues []string

	// Build a graph of module dependencies
	moduleMap := make(map[string]*Module)
	for i := range modules {
		moduleMap[modules[i].Name] = &modules[i]
	}

	// Track visited states: 0=unvisited, 1=visiting, 2=visited
	visited := make(map[string]int)

	// Perform DFS from each unvisited module
	for _, module := range modules {
		if visited[module.Name] == 0 {
			if cycle := dfs(module.Name, moduleMap, visited, []string{}); len(cycle) > 0 {
				issues = append(issues, fmt.Sprintf("dependency cycle detected: %s", strings.Join(cycle, " -> ")))
			}
		}
	}

	return issues
}

// dfs performs depth-first search to detect cycles.
// Returns the cycle path if found, empty slice otherwise.
func dfs(moduleName string, moduleMap map[string]*Module, visited map[string]int, path []string) []string {
	if visited[moduleName] == 1 {
		// Found a cycle - find where it starts
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
		// Check dependencies through dependents
		for _, dep := range module.Dependents {
			// Look for modules that this dependent depends on
			for depModuleName := range moduleMap {
				if depModuleName == moduleName {
					continue // Skip self
				}
				// Check if this dependent's module matches any other module
				depModule := moduleMap[depModuleName]
				if depModule != nil && dep.Module == depModule.Module {
					if cycle := dfs(depModuleName, moduleMap, visited, path); len(cycle) > 0 {
						return cycle
					}
				}
			}
		}
	}

	visited[moduleName] = 2 // Mark as visited
	return nil
}
