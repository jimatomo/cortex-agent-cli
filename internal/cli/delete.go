package cli

import (
	"context"
	"fmt"
	"os"

	"coragent/internal/agent"
	"coragent/internal/diff"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

type deleteItem struct {
	Parsed  agent.ParsedAgent
	Target  Target
	Exists  bool
	Changes []diff.Change
}

func newDeleteCmd(opts *RootOptions) *cobra.Command {
	var autoApprove bool
	var recursive bool
	cmd := &cobra.Command{
		Use:   "delete [path]",
		Short: "Delete agents defined in YAML files",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}

			specs, err := agent.LoadAgents(path, recursive, opts.Env)
			if err != nil {
				return err
			}

			client, cfg, err := buildClientAndCfg(opts)
			if err != nil {
				return err
			}

			var planItems []deleteItem
			var deleteCount, notFoundCount int
			for _, item := range specs {
				target, err := ResolveTarget(item.Spec, opts, cfg)
				if err != nil {
					return fmt.Errorf("%s: %w", item.Path, err)
				}

				_, exists, err := client.GetAgent(context.Background(), target.Database, target.Schema, item.Spec.Name)
				if err != nil {
					return fmt.Errorf("snowflake API error: %w", err)
				}

				if !exists {
					notFoundCount++
					planItems = append(planItems, deleteItem{Parsed: item, Target: target, Exists: false})
					continue
				}

				deleteCount++
				changes, err := diff.DiffForCreate(item.Spec)
				if err != nil {
					return fmt.Errorf("%s: %w", item.Path, err)
				}
				planItems = append(planItems, deleteItem{Parsed: item, Target: target, Exists: true, Changes: changes})
			}

			// Show detailed plan output
			for _, item := range planItems {
				fmt.Fprintf(os.Stdout, "%s:\n", item.Parsed.Spec.Name)
				fmt.Fprintf(os.Stdout, "  database: %s\n", item.Target.Database)
				fmt.Fprintf(os.Stdout, "  schema:   %s\n", item.Target.Schema)
				if !item.Exists {
					color.New(color.FgYellow).Fprintln(os.Stdout, "  ! not found (skipped)")
					continue
				}
				color.New(color.FgRed).Fprintln(os.Stdout, "  - delete")
				for _, c := range item.Changes {
					fmt.Fprintf(os.Stdout, "    %s %s: %s\n",
						color.New(color.FgRed).Sprint("-"),
						c.Path,
						formatValue(c.Before),
					)
				}
			}

			fmt.Fprintf(os.Stdout, "\nPlan: %d to delete, %d not found (skipped)\n", deleteCount, notFoundCount)
			if deleteCount == 0 {
				return nil
			}

			if !autoApprove {
				if !confirm("Delete these agents?") {
					fmt.Fprintln(os.Stdout, "Aborted.")
					return nil
				}
			}

			for _, item := range planItems {
				if !item.Exists {
					continue
				}

				fmt.Fprintf(os.Stdout, "Deleting %s... ", item.Parsed.Spec.Name)
				if err := client.DeleteAgent(context.Background(), item.Target.Database, item.Target.Schema, item.Parsed.Spec.Name); err != nil {
					fmt.Fprintln(os.Stdout, "failed")
					return fmt.Errorf("snowflake API error: %w", err)
				}
				color.New(color.FgGreen).Fprintln(os.Stdout, "done")
			}

			return nil
		},
	}
	cmd.Flags().BoolVarP(&autoApprove, "yes", "y", false, "Skip confirmation prompt")
	cmd.Flags().BoolVarP(&recursive, "recursive", "R", false, "Recursively load agents from subdirectories")
	return cmd
}

