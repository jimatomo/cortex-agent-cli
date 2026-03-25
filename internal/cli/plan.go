package cli

import (
	"fmt"
	"os"
	"slices"
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

type renderedChangeLine struct {
	Type      diff.ChangeType
	Text      string
	IsContext bool
	IsDivider bool
}

func formatChange(c diff.Change) []renderedChangeLine {
	switch c.Type {
	case diff.Added:
		return []renderedChangeLine{{Type: diff.Added, Text: formatValue(c.After)}}
	case diff.Removed:
		return []renderedChangeLine{{Type: diff.Removed, Text: formatValue(c.Before)}}
	default:
		return formatModifiedChange(c.Before, c.After)
	}
}

func formatModifiedChange(before, after any) []renderedChangeLine {
	beforeStr, beforeOK := before.(string)
	afterStr, afterOK := after.(string)
	if beforeOK && afterOK && (strings.Contains(beforeStr, "\n") || strings.Contains(afterStr, "\n")) {
		return diffStringLines(beforeStr, afterStr, 1)
	}

	return []renderedChangeLine{
		{Type: diff.Removed, Text: formatValue(before)},
		{Type: diff.Added, Text: formatValue(after)},
	}
}

func diffStringLines(before, after string, contextLines int) []renderedChangeLine {
	lines := buildDiffLines(strings.Split(before, "\n"), strings.Split(after, "\n"))
	return collapseDiffContext(lines, contextLines)
}

func buildDiffLines(before, after []string) []renderedChangeLine {
	dp := make([][]int, len(before)+1)
	for i := range dp {
		dp[i] = make([]int, len(after)+1)
	}

	for i := len(before) - 1; i >= 0; i-- {
		for j := len(after) - 1; j >= 0; j-- {
			if before[i] == after[j] {
				dp[i][j] = dp[i+1][j+1] + 1
				continue
			}
			dp[i][j] = max(dp[i+1][j], dp[i][j+1])
		}
	}

	lines := make([]renderedChangeLine, 0, len(before)+len(after))
	for i, j := 0, 0; i < len(before) || j < len(after); {
		switch {
		case i < len(before) && j < len(after) && before[i] == after[j]:
			lines = append(lines, renderedChangeLine{Text: before[i], IsContext: true})
			i++
			j++
		case j == len(after) || (i < len(before) && dp[i+1][j] >= dp[i][j+1]):
			lines = append(lines, renderedChangeLine{Type: diff.Removed, Text: before[i]})
			i++
		default:
			lines = append(lines, renderedChangeLine{Type: diff.Added, Text: after[j]})
			j++
		}
	}

	return slices.Clip(lines)
}

func collapseDiffContext(lines []renderedChangeLine, contextLines int) []renderedChangeLine {
	if len(lines) == 0 {
		return nil
	}

	include := make([]bool, len(lines))
	for i, line := range lines {
		if line.IsContext {
			continue
		}
		start := max(0, i-contextLines)
		end := min(len(lines)-1, i+contextLines)
		for j := start; j <= end; j++ {
			include[j] = true
		}
	}

	collapsed := make([]renderedChangeLine, 0, len(lines))
	prevIncluded := false
	for i, line := range lines {
		if !include[i] {
			prevIncluded = false
			continue
		}
		if len(collapsed) > 0 && !prevIncluded {
			collapsed = append(collapsed, renderedChangeLine{IsDivider: true, Text: "..."})
		}
		collapsed = append(collapsed, line)
		prevIncluded = true
	}

	if len(collapsed) > 0 && collapsed[0].IsDivider {
		collapsed = collapsed[1:]
	}
	return slices.Clip(collapsed)
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
