package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/pkg/config"
	"github.com/goliatone/cascade/pkg/di"
	"github.com/spf13/cobra"
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
	req := manifestGenerateRequest{}

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
			return runManifestGenerate(req)
		},
	}

	// Core metadata flags
	cmd.Flags().StringVar(&req.ModuleName, "module-name", "", "Human-friendly module name (defaults to basename of module path)")
	cmd.Flags().StringVar(&req.ModulePath, "module-path", "", "Go module path (e.g., github.com/example/lib). Auto-detected from go.mod if not provided")
	cmd.Flags().StringVar(&req.Repository, "repository", "", "GitHub repository (defaults to module path without domain)")
	cmd.Flags().StringVar(&req.Version, "version", "", "Target version (e.g., v1.2.3, latest). Auto-detected from .version file or git tags if not provided")

	// Output and dependent configuration
	cmd.Flags().StringVar(&req.OutputPath, "output", "", "Output file path (default: .cascade.yaml)")
	cmd.Flags().StringSliceVar(&req.Dependents, "dependents", []string{}, "Dependent repositories (format: owner/repo). If omitted, discovers dependents in workspace")
	cmd.Flags().StringVar(&req.SlackChannel, "slack-channel", "", "Default Slack notification channel")
	cmd.Flags().StringVar(&req.Webhook, "webhook", "", "Default webhook URL for notifications")

	addConfirmationFlags(cmd, &req)
	addWorkspaceDiscoveryFlags(cmd, &req)
	addGitHubDiscoveryFlags(cmd, &req)

	// No required flags - all values can be auto-detected or have sensible defaults
	return cmd
}

func runManifestGenerate(options manifestGenerateRequest) error {
	return manifestGenerate(context.Background(), options, container.Config())
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
