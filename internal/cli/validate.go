package cli

import (
	"fmt"

	"coragent/internal/agent"

	"github.com/spf13/cobra"
)

func newValidateCmd(opts *RootOptions) *cobra.Command {
	var recursive bool
	cmd := &cobra.Command{
		Use:   "validate [path]",
		Short: "Validate YAML files without applying",
		Example: `  # Validate current directory
  coragent validate

  # Validate a single file
  coragent validate agent.yaml

  # Validate all agents in a directory tree
  coragent validate -R ./agents/`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}

			specs, err := agent.LoadAgents(path, recursive, opts.Env)
			if err != nil {
				return UserErr(err)
			}

			for _, item := range specs {
				fmt.Fprintf(cmd.OutOrStdout(), "ok: %s\n", item.Path)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&recursive, "recursive", "R", false, "Recursively load agents from subdirectories")
	return cmd
}
