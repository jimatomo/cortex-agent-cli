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

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

type applyItem struct {
	Parsed  agent.ParsedAgent
	Target  Target
	Exists  bool
	Changes []diff.Change
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

			cfg := auth.FromEnv()
			applyAuthOverrides(&cfg, opts)

			client, err := api.NewClientWithDebug(cfg, opts.Debug)
			if err != nil {
				return err
			}

			var planItems []applyItem
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

				if !exists {
					createCount++
					planItems = append(planItems, applyItem{Parsed: item, Target: target, Exists: false})
					continue
				}

				changes, err := diff.Diff(item.Spec, remote)
				if err != nil {
					return fmt.Errorf("%s: %w", item.Path, err)
				}
				if !diff.HasChanges(changes) {
					noChangeCount++
					planItems = append(planItems, applyItem{Parsed: item, Target: target, Exists: true})
					continue
				}

				updateCount++
				planItems = append(planItems, applyItem{Parsed: item, Target: target, Exists: true, Changes: changes})
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
							formatValue(c.Before),
						)
					}
					continue
				}
				if !diff.HasChanges(item.Changes) {
					color.New(color.FgCyan).Fprintln(os.Stdout, "  = no changes")
					continue
				}
				for _, c := range item.Changes {
					fmt.Fprintf(os.Stdout, "  %s %s: %s -> %s\n",
						changeSymbol(c.Type),
						c.Path,
						formatValue(c.After),
						formatValue(c.Before),
					)
				}
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
					continue
				}

				if !diff.HasChanges(item.Changes) {
					color.New(color.FgCyan).Fprintf(os.Stdout, "No changes for %s\n", item.Parsed.Spec.Name)
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
			}

			return nil
		},
	}
	cmd.Flags().BoolVar(&autoApprove, "yes", false, "Skip confirmation prompt")
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
			payload[key] = nil
		}
	}
	return payload, nil
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
