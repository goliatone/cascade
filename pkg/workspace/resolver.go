package workspace

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/goliatone/cascade/pkg/config"
)

// Resolve returns the workspace directory using unified heuristics.
func Resolve(explicit string, cfg *config.Config, targetModulePath, targetModuleDir string) string {
	if explicit != "" {
		if !filepath.IsAbs(explicit) {
			if abs, err := filepath.Abs(explicit); err == nil {
				return abs
			}
		}
		return explicit
	}

	if intelligent := detectDefaultWorkspaceWithTarget(targetModulePath, targetModuleDir); intelligent != "" {
		return intelligent
	}

	if cfg != nil {
		if path := strings.TrimSpace(cfg.Workspace.Path); path != "" {
			return path
		}
		if path := strings.TrimSpace(cfg.ManifestGenerator.DefaultWorkspace); path != "" {
			return path
		}
	}

	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".cache", "cascade")
	}

	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}

	return "."
}

// ResolveForConfig resolves a workspace when only configuration values are provided.
func ResolveForConfig(cfg *config.Config) string {
	return Resolve("", cfg, "", "")
}

// DeriveModuleDirFromPath attempts to derive a module directory from typical Go layouts.
func DeriveModuleDirFromPath(modulePath string) string {
	if modulePath == "" {
		return ""
	}

	gopath := os.Getenv("GOPATH")
	if gopath == "" {
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

	if home, err := os.UserHomeDir(); err == nil {
		for _, moduleDir := range []string{
			filepath.Join(home, "Development", "GO", "src", modulePath),
			filepath.Join(home, "dev", "go", "src", modulePath),
			filepath.Join(home, "src", modulePath),
		} {
			if isValidWorkspace(moduleDir) {
				if goModPath := filepath.Join(moduleDir, "go.mod"); isValidModuleDir(goModPath) {
					return moduleDir
				}
			}
		}
	}

	return ""
}

func detectDefaultWorkspaceWithTarget(targetModulePath, targetModuleDir string) string {
	modulePath := targetModulePath
	moduleDir := targetModuleDir

	if modulePath == "" || moduleDir == "" {
		detectedPath, detectedDir, err := detectModuleInfo()
		if err != nil {
			return ""
		}
		modulePath = detectedPath
		moduleDir = detectedDir
	}

	if parentWorkspace := detectParentWorkspace(moduleDir, modulePath); parentWorkspace != "" {
		return parentWorkspace
	}

	if gopathOrgWorkspace := detectGopathOrgWorkspace(modulePath); gopathOrgWorkspace != "" {
		return gopathOrgWorkspace
	}

	if gopathWorkspace := detectGopathWorkspace(); gopathWorkspace != "" {
		return gopathWorkspace
	}

	return ""
}

func detectModuleInfo() (string, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", err
	}

	for dir := cwd; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		goModPath := filepath.Join(dir, "go.mod")
		info, err := os.Stat(goModPath)
		if err != nil {
			if os.IsNotExist(err) {
				if parent := filepath.Dir(dir); parent == dir {
					break
				}
				continue
			}
			return "", "", err
		}
		if info.IsDir() {
			continue
		}

		content, err := os.ReadFile(goModPath)
		if err != nil {
			return "", "", err
		}
		modulePath := parseGoModModulePath(string(content))
		if modulePath == "" {
			return "", "", os.ErrInvalid
		}
		return modulePath, dir, nil
	}

	return "", "", os.ErrNotExist
}

func parseGoModModulePath(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1]
			}
		}
	}
	return ""
}

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

func isValidModuleDir(goModPath string) bool {
	info, err := os.Stat(goModPath)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func detectParentWorkspace(moduleDir, modulePath string) string {
	if moduleDir == "" {
		return ""
	}

	org := extractOrgFromModulePath(modulePath)
	if org == "" {
		return ""
	}

	current := moduleDir
	var potential string

	for i := 0; i < 5; i++ {
		parent := filepath.Dir(current)
		if parent == current || parent == "/" || parent == "." {
			break
		}

		if containsMultipleModules(parent) {
			if filepath.Base(parent) == org {
				return parent
			}
			if potential == "" {
				potential = parent
			}
		}
		current = parent
	}

	return potential
}

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

func containsMultipleModules(dir string) bool {
	moduleCount := 0
	maxCheck := 50

	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if moduleCount >= maxCheck {
			return filepath.SkipDir
		}
		if strings.Count(strings.TrimPrefix(path, dir), string(filepath.Separator)) > 3 {
			return filepath.SkipDir
		}
		switch filepath.Base(path) {
		case ".git", "vendor", "node_modules", ".cache":
			return filepath.SkipDir
		}
		if !info.IsDir() && info.Name() == "go.mod" {
			moduleCount++
			if moduleCount >= 2 {
				return filepath.SkipAll
			}
		}
		return nil
	})

	return moduleCount >= 2
}

func detectGopathOrgWorkspace(modulePath string) string {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		if home, err := os.UserHomeDir(); err == nil {
			gopath = filepath.Join(home, "go")
		}
	}
	if gopath == "" {
		return ""
	}

	parts := strings.Split(modulePath, "/")
	if len(parts) < 3 {
		return ""
	}

	hosting := parts[0]
	org := parts[1]

	orgPath := filepath.Join(gopath, "src", hosting, org)
	if isValidWorkspace(orgPath) {
		return orgPath
	}
	return ""
}

func detectGopathWorkspace() string {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
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
