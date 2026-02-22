package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"coragent/internal/agent"
	"coragent/internal/auth"
	"coragent/internal/diff"
	"coragent/internal/grant"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func newPlanCmd(opts *RootOptions) *cobra.Command {
	var recursive bool
	cmd := &cobra.Command{
		Use:   "plan [path]",
		Short: "Show execution plan without applying changes",
		Example: `  # Plan current directory
  coragent plan

  # Plan a single file
  coragent plan agent.yaml

  # Plan all agents in a directory tree
  coragent plan -R ./agents/`,
		Args: cobra.MaximumNArgs(1),
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

			planItems, err := buildPlanItems(context.Background(), specs, opts, cfg, client, client)
			if err != nil {
				return err
			}

			var createCount, updateCount, noChangeCount int
			for _, pi := range planItems {
				fmt.Fprintf(os.Stdout, "%s:\n", pi.Parsed.Spec.Name)
				fmt.Fprintf(os.Stdout, "  database: %s\n", pi.Target.Database)
				fmt.Fprintf(os.Stdout, "  schema:   %s\n", pi.Target.Schema)

				if !pi.Exists {
					createCount++
					color.New(color.FgGreen).Fprintln(os.Stdout, "  + create")
					createChanges, err := diff.DiffForCreate(pi.Parsed.Spec)
					if err != nil {
						return fmt.Errorf("%s: %w", pi.Parsed.Path, err)
					}
					for _, c := range createChanges {
						fmt.Fprintf(os.Stdout, "    %s %s: %s\n",
							color.New(color.FgGreen).Sprint("+"),
							c.Path,
							formatValue(c.Before),
						)
					}
					showGrantPlan(pi.GrantDiff)
					continue
				}

				if !diff.HasChanges(pi.Changes) && !pi.GrantDiff.HasChanges() {
					noChangeCount++
					color.New(color.FgCyan).Fprintln(os.Stdout, "  = no changes")
					continue
				}

				updateCount++
				for _, c := range pi.Changes {
					fmt.Fprintf(os.Stdout, "  %s %s: %s -> %s\n",
						changeSymbol(c.Type),
						c.Path,
						formatValue(c.Before),
						formatValue(c.After),
					)
				}
				showGrantPlan(pi.GrantDiff)
			}

			fmt.Fprintf(os.Stdout, "\nPlan: %d to create, %d to update, %d unchanged\n", createCount, updateCount, noChangeCount)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&recursive, "recursive", "R", false, "Recursively load agents from subdirectories")
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
		cfg.Account = strings.ToUpper(strings.TrimSpace(opts.Account))
	}
	if strings.TrimSpace(opts.Role) != "" {
		cfg.Role = strings.ToUpper(strings.TrimSpace(opts.Role))
	}
	if strings.TrimSpace(opts.Database) != "" {
		cfg.Database = strings.TrimSpace(opts.Database)
	}
	if strings.TrimSpace(opts.Schema) != "" {
		cfg.Schema = strings.TrimSpace(opts.Schema)
	}
}

// showGrantPlan displays the grant diff in plan output.
func showGrantPlan(diff grant.GrantDiff) {
	if !diff.HasChanges() {
		return
	}

	fmt.Fprintf(os.Stdout, "  grants:\n")

	for _, e := range diff.ToRevoke {
		fmt.Fprintf(os.Stdout, "    %s %s TO %s %s\n",
			color.New(color.FgRed).Sprint("-"),
			e.Privilege,
			e.RoleType,
			e.RoleName)
	}

	for _, e := range diff.ToGrant {
		fmt.Fprintf(os.Stdout, "    %s %s TO %s %s\n",
			color.New(color.FgGreen).Sprint("+"),
			e.Privilege,
			e.RoleType,
			e.RoleName)
	}
}
