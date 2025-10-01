package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// detectModuleInfo walks up from the current working directory to locate a go.mod file
// and returns the module path with the directory that contains it. It enables
// `cascade manifest generate` to infer sensible defaults without requiring flags.
func detectModuleInfo() (string, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("determine working directory: %w", err)
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
			return "", "", fmt.Errorf("stat go.mod: %w", err)
		}
		if info.IsDir() {
			continue
		}

		content, err := os.ReadFile(goModPath)
		if err != nil {
			return "", "", fmt.Errorf("read go.mod: %w", err)
		}
		modulePath := parseGoModModulePath(string(content))
		if modulePath == "" {
			return "", "", fmt.Errorf("module declaration not found in %s", goModPath)
		}
		return modulePath, dir, nil
	}

	return "", "", fmt.Errorf("go.mod not found in current tree")
}

// detectDefaultVersion inspects common local sources for a module version.
// Priority: `.version` file (or `VERSION`), then latest annotated tag in git.
// Returns any warnings encountered while probing so the CLI can surface them.
func detectDefaultVersion(ctx context.Context, moduleDir string) (string, []string) {
	var warnings []string

	if strings.TrimSpace(moduleDir) == "" {
		return "", warnings
	}

	versionFiles := []string{filepath.Join(moduleDir, ".version"), filepath.Join(moduleDir, "VERSION")}
	for _, candidate := range versionFiles {
		data, err := os.ReadFile(candidate)
		if err != nil {
			if !os.IsNotExist(err) {
				warnings = append(warnings, fmt.Sprintf("failed to read %s: %v", candidate, err))
			}
			continue
		}
		if v := normalizeVersionString(string(data)); v != "" {
			return v, warnings
		}
	}

	if _, err := os.Stat(filepath.Join(moduleDir, ".git")); err == nil {
		cmd := exec.CommandContext(ctx, "git", "-C", moduleDir, "describe", "--tags", "--abbrev=0")
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		output, err := cmd.Output()
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("git tag detection failed: %v", err))
		} else if v := normalizeVersionString(string(output)); v != "" {
			return v, warnings
		}
	} else if !os.IsNotExist(err) {
		warnings = append(warnings, fmt.Sprintf("git metadata unavailable: %v", err))
	}

	return "", warnings
}

// normalizeVersionString trims whitespace and ensures versions have the expected leading "v".
func normalizeVersionString(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "v") {
		return trimmed
	}
	if trimmed[0] >= '0' && trimmed[0] <= '9' {
		return "v" + trimmed
	}
	return trimmed
}

// deriveModuleName extracts the module name from the module path
func deriveModuleName(modulePath string) string {
	if modulePath == "" {
		return ""
	}
	parts := strings.Split(modulePath, "/")
	return parts[len(parts)-1]
}

// deriveGitHubOrgFromModule extracts the GitHub organization from a module path when available.
func deriveGitHubOrgFromModule(modulePath string) string {
	parts := strings.Split(modulePath, "/")
	if len(parts) >= 3 && parts[0] == "github.com" {
		return parts[1]
	}
	return ""
}
