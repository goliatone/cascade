package main

import "github.com/spf13/cobra"

func newUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Apply dependency updates",
		Long:  "Apply dependency updates for supported local workflows.",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = cmd.Help()
			return newValidationError("update requires a subcommand", nil)
		},
	}
	cmd.AddCommand(newUpdateLocalCommand())
	return cmd
}

func newUpdateLocalCommand() *cobra.Command {
	opts := localCommandOptions{}
	cmd := &cobra.Command{
		Use:   "local",
		Short: "Update local sibling dependencies",
		Long: `Update local Go dependencies across every module in the current
repository. Cascade uses go.work when present and discovers repository go.mod
files otherwise, then runs go get in each module with outdated local candidates.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdateLocal(cmd, opts)
		},
	}
	addLocalPlanFlags(cmd, &opts)
	cmd.Flags().BoolVar(&opts.NoTidy, "no-tidy", false, "Do not run go mod tidy after successful updates")
	cmd.Flags().BoolVar(&opts.NoHooks, "no-hooks", false, "Do not run configured local update hooks")
	return cmd
}
