package cli

import (
	"fmt"

	"coragent/internal/agent"

	"github.com/spf13/cobra"
)

func newValidateCmd(_ *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate [path]",
		Short: "Validate YAML files without applying",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}

			specs, err := agent.LoadAgents(path)
			if err != nil {
				return err
			}

			for _, item := range specs {
				fmt.Fprintf(cmd.OutOrStdout(), "ok: %s\n", item.Path)
			}
			return nil
		},
	}
	return cmd
}
