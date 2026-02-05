package cli

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/spf13/cobra"
)

type RootOptions struct {
	Account    string
	Database   string
	Schema     string
	Role       string
	Connection string
	Debug      bool
}

var DebugEnabled bool

func NewRootCmd() *cobra.Command {
	opts := &RootOptions{}
	cmd := &cobra.Command{
		Use:           "coragent",
		Short:         "CLI for managing Snowflake Cortex Agents",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			DebugEnabled = opts.Debug
		},
	}

	cmd.PersistentFlags().StringVarP(&opts.Account, "account", "a", "", "Snowflake account identifier")
	cmd.PersistentFlags().StringVarP(&opts.Database, "database", "d", "", "Target database")
	cmd.PersistentFlags().StringVarP(&opts.Schema, "schema", "s", "", "Target schema")
	cmd.PersistentFlags().StringVarP(&opts.Role, "role", "r", "", "Snowflake role to use (e.g., CORTEX_USER)")
	cmd.PersistentFlags().StringVarP(&opts.Connection, "connection", "c", "", "Snowflake CLI connection name (from ~/.snowflake/config.toml)")
	cmd.PersistentFlags().BoolVar(&opts.Debug, "debug", false, "Enable debug logging with trace output")

	cmd.AddCommand(
		newPlanCmd(opts),
		newApplyCmd(opts),
		newDeleteCmd(opts),
		newValidateCmd(opts),
		newExportCmd(opts),
		newRunCmd(opts),
		newThreadsCmd(opts),
		newEvalCmd(opts),
		newLoginCmd(opts),
		newLogoutCmd(opts),
		newAuthCmd(opts),
	)

	return cmd
}

func Execute() {
	root := NewRootCmd()
	if err := root.Execute(); err != nil {
		if DebugEnabled {
			fmt.Fprintln(os.Stderr, "DEBUG STACK TRACE:")
			fmt.Fprintln(os.Stderr, string(debug.Stack()))
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
