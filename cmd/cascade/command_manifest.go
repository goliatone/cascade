package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/pkg/config"
	"github.com/goliatone/cascade/pkg/di"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// newManifestCommand creates the manifest management subcommand with generate subcommand
func newManifestCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manifest",
		Short: "Manage dependency manifests",
		Long: `Manifest management commands for creating and manipulating dependency manifests.
Use subcommands to perform specific manifest operations.`,
	}

	cmd.AddCommand(newManifestGenerateCommand())
	return cmd
}

// newManifestGenerateCommand creates the manifest generate subcommand
func newManifestGenerateCommand() *cobra.Command {
	var (
		moduleName      string
		modulePath      string
		repository      string
		version         string
		outputPath      string
		dependents      []string
		slackChannel    string
		webhook         string
		force           bool
		yes             bool
		nonInteractive  bool
		workspace       string
		maxDepth        int
		includePatterns []string
		excludePatterns []string
		// GitHub discovery flags
		githubOrg             string
		githubIncludePatterns []string
		githubExcludePatterns []string
	)

	cmd := &cobra.Command{
		Use:     "generate",
		Aliases: []string{"gen"},
		Short:   "Generate a new dependency manifest",
		Long: `Generate creates a new dependency manifest file with intelligent defaults.
The command automatically detects module information from the current directory's go.mod
file and version from .version file or git tags when not explicitly provided.

Smart Defaults:
  - Module path: Auto-detected from go.mod in current directory tree
  - Version: Auto-detected from .version file, VERSION file, or latest git tag
  - Output file: .cascade.yaml (non-conflicting default)
  - GitHub org: Extracted from module path for GitHub.com modules
  - Workspace: Intelligently detected from module location (parent dir, $GOPATH/src/org/, etc.)

When --dependents is omitted, cascade will automatically discover dependent repositories
by scanning the workspace for Go modules that import the target module.

The command will display a summary of discovered dependents and default configurations
before proceeding. Use --yes or --non-interactive to skip confirmation prompts.

Examples:
  cascade manifest generate                                                    # Use all auto-detected defaults
  cascade manifest generate --version=v1.2.3                                 # Override just the version
  cascade manifest generate --output=.cascade.yaml                               # Custom output file
  cascade manifest generate --dependents=owner/repo1,owner/repo2             # Explicit dependents
  cascade manifest generate --workspace=/path/to/workspace --max-depth=3     # Custom workspace discovery
  cascade manifest generate --yes --dry-run                                  # Non-interactive dry run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runManifestGenerate(moduleName, modulePath, repository, version, outputPath, dependents, slackChannel, webhook, force, yes, nonInteractive, workspace, maxDepth, includePatterns, excludePatterns, githubOrg, githubIncludePatterns, githubExcludePatterns)
		},
	}

	// Module and version flags (auto-detected if not provided)
	cmd.Flags().StringVar(&modulePath, "module-path", "", "Go module path (e.g., github.com/example/lib). Auto-detected from go.mod if not provided")
	cmd.Flags().StringVar(&version, "version", "", "Target version (e.g., v1.2.3, latest). Auto-detected from .version file or git tags if not provided")

	// Optional configuration flags
	cmd.Flags().StringVar(&moduleName, "module-name", "", "Human-friendly module name (defaults to basename of module path)")
	cmd.Flags().StringVar(&repository, "repository", "", "GitHub repository (defaults to module path without domain)")
	cmd.Flags().StringVar(&outputPath, "output", "", "Output file path (default: .cascade.yaml)")
	cmd.Flags().StringSliceVar(&dependents, "dependents", []string{}, "Dependent repositories (format: owner/repo). If omitted, discovers dependents in workspace")
	cmd.Flags().StringVar(&slackChannel, "slack-channel", "", "Default Slack notification channel")
	cmd.Flags().StringVar(&webhook, "webhook", "", "Default webhook URL for notifications")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing manifest without prompting")
	cmd.Flags().BoolVar(&yes, "yes", false, "Automatically confirm all prompts")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Run in non-interactive mode (same as --yes)")

	// Workspace discovery flags
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace directory to scan for dependents (default: auto-detected from module location)")
	cmd.Flags().IntVar(&maxDepth, "max-depth", 0, "Maximum depth to scan in workspace directory (0 = no limit)")
	cmd.Flags().StringSliceVar(&includePatterns, "include", []string{}, "Directory patterns to include during discovery")
	cmd.Flags().StringSliceVar(&excludePatterns, "exclude", []string{}, "Directory patterns to exclude during discovery (e.g., vendor, .git)")

	// GitHub discovery flags
	cmd.Flags().StringVar(&githubOrg, "github-org", "", "GitHub organization to search for dependent repositories (auto-detected from module path if not provided)")
	cmd.Flags().StringSliceVar(&githubIncludePatterns, "github-include", []string{}, "Repository name patterns to include during GitHub discovery")
	cmd.Flags().StringSliceVar(&githubExcludePatterns, "github-exclude", []string{}, "Repository name patterns to exclude during GitHub discovery")

	// No required flags - all values can be auto-detected or have sensible defaults

	return cmd
}

func runManifestGenerate(moduleName, modulePath, repository, version, outputPath string, dependents []string, slackChannel, webhook string, force, yes, nonInteractive bool, workspace string, maxDepth int, includePatterns, excludePatterns []string, githubOrg string, githubIncludePatterns, githubExcludePatterns []string) error {
	start := time.Now()
	ctx := context.Background()
	logger := container.Logger()
	cfg := container.Config()

	defer func() {
		if logger != nil {
			logger.Debug("Manifest generate command completed",
				"duration_ms", time.Since(start).Milliseconds(),
				"module_path", modulePath,
				"version", version,
				"output", outputPath,
				"dry_run", cfg.Executor.DryRun,
			)
		}
	}()

	// Detect module information when not explicitly provided
	finalModulePath := strings.TrimSpace(modulePath)
	moduleDir := ""
	if finalModulePath != "" {
		// Module path explicitly provided, try to derive directory
		moduleDir = deriveModuleDirFromPath(finalModulePath)
	}

	// Fall back to auto-detection if no explicit module path or derivation failed
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
	modulePath = finalModulePath

	// Derive defaults from module path
	if moduleName == "" {
		moduleName = deriveModuleName(modulePath)
	}
	if repository == "" {
		repository = deriveRepository(modulePath)
	}
	if githubOrg == "" {
		githubOrg = deriveGitHubOrgFromModule(modulePath)
	}

	// Resolve version if not provided or if "latest" specified
	finalVersion := strings.TrimSpace(version)
	var versionWarnings []string
	if finalVersion == "" {
		detectedVersion, warnings := detectDefaultVersion(ctx, moduleDir)
		versionWarnings = append(versionWarnings, warnings...)
		finalVersion = detectedVersion
	}
	if finalVersion == "" || strings.EqualFold(finalVersion, "latest") {
		workspaceDir := resolveWorkspaceDirWithTarget(workspace, cfg, modulePath, moduleDir)
		resolvedVersion, warnings, err := resolveVersionFromWorkspace(ctx, modulePath, finalVersion, workspaceDir, logger)
		if err != nil {
			if finalVersion == "" {
				return newValidationError("version resolution failed and no explicit version provided", err)
			} else {
				return newValidationError("latest version resolution failed", err)
			}
		}
		finalVersion = resolvedVersion
		versionWarnings = warnings
	}

	version = finalVersion

	// Resolve output path
	finalOutputPath := resolveGenerateOutputPath(outputPath, cfg)
	outputPath = finalOutputPath

	// Resolve discovery options if dependents not explicitly provided
	var discoveredDependents []manifest.DependentOptions
	workspaceDir := ""
	finalDependentOptions := []manifest.DependentOptions{}

	if len(dependents) == 0 {
		workspaceDir = resolveWorkspaceDirWithTarget(workspace, cfg, modulePath, moduleDir)
		mergedDependents, err := performMultiSourceDiscovery(ctx, modulePath, version, githubOrg, workspaceDir, maxDepth,
			includePatterns, excludePatterns, githubIncludePatterns, githubExcludePatterns, cfg, logger)
		if err != nil {
			logger.Warn("Discovery failed, proceeding with empty dependents list", "error", err)
		} else {
			discoveredDependents = mergedDependents

			filteredDependents, skippedDependents := filterDiscoveredDependents(discoveredDependents, modulePath, finalVersion, workspaceDir, logger)
			if len(skippedDependents) > 0 && logger != nil {
				logger.Info("Filtered discovered dependents",
					"skipped", dependentsOptionsToStrings(skippedDependents))
			}
			discoveredDependents = filteredDependents
			finalDependentOptions = append(finalDependentOptions, discoveredDependents...)

			if len(discoveredDependents) > 0 {
				logger.Info("Discovery completed",
					"total_dependents", len(discoveredDependents),
					"dependents", dependentsOptionsToStrings(discoveredDependents))
			}
		}

		if len(discoveredDependents) > 0 && !yes && !nonInteractive {
			filteredDependents, err := promptForDependentSelection(discoveredDependents)
			if err != nil {
				return fmt.Errorf("dependent selection failed: %w", err)
			}
			discoveredDependents = filteredDependents
			finalDependentOptions = append([]manifest.DependentOptions{}, discoveredDependents...)
		}
	} else {
		finalDependentOptions = buildDependentOptions(dependents)
	}

	finalDependentNames := dependentsOptionsToStrings(finalDependentOptions)

	// Display discovery summary and handle confirmation
	if err := displayDiscoverySummary(modulePath, finalVersion, workspaceDir, discoveredDependents, finalDependentNames, versionWarnings, yes, nonInteractive, cfg.Executor.DryRun); err != nil {
		return err
	}

	logger.Info("Generating dependency manifest",
		"module", modulePath,
		"version", finalVersion,
		"output", finalOutputPath)

	// Build generate options with config defaults merged
	defaultBranch := config.ManifestDefaultBranch(cfg)
	defaultTests := buildManifestDefaultTests(cfg)
	slackDefault := slackChannel
	if slackDefault == "" {
		slackDefault = config.ManifestDefaultSlackChannel(cfg)
	}
	webhookDefault := webhook
	if webhookDefault == "" {
		webhookDefault = config.ManifestDefaultWebhook(cfg)
	}

	options := manifest.GenerateOptions{
		ModuleName:        moduleName,
		ModulePath:        modulePath,
		Repository:        repository,
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

	// Generate manifest
	generator := container.ManifestGenerator()
	generatedManifest, err := generator.Generate(ctx, options)
	if err != nil {
		return newValidationError("failed to generate manifest", err)
	}

	manifestToWrite := generatedManifest

	if _, err := os.Stat(finalOutputPath); err == nil {
		loader := container.Manifest()
		if loader != nil {
			existingManifest, loadErr := loader.Load(finalOutputPath)
			if loadErr != nil {
				if logger != nil {
					logger.Warn("Existing manifest could not be parsed; overwriting with generated manifest",
						"path", finalOutputPath,
						"error", loadErr,
					)
				}
			} else {
				manifestToWrite = mergeManifestDependents(existingManifest, generatedManifest)
			}
		}
	}

	// Serialize to YAML
	yamlData, err := yaml.Marshal(manifestToWrite)
	if err != nil {
		return newConfigError("failed to serialize manifest to YAML", err)
	}

	// Handle dry-run vs actual file writing
	if cfg.Executor.DryRun {
		fmt.Printf("DRY RUN: Would write manifest to %s\n", finalOutputPath)
		fmt.Printf("--- Generated Manifest ---\n%s", string(yamlData))
		return nil
	}

	// Check for existing file and handle overwrite logic
	if _, err := os.Stat(finalOutputPath); err == nil {
		if !force {
			fmt.Printf("File %s already exists. Overwrite? [y/N]: ", finalOutputPath)
			var response string
			fmt.Scanln(&response)
			if response != "y" && response != "Y" && response != "yes" && response != "YES" {
				fmt.Println("Manifest generation cancelled.")
				return nil
			}
		} else {
			logger.Info("Overwriting existing manifest with --force flag", "path", finalOutputPath)
		}
	}

	// Write to file
	if err := os.WriteFile(finalOutputPath, yamlData, 0644); err != nil {
		return newFileError("failed to write manifest file", err)
	}

	fmt.Printf("Manifest generated successfully: %s\n", finalOutputPath)
	return nil
}

func mergeManifestDependents(existing, generated *manifest.Manifest) *manifest.Manifest {
	if existing == nil {
		return generated
	}

	if generated == nil || len(generated.Modules) == 0 {
		return existing
	}

	newModule := generated.Modules[0]
	replaced := false

	for i := range existing.Modules {
		module := &existing.Modules[i]
		if module.Module == newModule.Module || module.Repo == newModule.Repo {
			module.Dependents = newModule.Dependents
			replaced = true
			break
		}
	}

	if !replaced {
		existing.Modules = append(existing.Modules, newModule)
	}

	return existing
}

func filterDiscoveredDependents(discovered []manifest.DependentOptions, targetModule, targetVersion, workspaceDir string, logger di.Logger) ([]manifest.DependentOptions, []manifest.DependentOptions) {
	if len(discovered) == 0 {
		return discovered, nil
	}

	var filtered []manifest.DependentOptions
	var skipped []manifest.DependentOptions

	for _, dep := range discovered {
		if dep.ModulePath == targetModule {
			skipped = append(skipped, dep)
			continue
		}

		if dep.DiscoverySource == "workspace" && targetVersion != "" && workspaceDir != "" {
			upToDate, err := workspaceDependentIsUpToDate(dep, targetModule, targetVersion, workspaceDir)
			if err != nil && logger != nil {
				logger.Debug("workspace version check failed",
					"repo", dep.Repository,
					"error", err)
			}
			if err == nil && upToDate {
				skipped = append(skipped, dep)
				continue
			}
		}

		filtered = append(filtered, dep)
	}

	return filtered, skipped
}

func workspaceDependentIsUpToDate(dep manifest.DependentOptions, targetModule, targetVersion, workspaceDir string) (bool, error) {
	repoParts := strings.Split(dep.Repository, "/")
	if len(repoParts) == 0 {
		return false, fmt.Errorf("invalid repository: %s", dep.Repository)
	}

	repoDir := filepath.Join(workspaceDir, repoParts[len(repoParts)-1])
	moduleDir := repoDir
	if dep.LocalModulePath != "" && dep.LocalModulePath != "." {
		moduleDir = filepath.Join(repoDir, dep.LocalModulePath)
	}

	goModPath := filepath.Join(moduleDir, "go.mod")
	version, err := readDependencyVersionFromGoMod(goModPath, targetModule)
	if err != nil {
		return false, err
	}
	if version == "" {
		return false, nil
	}

	needsUpdate, err := planner.CompareVersions(version, targetVersion)
	if err != nil {
		return false, err
	}

	return !needsUpdate, nil
}

func readDependencyVersionFromGoMod(goModPath, targetModule string) (string, error) {
	file, err := os.Open(goModPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	inRequireBlock := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "require ") {
			parts := strings.Fields(line)
			if len(parts) >= 3 && parts[1] == targetModule {
				return parts[2], nil
			}
			if strings.HasSuffix(line, "(") {
				inRequireBlock = true
				continue
			}
		}

		if inRequireBlock {
			if strings.Contains(line, ")") {
				inRequireBlock = false
				continue
			}
			parts := strings.Fields(line)
			if len(parts) >= 2 && parts[0] == targetModule {
				return parts[1], nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", nil
}

func buildManifestDefaultTests(cfg *config.Config) []manifest.Command {
	specs := config.ManifestDefaultTests(cfg)
	if len(specs) == 0 {
		return []manifest.Command{}
	}

	commands := make([]manifest.Command, len(specs))
	for i, spec := range specs {
		commands[i] = manifest.Command{Cmd: spec.Cmd, Dir: spec.Dir}
	}
	return commands
}
