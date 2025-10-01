package main

import (
	"context"
	"strings"

	"github.com/goliatone/cascade/pkg/config"
	"github.com/goliatone/cascade/pkg/workspace"
)

// applyManifestDefaults applies default value discovery logic for manifest path
func applyManifestDefaults(manifestPath string, cfg *config.Config) string {
	return resolveManifestPath(manifestPath, cfg)
}

// applyModuleDefaults applies default value discovery logic for module path
func applyModuleDefaults(modulePath string) (string, string, error) {
	finalModulePath := strings.TrimSpace(modulePath)
	moduleDir := ""

	if autoModulePath, autoModuleDir, err := detectModuleInfo(); err == nil {
		moduleDir = autoModuleDir
		if finalModulePath == "" {
			finalModulePath = autoModulePath
		}
	} else if finalModulePath == "" {
		return "", "", newValidationError("module path must be provided or go.mod must be present in the current directory", err)
	}

	return finalModulePath, moduleDir, nil
}

// applyVersionDefaults applies default value discovery logic for version
func applyVersionDefaults(ctx context.Context, version, moduleDir string, cfg *config.Config) (string, []string, error) {
	finalVersion := strings.TrimSpace(version)
	var versionWarnings []string

	if finalVersion == "" {
		detectedVersion, warnings := detectDefaultVersion(ctx, moduleDir)
		versionWarnings = append(versionWarnings, warnings...)
		finalVersion = detectedVersion
	}

	if finalVersion == "" || strings.EqualFold(finalVersion, "latest") {
		workspaceDir := workspace.Resolve("", cfg, "", "")
		resolvedVersion, warnings, err := resolveVersionFromWorkspace(ctx, "", finalVersion, workspaceDir, container.Logger())
		if err != nil {
			if finalVersion == "" {
				return "", versionWarnings, newValidationError("version resolution failed and no explicit version provided", err)
			} else {
				return "", versionWarnings, newValidationError("latest version resolution failed", err)
			}
		}
		finalVersion = resolvedVersion
		versionWarnings = warnings
	}

	return finalVersion, versionWarnings, nil
}
