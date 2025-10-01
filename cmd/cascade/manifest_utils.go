package main

import (
	"path/filepath"
	"strings"

	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/pkg/config"
	"github.com/goliatone/cascade/pkg/util/modpath"
)

func buildDependentOptions(dependents []string) []manifest.DependentOptions {
	if len(dependents) == 0 {
		return []manifest.DependentOptions{}
	}

	options := make([]manifest.DependentOptions, len(dependents))
	for i, dep := range dependents {
		repo := strings.TrimSpace(dep)
		modulePath := ""

		if strings.Count(repo, "/") == 1 && !strings.Contains(repo, ".") {
			modulePath = "github.com/" + repo
		} else {
			modulePath = repo
			repo = modpath.DeriveRepository(repo)
		}

		options[i] = manifest.DependentOptions{
			Repository:      repo,
			CloneURL:        modpath.BuildCloneURL(repo),
			ModulePath:      modulePath,
			LocalModulePath: modpath.DeriveLocalModulePath(modulePath),
		}
	}

	return options
}

func resolveGenerateOutputPath(outputPath string, cfg *config.Config) string {
	if outputPath != "" {
		if !filepath.IsAbs(outputPath) {
			if abs, err := filepath.Abs(outputPath); err == nil {
				return abs
			}
		}
		return outputPath
	}

	if cfg != nil && cfg.Workspace.ManifestPath != "" {
		return cfg.Workspace.ManifestPath
	}

	if abs, err := filepath.Abs(".cascade.yaml"); err == nil {
		return abs
	}

	return ".cascade.yaml"
}

func resolveManifestPath(manifestPath string, cfg *config.Config) string {
	path := strings.TrimSpace(manifestPath)
	if path != "" {
		if !filepath.IsAbs(path) {
			if abs, err := filepath.Abs(path); err == nil {
				return abs
			}
		}
		return path
	}

	if abs, err := filepath.Abs(".cascade.yaml"); err == nil {
		return abs
	}

	if cfg != nil {
		if candidate := strings.TrimSpace(cfg.Workspace.ManifestPath); candidate != "" {
			return candidate
		}
		if base := strings.TrimSpace(cfg.Workspace.Path); base != "" {
			return filepath.Join(base, ".cascade.yaml")
		}
	}

	return ""
}

func resolvePlanManifestPath(manifestFlag, manifestArg string, cfg *config.Config) string {
	if manifestFlag != "" {
		return resolveManifestPath(manifestFlag, cfg)
	}
	if manifestArg != "" {
		return resolveManifestPath(manifestArg, cfg)
	}
	return resolveManifestPath("", cfg)
}

func dependentsOptionsToStrings(dependents []manifest.DependentOptions) []string {
	if len(dependents) == 0 {
		return []string{}
	}

	result := make([]string, len(dependents))
	for i, dep := range dependents {
		result[i] = dep.Repository
	}

	return result
}
