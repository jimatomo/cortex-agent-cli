package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"coragent/internal/agent"
	"coragent/internal/api"
	"coragent/internal/auth"
	"coragent/internal/diff"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func newPlanCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan [path]",
		Short: "Show execution plan without applying changes",
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

			cfg := auth.FromEnv()
			applyAuthOverrides(&cfg, opts)

			client, err := api.NewClientWithDebug(cfg, opts.Debug)
			if err != nil {
				return err
			}

			var createCount, updateCount, noChangeCount int
			for _, item := range specs {
				target, err := ResolveTarget(item.Spec, opts)
				if err != nil {
					return fmt.Errorf("%s: %w", item.Path, err)
				}

				remote, exists, err := client.GetAgent(context.Background(), target.Database, target.Schema, item.Spec.Name)
				if err != nil {
					return fmt.Errorf("snowflake API error: %w", err)
				}

				fmt.Fprintf(os.Stdout, "%s:\n", item.Spec.Name)
				fmt.Fprintf(os.Stdout, "  database: %s\n", target.Database)
				fmt.Fprintf(os.Stdout, "  schema:   %s\n", target.Schema)
				if !exists {
					createCount++
					color.New(color.FgGreen).Fprintln(os.Stdout, "  + create")
					// Show what will be created
					createChanges, err := diff.DiffForCreate(item.Spec)
					if err != nil {
						return fmt.Errorf("%s: %w", item.Path, err)
					}
					for _, c := range createChanges {
						fmt.Fprintf(os.Stdout, "    %s %s: %s\n",
							color.New(color.FgGreen).Sprint("+"),
							c.Path,
							formatValue(c.Before),
						)
					}
					continue
				}

				changes, err := diff.Diff(item.Spec, remote)
				if err != nil {
					return fmt.Errorf("%s: %w", item.Path, err)
				}
				if !diff.HasChanges(changes) {
					noChangeCount++
					color.New(color.FgCyan).Fprintln(os.Stdout, "  = no changes")
					continue
				}

				updateCount++
				for _, c := range changes {
					fmt.Fprintf(os.Stdout, "  %s %s: %s -> %s\n",
						changeSymbol(c.Type),
						c.Path,
						formatValue(c.After),
						formatValue(c.Before),
					)
				}
			}

			fmt.Fprintf(os.Stdout, "\nPlan: %d to create, %d to update, %d unchanged\n", createCount, updateCount, noChangeCount)
			return nil
		},
	}
	return cmd
}

func changeSymbol(t diff.ChangeType) string {
	switch t {
	case diff.Added:
		return color.New(color.FgGreen).Sprint("+")
	case diff.Removed:
		return color.New(color.FgRed).Sprint("-")
	default:
		return color.New(color.FgYellow).Sprint("~")
	}
}

func formatValue(v any) string {
	switch val := v.(type) {
	case nil:
		return "null"
	case string:
		if val == "" {
			return "\"\""
		}
		if len(val) > 80 {
			return fmt.Sprintf("%q...", val[:77])
		}
		return fmt.Sprintf("%q", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func applyAuthOverrides(cfg *auth.Config, opts *RootOptions) {
	if strings.TrimSpace(opts.Account) != "" {
		cfg.Account = strings.TrimSpace(opts.Account)
	}
	if strings.TrimSpace(opts.Role) != "" {
		cfg.Role = strings.TrimSpace(opts.Role)
	}
}
