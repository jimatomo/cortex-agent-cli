package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"coragent/internal/agent"
	"coragent/internal/config"
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
	var runEval bool
	cmd := &cobra.Command{
		Use:   "apply [path]",
		Short: "Apply agent changes",
		Example: `  # Apply current directory (with confirmation prompt)
  coragent apply

  # Apply a single file, skip confirmation
  coragent apply agent.yaml -y

  # Apply all agents recursively and run eval tests after
  coragent apply -R ./agents/ --eval`,
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
				currentGrants := grant.FromShowGrantsRows(convertGrantRows(grantRows))
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
				if !confirm("Apply these changes?", cmd.InOrStdin()) {
					fmt.Fprintln(os.Stdout, "Aborted.")
					return nil
				}
			}

			for _, item := range planItems {
				if !item.Exists {
					color.New(color.FgGreen).Fprintf(os.Stdout, "Creating %s...\n", item.Parsed.Spec.Name)
				} else if diff.HasChanges(item.Changes) {
					color.New(color.FgYellow).Fprintf(os.Stdout, "Updating %s...\n", item.Parsed.Spec.Name)
				} else {
					color.New(color.FgCyan).Fprintf(os.Stdout, "No changes for %s\n", item.Parsed.Spec.Name)
				}
			}

			appliedItems, err := executeApply(context.Background(), planItems, client, client)
			if err != nil {
				return err
			}

			color.New(color.FgGreen).Fprintln(os.Stdout, "\nApply complete successfully!")

			if !runEval {
				return nil
			}

			// Filter applied agents that have eval tests
			var evalItems []applyItem
			for _, item := range appliedItems {
				if item.Parsed.Spec.Eval != nil && len(item.Parsed.Spec.Eval.Tests) > 0 {
					evalItems = append(evalItems, item)
				}
			}
			if len(evalItems) == 0 {
				fmt.Fprintln(os.Stdout, "No eval tests defined for changed agents.")
				return nil
			}

			// Resolve eval output directory
			appCfg := config.LoadCoragentConfig()
			outputDir := "."
			if appCfg.Eval.OutputDir != "" {
				outputDir = appCfg.Eval.OutputDir
			}
			if err := os.MkdirAll(outputDir, 0o755); err != nil {
				return fmt.Errorf("create eval output dir: %w", err)
			}

			// Run eval for each changed agent
			var evalErrors []string
			for _, item := range evalItems {
				specDir := filepath.Dir(item.Parsed.Path)
				eo := evalOptions{
					judgeModel:             resolveJudgeModel(item.Parsed.Spec, appCfg),
					responseScoreThreshold: resolveResponseScoreThreshold(item.Parsed.Spec, appCfg),
				}
				if err := runEvalForAgent(client, item.Target, item.Parsed.Spec, outputDir, specDir, appCfg.Eval.TimestampSuffix, eo); err != nil {
					evalErrors = append(evalErrors, fmt.Sprintf("%s: %v", item.Parsed.Spec.Name, err))
				}
			}
			if len(evalErrors) > 0 {
				return fmt.Errorf("apply succeeded but eval failed:\n  %s", strings.Join(evalErrors, "\n  "))
			}

			return nil
		},
	}
	cmd.Flags().BoolVarP(&autoApprove, "yes", "y", false, "Skip confirmation prompt")
	cmd.Flags().BoolVarP(&recursive, "recursive", "R", false, "Recursively load agents from subdirectories")
	cmd.Flags().BoolVar(&runEval, "eval", false, "Run eval tests for changed agents after apply")
	return cmd
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
