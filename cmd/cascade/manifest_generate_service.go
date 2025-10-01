package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/manifest/persist"
	"github.com/goliatone/cascade/pkg/config"
	"github.com/goliatone/cascade/pkg/util/modpath"
	workspacepkg "github.com/goliatone/cascade/pkg/workspace"
)

type manifestGenerateRequest struct {
	ModuleName      string
	ModulePath      string
	Repository      string
	Version         string
	OutputPath      string
	Dependents      []string
	SlackChannel    string
	Webhook         string
	Force           bool
	Yes             bool
	NonInteractive  bool
	Workspace       string
	MaxDepth        int
	IncludePatterns []string
	ExcludePatterns []string
	GitHubOrg       string
	GitHubInclude   []string
	GitHubExclude   []string
}

func manifestGenerate(ctx context.Context, req manifestGenerateRequest, cfg *config.Config) error {
	logger := container.Logger()

	start := time.Now()
	defer func() {
		if logger != nil {
			logger.Debug("Manifest generate command completed",
				"duration_ms", time.Since(start).Milliseconds(),
				"module_path", req.ModulePath,
				"version", req.Version,
				"output", req.OutputPath,
				"dry_run", cfg.Executor.DryRun,
			)
		}
	}()

	finalModulePath := strings.TrimSpace(req.ModulePath)
	moduleDir := ""
	if finalModulePath != "" {
		moduleDir = workspacepkg.DeriveModuleDirFromPath(finalModulePath)
	}

	if finalModulePath == "" || moduleDir == "" {
		if autoModulePath, autoModuleDir, err := detectModuleInfo(); err == nil {
			if moduleDir == "" {
				moduleDir = autoModuleDir
			}
			if finalModulePath == "" {
				finalModulePath = autoModulePath
			}
		} else if finalModulePath == "" {
			return newValidationError("module path must be provided or go.mod must be present in the current directory", err)
		}
	}

	req.ModulePath = finalModulePath

	if req.ModuleName == "" {
		req.ModuleName = deriveModuleName(req.ModulePath)
	}
	if req.Repository == "" {
		req.Repository = modpath.DeriveRepository(req.ModulePath)
	}
	if req.GitHubOrg == "" {
		req.GitHubOrg = deriveGitHubOrgFromModule(req.ModulePath)
	}

	finalVersion := strings.TrimSpace(req.Version)
	var versionWarnings []string
	if finalVersion == "" {
		detectedVersion, warnings := detectDefaultVersion(ctx, moduleDir)
		versionWarnings = append(versionWarnings, warnings...)
		finalVersion = detectedVersion
	}
	if finalVersion == "" || strings.EqualFold(finalVersion, "latest") {
		workspaceDir := workspacepkg.Resolve(req.Workspace, cfg, req.ModulePath, moduleDir)
		resolvedVersion, warnings, err := resolveVersionFromWorkspace(ctx, req.ModulePath, finalVersion, workspaceDir, logger)
		if err != nil {
			if finalVersion == "" {
				return newValidationError("version resolution failed and no explicit version provided", err)
			}
			return newValidationError("latest version resolution failed", err)
		}
		finalVersion = resolvedVersion
		versionWarnings = warnings
	}
	req.Version = finalVersion

	finalOutputPath := resolveGenerateOutputPath(req.OutputPath, cfg)
	req.OutputPath = finalOutputPath

	var discoveredDependents []manifest.DependentOptions
	workspaceDir := ""
	finalDependentOptions := []manifest.DependentOptions{}

	if len(req.Dependents) == 0 {
		workspaceDir = workspacepkg.Resolve(req.Workspace, cfg, req.ModulePath, moduleDir)
		mergedDependents, err := performMultiSourceDiscovery(ctx, req.ModulePath, req.Version, req.GitHubOrg, workspaceDir, req.MaxDepth,
			req.IncludePatterns, req.ExcludePatterns, req.GitHubInclude, req.GitHubExclude, cfg, logger)
		if err != nil {
			if logger != nil {
				logger.Warn("Discovery failed, proceeding with empty dependents list", "error", err)
			}
		} else {
			discoveredDependents = mergedDependents

			filtered, skipped := filterDiscoveredDependents(discoveredDependents, req.ModulePath, finalVersion, workspaceDir, logger)
			if len(skipped) > 0 && logger != nil {
				logger.Info("Filtered discovered dependents", "skipped", dependentsOptionsToStrings(skipped))
			}
			discoveredDependents = filtered
			finalDependentOptions = append(finalDependentOptions, discoveredDependents...)

			if len(discoveredDependents) > 0 && logger != nil {
				logger.Info("Discovery completed",
					"total_dependents", len(discoveredDependents),
					"dependents", dependentsOptionsToStrings(discoveredDependents))
			}
		}

		if len(discoveredDependents) > 0 && !req.Yes && !req.NonInteractive {
			filteredDependents, err := promptForDependentSelection(discoveredDependents)
			if err != nil {
				return fmt.Errorf("dependent selection failed: %w", err)
			}
			discoveredDependents = filteredDependents
			finalDependentOptions = append([]manifest.DependentOptions{}, discoveredDependents...)
		}
	} else {
		finalDependentOptions = buildDependentOptions(req.Dependents)
	}

	finalDependentNames := dependentsOptionsToStrings(finalDependentOptions)
	if err := displayDiscoverySummary(req.ModulePath, finalVersion, workspaceDir, discoveredDependents, finalDependentNames, versionWarnings, req.Yes, req.NonInteractive, cfg.Executor.DryRun); err != nil {
		return err
	}

	if logger != nil {
		logger.Info("Generating dependency manifest",
			"module", req.ModulePath,
			"version", finalVersion,
			"output", finalOutputPath)
	}

	defaultBranch := config.ManifestDefaultBranch(cfg)
	defaultTests := buildManifestDefaultTests(cfg)

	slackDefault := req.SlackChannel
	if slackDefault == "" {
		slackDefault = config.ManifestDefaultSlackChannel(cfg)
	}
	webhookDefault := req.Webhook
	if webhookDefault == "" {
		webhookDefault = config.ManifestDefaultWebhook(cfg)
	}

	options := manifest.GenerateOptions{
		ModuleName:        req.ModuleName,
		ModulePath:        req.ModulePath,
		Repository:        req.Repository,
		Version:           finalVersion,
		Dependents:        finalDependentOptions,
		DefaultBranch:     defaultBranch,
		DefaultLabels:     []string{"automation:cascade"},
		DefaultCommitTmpl: "chore(deps): bump {{ .Module }} to {{ .Version }}",
		DefaultTests:      defaultTests,
		DefaultNotifications: manifest.Notifications{
			SlackChannel: slackDefault,
			Webhook:      webhookDefault,
		},
		DefaultPRConfig: manifest.PRConfig{
			TitleTemplate: "chore(deps): bump {{ .Module }} to {{ .Version }}",
			BodyTemplate:  "Automated dependency update for {{ .Module }} to {{ .Version }}",
		},
	}

	generator := container.ManifestGenerator()
	generatedManifest, err := generator.Generate(ctx, options)
	if err != nil {
		return newValidationError("failed to generate manifest", err)
	}
	if generatedManifest != nil && len(generatedManifest.Modules) > 0 {
		module := &generatedManifest.Modules[0]
		if module.Name == "" {
			module.Name = req.ModuleName
		}
		if module.Module == "" {
			module.Module = req.ModulePath
		}
		if module.Repo == "" {
			module.Repo = req.Repository
		}
	}

	fileExists := false
	if info, err := os.Stat(finalOutputPath); err == nil && !info.IsDir() {
		fileExists = true
	}

	shouldWrite := !cfg.Executor.DryRun
	if shouldWrite && fileExists {
		if !req.Force {
			fmt.Printf("File %s already exists. Overwrite? [y/N]: ", finalOutputPath)
			var response string
			fmt.Scanln(&response)
			if response != "y" && response != "Y" && response != "yes" && response != "YES" {
				fmt.Println("Manifest generation cancelled.")
				return nil
			}
		} else if logger != nil {
			logger.Info("Overwriting existing manifest with --force flag", "path", finalOutputPath)
		}
	}

	persistor := persist.NewPersistor(container.Manifest())
	result, err := persistor.Save(generatedManifest, persist.Options{
		Path:          finalOutputPath,
		TargetModule:  req.ModulePath,
		TargetVersion: finalVersion,
		DryRun:        !shouldWrite,
	})
	if err != nil {
		var validationErr *manifest.ValidationError
		if errors.As(err, &validationErr) {
			return newValidationError("manifest validation failed", validationErr)
		}
		if shouldWrite {
			return newFileError("failed to persist manifest", err)
		}
		return newConfigError("failed to prepare manifest", err)
	}

	if logger != nil {
		for _, warning := range result.Report.LoadWarnings {
			logger.Warn("Manifest persistence warning", "warning", warning)
		}
		if len(result.Report.DeduplicatedRepos) > 0 {
			logger.Info("Deduplicated dependents during manifest sanitization", "repos", result.Report.DeduplicatedRepos)
		}
		if len(result.Report.DroppedDependents) > 0 {
			droppedRepos := make([]string, len(result.Report.DroppedDependents))
			for i, drop := range result.Report.DroppedDependents {
				droppedRepos[i] = drop.Repo
			}
			logger.Info("Dropped dependents during manifest sanitization",
				"count", len(result.Report.DroppedDependents),
				"repos", droppedRepos,
			)
		}
		if result.Report.ManifestVersionUpdated {
			logger.Info("Normalized manifest schema version", "version", result.Manifest.ManifestVersion)
		}
	}

	if !shouldWrite {
		fmt.Printf("DRY RUN: Would write manifest to %s\n", finalOutputPath)
		fmt.Printf("--- Generated Manifest ---\n%s", string(result.YAML))
		return nil
	}

	fmt.Printf("Manifest generated successfully: %s\n", finalOutputPath)
	return nil
}
