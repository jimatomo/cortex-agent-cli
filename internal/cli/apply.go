package cli

import (
	"bufio"
	"context"
	"encoding/json"
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

type applyItem struct {
	Parsed    agent.ParsedAgent
	Target    Target
	Exists    bool
	Changes   []diff.Change
	GrantDiff grant.GrantDiff
}

func newApplyCmd(opts *RootOptions) *cobra.Command {
	var autoApprove bool
	var recursive bool
	cmd := &cobra.Command{
		Use:   "apply [path]",
		Short: "Apply agent changes",
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

			var planItems []applyItem
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

				// Get desired grant state from YAML
				var grantCfg *agent.GrantConfig
				if item.Spec.Deploy != nil {
					grantCfg = item.Spec.Deploy.Grant
				}
				desiredGrants := grant.FromGrantConfig(grantCfg)

				if !exists {
					createCount++
					grantDiff := grant.ComputeDiff(desiredGrants, grant.GrantState{})
					planItems = append(planItems, applyItem{Parsed: item, Target: target, Exists: false, GrantDiff: grantDiff})
					continue
				}

				// Get current grant state from Snowflake
				grantRows, err := client.ShowGrants(context.Background(), target.Database, target.Schema, item.Spec.Name)
				if err != nil {
					return fmt.Errorf("show grants: %w", err)
				}
				currentGrants := grant.FromShowGrantsRows(toGrantRows(grantRows))
				grantDiff := grant.ComputeDiff(desiredGrants, currentGrants)

				changes, err := diff.Diff(item.Spec, remote)
				if err != nil {
					return fmt.Errorf("%s: %w", item.Path, err)
				}

				// Count as no change only if both spec and grants have no changes
				if !diff.HasChanges(changes) && !grantDiff.HasChanges() {
					noChangeCount++
					planItems = append(planItems, applyItem{Parsed: item, Target: target, Exists: true, GrantDiff: grantDiff})
					continue
				}

				updateCount++
				planItems = append(planItems, applyItem{Parsed: item, Target: target, Exists: true, Changes: changes, GrantDiff: grantDiff})
			}

			// Show detailed plan output
			for _, item := range planItems {
				fmt.Fprintf(os.Stdout, "%s:\n", item.Parsed.Spec.Name)
				fmt.Fprintf(os.Stdout, "  database: %s\n", item.Target.Database)
				fmt.Fprintf(os.Stdout, "  schema:   %s\n", item.Target.Schema)

				if !item.Exists {
					color.New(color.FgGreen).Fprintln(os.Stdout, "  + create")
					// Show what will be created
					createChanges, err := diff.DiffForCreate(item.Parsed.Spec)
					if err != nil {
						return fmt.Errorf("%s: %w", item.Parsed.Path, err)
					}
					for _, c := range createChanges {
						fmt.Fprintf(os.Stdout, "    %s %s: %s\n",
							color.New(color.FgGreen).Sprint("+"),
							c.Path,
							formatValue(c.After),
						)
					}
					showApplyGrantPlan(item.GrantDiff)
					continue
				}

				if !diff.HasChanges(item.Changes) && !item.GrantDiff.HasChanges() {
					color.New(color.FgCyan).Fprintln(os.Stdout, "  = no changes")
					continue
				}
				for _, c := range item.Changes {
					fmt.Fprintf(os.Stdout, "  %s %s: %s -> %s\n",
						changeSymbol(c.Type),
						c.Path,
						formatValue(c.Before),
						formatValue(c.After),
					)
				}
				showApplyGrantPlan(item.GrantDiff)
			}

			fmt.Fprintf(os.Stdout, "\nPlan: %d to create, %d to update, %d unchanged\n", createCount, updateCount, noChangeCount)
			if createCount+updateCount == 0 {
				return nil
			}

			if !autoApprove {
				if !confirmApply() {
					fmt.Fprintln(os.Stdout, "Aborted.")
					return nil
				}
			}

			for _, item := range planItems {
				if !item.Exists {
					color.New(color.FgGreen).Fprintf(os.Stdout, "Creating %s...\n", item.Parsed.Spec.Name)
					if err := client.CreateAgent(context.Background(), item.Target.Database, item.Target.Schema, item.Parsed.Spec); err != nil {
						return fmt.Errorf("snowflake API error: %w", err)
					}
					// Apply grants after creating the agent
					if err := applyGrants(context.Background(), client, item.Target, item.Parsed.Spec, true); err != nil {
						return fmt.Errorf("grant error: %w", err)
					}
					continue
				}

				if !diff.HasChanges(item.Changes) {
					color.New(color.FgCyan).Fprintf(os.Stdout, "No changes for %s\n", item.Parsed.Spec.Name)
					// Still apply grants even if agent has no changes
					if err := applyGrants(context.Background(), client, item.Target, item.Parsed.Spec, true); err != nil {
						return fmt.Errorf("grant error: %w", err)
					}
					continue
				}

				color.New(color.FgYellow).Fprintf(os.Stdout, "Updating %s...\n", item.Parsed.Spec.Name)
				payload, err := updatePayload(item.Parsed.Spec, item.Changes)
				if err != nil {
					return fmt.Errorf("%s: %w", item.Parsed.Path, err)
				}
				if err := client.UpdateAgent(context.Background(), item.Target.Database, item.Target.Schema, item.Parsed.Spec.Name, payload); err != nil {
					return fmt.Errorf("snowflake API error: %w", err)
				}
				// Apply grants after updating the agent
				if err := applyGrants(context.Background(), client, item.Target, item.Parsed.Spec, true); err != nil {
					return fmt.Errorf("grant error: %w", err)
				}
			}

			return nil
		},
	}
	cmd.Flags().BoolVarP(&autoApprove, "yes", "y", false, "Skip confirmation prompt")
	cmd.Flags().BoolVarP(&recursive, "recursive", "R", false, "Recursively load agents from subdirectories")
	return cmd
}

