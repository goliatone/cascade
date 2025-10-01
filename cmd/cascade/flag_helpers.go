package main

import "github.com/spf13/cobra"

// addConfirmationFlags wires shared confirmation and overwrite behaviour flags to the command.
func addConfirmationFlags(cmd *cobra.Command, req *manifestGenerateRequest) {
	cmd.Flags().BoolVar(&req.Force, "force", false, "Overwrite existing manifest without prompting")
	cmd.Flags().BoolVar(&req.Yes, "yes", false, "Automatically confirm all prompts")
	cmd.Flags().BoolVar(&req.NonInteractive, "non-interactive", false, "Run in non-interactive mode (same as --yes)")
}

// addWorkspaceDiscoveryFlags wires workspace discovery related flags.
func addWorkspaceDiscoveryFlags(cmd *cobra.Command, req *manifestGenerateRequest) {
	cmd.Flags().StringVar(&req.Workspace, "workspace", "", "Workspace directory to scan for dependents (default: auto-detected from module location)")
	cmd.Flags().IntVar(&req.MaxDepth, "max-depth", 0, "Maximum depth to scan in workspace directory (0 = no limit)")
	cmd.Flags().StringSliceVar(&req.IncludePatterns, "include", []string{}, "Directory patterns to include during discovery")
	cmd.Flags().StringSliceVar(&req.ExcludePatterns, "exclude", []string{}, "Directory patterns to exclude during discovery (e.g., vendor, .git)")
}

// addGitHubDiscoveryFlags wires GitHub discovery controls shared across commands.
func addGitHubDiscoveryFlags(cmd *cobra.Command, req *manifestGenerateRequest) {
	cmd.Flags().StringVar(&req.GitHubOrg, "github-org", "", "GitHub organization to search for dependent repositories (auto-detected from module path if not provided)")
	cmd.Flags().StringSliceVar(&req.GitHubInclude, "github-include", []string{}, "Repository name patterns to include during GitHub discovery")
	cmd.Flags().StringSliceVar(&req.GitHubExclude, "github-exclude", []string{}, "Repository name patterns to exclude during GitHub discovery")
}
