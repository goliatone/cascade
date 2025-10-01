package main

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/pkg/config"
	"github.com/goliatone/cascade/pkg/di"
)

func resolveVersionFromWorkspace(ctx context.Context, modulePath, version, workspaceDir string, logger di.Logger) (string, []string, error) {
	discovery := manifest.NewWorkspaceDiscovery()

	var strategy manifest.VersionResolutionStrategy
	allowNetwork := true

	if version == "latest" {
		strategy = manifest.VersionResolutionLatest
	} else {
		strategy = manifest.VersionResolutionAuto
	}

	options := manifest.VersionResolutionOptions{
		WorkspaceDir:       workspaceDir,
		TargetModule:       modulePath,
		Strategy:           strategy,
		AllowNetworkAccess: allowNetwork,
	}

	resolution, err := discovery.ResolveVersion(ctx, options)
	if err != nil {
		return "", nil, err
	}

	if logger != nil {
		logger.Info("Version resolved",
			"module", modulePath,
			"version", resolution.Version,
			"source", string(resolution.Source),
			"source_path", resolution.SourcePath)
	}

	return resolution.Version, resolution.Warnings, nil
}

func displayDiscoverySummary(modulePath, version, workspaceDir string, discoveredDependents []manifest.DependentOptions, finalDependents, versionWarnings []string, yes, nonInteractive, dryRun bool) error {
	shouldShowSummary := workspaceDir != "" || len(finalDependents) > 0
	if !shouldShowSummary {
		return nil
	}

	fmt.Printf("Generating manifest for %s@%s\n", modulePath, version)

	if workspaceDir != "" {
		fmt.Printf("Discovery workspace: %s\n", workspaceDir)
	}

	if len(discoveredDependents) > 0 {
		fmt.Printf("Discovered %d dependent repositories:\n", len(discoveredDependents))
		for i, dep := range discoveredDependents {
			fmt.Printf("  %d. %s (module: %s)\n", i+1, dep.Repository, dep.ModulePath)
		}
	} else if len(finalDependents) > 0 {
		fmt.Printf("Using %d configured dependent repositories:\n", len(finalDependents))
		for i, dep := range finalDependents {
			fmt.Printf("  %d. %s\n", i+1, dep)
		}
	} else {
		fmt.Println("No dependent repositories found or configured.")
	}

	if len(versionWarnings) > 0 {
		fmt.Println("\nVersion Resolution Warnings:")
		for _, warning := range versionWarnings {
			fmt.Printf("  ! %s\n", warning)
		}
	}

	fmt.Println("\nDefault configurations:")
	fmt.Println("  Branch: main")
	fmt.Println("  Labels: [automation:cascade]")
	fmt.Println("  Test commands: go test ./... -race -count=1")
	fmt.Println("  Commit template: chore(deps): bump {{ .Module }} to {{ .Version }}")
	fmt.Println("  PR title: chore(deps): bump {{ .Module }} to {{ .Version }}")

	if !dryRun && !yes && !nonInteractive {
		fmt.Printf("\nProceed with manifest generation? [Y/n]: ")
		var response string
		fmt.Scanln(&response)
		if response != "" && (response == "n" || response == "N" || response == "no" || response == "NO") {
			fmt.Println("Manifest generation cancelled.")
			return fmt.Errorf("manifest generation cancelled by user")
		}
	}

	if dryRun {
		fmt.Println("\n--- DRY RUN: Would proceed with manifest generation ---")
	} else if yes || nonInteractive {
		fmt.Println("\n--- Proceeding with manifest generation ---")
	}

	return nil
}

func promptForDependentSelection(dependents []manifest.DependentOptions) ([]manifest.DependentOptions, error) {
	if len(dependents) == 0 {
		return dependents, nil
	}

	fmt.Printf("\nDiscovered %d dependent repositories:\n\n", len(dependents))

	for i, dep := range dependents {
		source := dep.DiscoverySource
		if source == "" {
			source = "unknown"
		}
		fmt.Printf("  %d. %s (module: %s, source: %s)\n", i+1, dep.Repository, dep.ModulePath, source)
		if dep.LocalModulePath != "." && dep.LocalModulePath != "" {
			fmt.Printf("     Local path: %s\n", dep.LocalModulePath)
		}
	}

	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  a - include all")
	fmt.Println("  n - include none")
	fmt.Println("  1,2,3 - include specific repositories by number")
	fmt.Println("  1-3,5 - include ranges and specific repositories")
	fmt.Print("\nSelect dependents to include [a]: ")

	var input string
	fmt.Scanln(&input)

	if input == "" || input == "a" || input == "all" {
		return dependents, nil
	}

	if input == "n" || input == "none" {
		return []manifest.DependentOptions{}, nil
	}

	selectedIndices, err := parseSelectionInput(input, len(dependents))
	if err != nil {
		return nil, fmt.Errorf("invalid selection: %w", err)
	}

	result := make([]manifest.DependentOptions, 0, len(selectedIndices))
	for _, index := range selectedIndices {
		result = append(result, dependents[index])
	}

	fmt.Printf("Selected %d dependents for inclusion.\n", len(result))
	return result, nil
}

func parseSelectionInput(input string, maxIndex int) ([]int, error) {
	if input == "" {
		return nil, fmt.Errorf("empty input")
	}

	var indices []int
	parts := strings.Split(input, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				return nil, fmt.Errorf("invalid range format: %s", part)
			}

			start, err := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
			if err != nil {
				return nil, fmt.Errorf("invalid start number in range %s: %w", part, err)
			}

			end, err := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid end number in range %s: %w", part, err)
			}

			if start < 1 || end > maxIndex || start > end {
				return nil, fmt.Errorf("invalid range %s: must be between 1 and %d", part, maxIndex)
			}

			for i := start; i <= end; i++ {
				indices = append(indices, i-1)
			}
		} else {
			num, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid number: %s", part)
			}

			if num < 1 || num > maxIndex {
				return nil, fmt.Errorf("number %d out of range: must be between 1 and %d", num, maxIndex)
			}

			indices = append(indices, num-1)
		}
	}

	uniqueIndices := make([]int, 0, len(indices))
	seen := make(map[int]bool)
	for _, index := range indices {
		if !seen[index] {
			uniqueIndices = append(uniqueIndices, index)
			seen[index] = true
		}
	}

	return uniqueIndices, nil
}
