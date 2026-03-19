package cli

import (
	"fmt"
	"os"
	"strings"

	"coragent/internal/agent"
	"coragent/internal/auth"
	"coragent/internal/diff"

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
				return UserErr(err)
			}

			client, cfg, err := buildClientAndCfg(opts)
			if err != nil {
				return err
			}

			planItems, err := buildPlanItems(commandContext("plan"), specs, opts, cfg, client, client)
			if err != nil {
				return err
			}

			_, err = writePlanPreview(os.Stdout, planItems)
			return err
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

func formatChange(c diff.Change) string {
	switch c.Type {
	case diff.Added:
		return formatValue(c.After)
	case diff.Removed:
		return formatValue(c.Before)
	default: // Modified
		return fmt.Sprintf("%s %s %s", formatValue(c.Before), color.New(color.FgYellow).Sprint("->"), formatValue(c.After))
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