func confirmApply() bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprint(os.Stdout, "Apply these changes? [y/N]: ")
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes"
}

func updatePayload(spec agent.AgentSpec, changes []diff.Change) (map[string]any, error) {
	data, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("marshal spec: %w", err)
	}
	var local map[string]any
	if err := json.Unmarshal(data, &local); err != nil {
		return nil, fmt.Errorf("unmarshal spec: %w", err)
	}

	payload := make(map[string]any)
	for _, change := range changes {
		key := topLevel(change.Path)
		if key == "" {
			continue
		}
		if val, ok := local[key]; ok {
			payload[key] = val
		} else {
			// Use empty values instead of null for deletion
			// Snowflake API may ignore null values
			payload[key] = emptyValueForKey(key)
		}
	}
	return payload, nil
}

// emptyValueForKey returns the appropriate empty value for a given field.
// Some fields require empty arrays, others require empty objects.
func emptyValueForKey(key string) any {
	switch key {
	case "tools":
		return []any{}
	case "tool_resources":
		return map[string]any{}
	default:
		return nil
	}
}

func topLevel(path string) string {
	if path == "" {
		return ""
	}
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return ""
	}
	return strings.Split(parts[0], "[")[0]
}

// applyGrants computes grant diff and executes necessary GRANT/REVOKE statements.
func applyGrants(ctx context.Context, client *api.Client, target Target, spec agent.AgentSpec, agentExists bool) error {
	// Get desired state from YAML
	var grantCfg *agent.GrantConfig
	if spec.Deploy != nil {
		grantCfg = spec.Deploy.Grant
	}
	desired := grant.FromGrantConfig(grantCfg)

	// Get current state from Snowflake (only if agent exists)
	var current grant.GrantState
	if agentExists {
		rows, err := client.ShowGrants(ctx, target.Database, target.Schema, spec.Name)
		if err != nil {
			return fmt.Errorf("show grants: %w", err)
		}
		current = grant.FromShowGrantsRows(toGrantRows(rows))
	}

	// Compute diff
	diff := grant.ComputeDiff(desired, current)

	if !diff.HasChanges() {
		return nil
	}

	var grantErrors []string

	// Execute REVOKEs first (remove old permissions before adding new)
	for _, e := range diff.ToRevoke {
		if err := client.ExecuteRevoke(ctx, target.Database, target.Schema,
			spec.Name, e.RoleType, e.RoleName, e.Privilege); err != nil {
			grantErrors = append(grantErrors,
				fmt.Sprintf("REVOKE %s FROM %s %s: %v", e.Privilege, e.RoleType, e.RoleName, err))
		} else {
			color.New(color.FgRed).Fprintf(os.Stdout,
				"  Revoked %s from %s %s\n", e.Privilege, e.RoleType, e.RoleName)
		}
	}

	// Execute GRANTs
	for _, e := range diff.ToGrant {
		if err := client.ExecuteGrant(ctx, target.Database, target.Schema,
			spec.Name, e.RoleType, e.RoleName, e.Privilege); err != nil {
			grantErrors = append(grantErrors,
				fmt.Sprintf("GRANT %s TO %s %s: %v", e.Privilege, e.RoleType, e.RoleName, err))
		} else {
			color.New(color.FgGreen).Fprintf(os.Stdout,
				"  Granted %s to %s %s\n", e.Privilege, e.RoleType, e.RoleName)
		}
	}

	if len(grantErrors) > 0 {
		return fmt.Errorf("grant/revoke errors:\n  %s", strings.Join(grantErrors, "\n  "))
	}
	return nil
}

// toGrantRows converts api.ShowGrantsRow to grant.ShowGrantsRow
func toGrantRows(rows []api.ShowGrantsRow) []grant.ShowGrantsRow {
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

// showApplyGrantPlan displays the grant diff in apply plan output.
func showApplyGrantPlan(diff grant.GrantDiff) {
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
