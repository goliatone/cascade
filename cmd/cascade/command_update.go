package main

import "github.com/spf13/cobra"

func newUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Apply dependency updates",
		Long:  "Apply dependency updates for supported local workflows.",
	}
	cmd.AddCommand(newUpdateLocalCommand())
	return cmd
}

func newUpdateLocalCommand() *cobra.Command {
	opts := localCommandOptions{}
	cmd := &cobra.Command{
		Use:   "local",
		Short: "Update local sibling dependencies",
		Long: `Update local Go dependencies by comparing the current module's direct
dependencies against sibling repositories in the local workspace and running
go get for outdated local candidates.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdateLocal(cmd, opts)
		},
	}
	addLocalPlanFlags(cmd, &opts)
	cmd.Flags().BoolVar(&opts.NoTidy, "no-tidy", false, "Do not run go mod tidy after successful updates")
	return cmd
}
