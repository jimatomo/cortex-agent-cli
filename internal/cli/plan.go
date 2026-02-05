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
	"coragent/internal/grant"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func newPlanCmd(opts *RootOptions) *cobra.Command {
	var recursive bool
	cmd := &cobra.Command{
		Use:   "plan [path]",
		Short: "Show execution plan without applying changes",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}

			specs, err := agent.LoadAgents(path, recursive)
			if err != nil {
				return err
			}

			cfg := auth.LoadConfig(opts.Connection)
			applyAuthOverrides(&cfg, opts)

			client, err := api.NewClientWithDebug(cfg, opts.Debug)
			if err != nil {
				return err
			}

			var createCount, updateCount, noChangeCount int
			for _, item := range specs {
				target, err := ResolveTarget(item.Spec, opts, cfg)
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

				// Get desired grant state from YAML
				var grantCfg *agent.GrantConfig
				if item.Spec.Deploy != nil {
					grantCfg = item.Spec.Deploy.Grant
				}
				desiredGrants := grant.FromGrantConfig(grantCfg)

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
					// Show grants for new agent (all are additions)
					grantDiff := grant.ComputeDiff(desiredGrants, grant.GrantState{})
					showGrantPlan(grantDiff)
					continue
				}

				// Get current grant state from Snowflake
				grantRows, err := client.ShowGrants(context.Background(), target.Database, target.Schema, item.Spec.Name)
				if err != nil {
					return fmt.Errorf("show grants: %w", err)
				}
				currentGrants := grant.FromShowGrantsRows(planToGrantRows(grantRows))
				grantDiff := grant.ComputeDiff(desiredGrants, currentGrants)

				changes, err := diff.Diff(item.Spec, remote)
				if err != nil {
					return fmt.Errorf("%s: %w", item.Path, err)
				}
				if !diff.HasChanges(changes) && !grantDiff.HasChanges() {
					noChangeCount++
					color.New(color.FgCyan).Fprintln(os.Stdout, "  = no changes")
					continue
				}

				// Count as update if there are agent spec changes or grant changes
				updateCount++
				for _, c := range changes {
					fmt.Fprintf(os.Stdout, "  %s %s: %s -> %s\n",
						changeSymbol(c.Type),
						c.Path,
						formatValue(c.Before),
						formatValue(c.After),
					)
				}
				showGrantPlan(grantDiff)
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

// planToGrantRows converts api.ShowGrantsRow to grant.ShowGrantsRow
func planToGrantRows(rows []api.ShowGrantsRow) []grant.ShowGrantsRow {
	result := make([]grant.ShowGrantsRow, len(rows))
	for i, r := range rows {
		result[i] = grant.ShowGrantsRow{
			Privilege:   r.Privilege,
			GrantedTo:   r.GrantedTo,
			GranteeName: r.GranteeName,
		}
	}
	return result
}
