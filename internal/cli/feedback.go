package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"coragent/internal/api"
	"coragent/internal/auth"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func newFeedbackCmd(opts *RootOptions) *cobra.Command {
	var showAll bool
	var limit int
	var jsonOut bool
	var yes bool
	var includeChecked bool
	var noTools bool

	cmd := &cobra.Command{
		Use:   "feedback <agent-name>",
		Short: "Show user feedback for a Cortex Agent",
		Long: `Retrieve user feedback events for a Cortex Agent from observability data.

Fetched records are cached locally at ~/.coragent/feedback/<agent>.json.
Records are shown one at a time. After each record you are prompted to mark
it as checked; checked records are hidden on subsequent runs, letting you
work through feedback incrementally.

By default, only negative feedback is shown. Use --all to show all feedback.`,
		Example: `  # Show negative feedback (default)
  coragent feedback my-agent -d MY_DB -s MY_SCHEMA

  # Show all feedback
  coragent feedback my-agent --all

  # Auto-confirm marking each record as checked
  coragent feedback my-agent -y

  # Also show already-checked records
  coragent feedback my-agent --include-checked

  # JSON output
  coragent feedback my-agent --json | jq .`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			agentName := args[0]

			cfg := auth.LoadConfig(opts.Connection)
			applyAuthOverrides(&cfg, opts)

			client, err := api.NewClientWithDebug(cfg, opts.Debug)
			if err != nil {
				return err
			}

			target, err := ResolveTargetForExport(opts, cfg)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			// 1. Fetch fresh records from Snowflake.
			fresh, err := client.GetFeedback(ctx, target.Database, target.Schema, agentName)
			if err != nil {
				return err
			}

			// 2. Load cache → merge → save.
			cache, err := loadFeedbackCache(agentName)
			if err != nil {
				return fmt.Errorf("load feedback cache: %w", err)
			}
			cache.merge(fresh)
			if err := saveFeedbackCache(agentName, cache); err != nil {
				return fmt.Errorf("save feedback cache: %w", err)
			}

			// 3. Select records to display.
			var toShow []CachedFeedbackRecord
			if includeChecked {
				toShow = cache.Records
			} else {
				for _, r := range cache.Records {
					if !r.Checked {
						toShow = append(toShow, r)
					}
				}
			}

			// 4. Apply sentiment filter (--all) and --limit.
			if !showAll {
				var filtered []CachedFeedbackRecord
				for _, r := range toShow {
					if r.Sentiment == "negative" {
						filtered = append(filtered, r)
					}
				}
				toShow = filtered
			}
			if limit > 0 && len(toShow) > limit {
				toShow = toShow[:limit]
			}

			// 5. JSON output — no prompt.
			if jsonOut {
				data, err := json.MarshalIndent(toShow, "", "  ")
				if err != nil {
					return fmt.Errorf("marshal JSON: %w", err)
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return err
			}

			// 6. Header.
			filter := "negative only"
			if showAll {
				filter = "all"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Feedback for agent %q (%s):\n\n", agentName, filter)

			if len(toShow) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No feedback found for agent %q.\n", agentName)
				return nil
			}

			allUnknown := true
			for _, r := range toShow {
				if r.Sentiment != "unknown" {
					allUnknown = false
					break
				}
			}
			if allUnknown {
				fmt.Fprintln(os.Stderr, "Warning: sentiment could not be determined from RECORD data; showing raw JSON.")
			}

			// 7. Show each record one at a time, prompt after each unchecked one.
			scanner := bufio.NewScanner(os.Stdin)
			markedCount := 0

			for i, r := range toShow {
				printOneRecord(cmd, i+1, len(toShow), r, includeChecked, noTools)

				// Already-checked records (only visible with --include-checked) skip the prompt.
				if r.Checked {
					continue
				}

				confirm := yes
				if !yes {
					fmt.Fprintf(cmd.OutOrStdout(), "  Mark as checked? [y/N] ")
					if scanner.Scan() {
						answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
						confirm = answer == "y" || answer == "yes"
					}
				}

				if confirm {
					// Update in cache and save immediately so partial progress is kept.
					for j := range cache.Records {
						if cache.Records[j].RecordID == r.RecordID {
							cache.Records[j].Checked = true
							break
						}
					}
					if err := saveFeedbackCache(agentName, cache); err != nil {
						return fmt.Errorf("save feedback cache: %w", err)
					}
					markedCount++
				}

				fmt.Fprintln(cmd.OutOrStdout())
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%d record(s) shown", len(toShow))
			if markedCount > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), ", %d marked as checked", markedCount)
			}
			fmt.Fprintln(cmd.OutOrStdout(), ".")

			return nil
		},
	}

	cmd.Flags().BoolVar(&showAll, "all", false, "Show all feedback (default: negative only)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum number of records to show (0 = unlimited)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON array")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Auto-confirm marking each record as checked")
	cmd.Flags().BoolVar(&includeChecked, "include-checked", false, "Also show already-checked records")
	cmd.Flags().BoolVar(&noTools, "no-tools", false, "Hide tool invocation details (Query, SQL, RespTime)")

	return cmd
}

// printOneRecord prints a single feedback record with its index out of total.
func printOneRecord(cmd *cobra.Command, idx, total int, r CachedFeedbackRecord, includeChecked bool, noTools bool) {
	checkedMark := ""
	if includeChecked {
		if r.Checked {
			checkedMark = "[✓] "
		} else {
			checkedMark = "[ ] "
		}
	}
	fmt.Fprintf(cmd.OutOrStdout(), "  %s[%d/%d] %s  user: %s\n", checkedMark, idx, total, r.Timestamp, feedbackUserDisplay(r.UserName))

	switch r.Sentiment {
	case "negative":
		color.New(color.FgRed).Fprintf(cmd.OutOrStdout(), "      Sentiment: %s\n", r.Sentiment)
	case "positive":
		color.New(color.FgGreen).Fprintf(cmd.OutOrStdout(), "      Sentiment: %s\n", r.Sentiment)
	default:
		fmt.Fprintf(cmd.OutOrStdout(), "      Sentiment: %s\n", r.Sentiment)
	}

	if r.Comment != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "      Comment:   %q\n", r.Comment)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "      Comment:   (not identified)\n")
	}
	if len(r.Categories) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "      Categories: %s\n", strings.Join(r.Categories, ", "))
	}

	if r.Question != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "      Question:  %s\n", indentMultiline(r.Question, "               "))
	}
	if r.Response != "" {
		display := r.Response
		truncated := false
		const maxLen = 1000
		if len([]rune(display)) > maxLen {
			runes := []rune(display)
			display = string(runes[:maxLen])
			truncated = true
		}
		const indent = "      "
		const sepWidth = 40
		fmt.Fprintln(cmd.OutOrStdout(), indent+"── Response "+strings.Repeat("─", sepWidth-len("── Response ")))
		for _, line := range strings.Split(display, "\n") {
			fmt.Fprintf(cmd.OutOrStdout(), "%s%s\n", indent, line)
		}
		if truncated {
			fmt.Fprintf(cmd.OutOrStdout(), "%s...(truncated)\n", indent)
		}
		fmt.Fprintln(cmd.OutOrStdout(), indent+strings.Repeat("─", sepWidth))
	}
	if r.ResponseTimeMs > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "      RespTime:  %s\n", formatDuration(time.Duration(r.ResponseTimeMs)*time.Millisecond))
	}
	if !noTools {
		if len(r.ToolUses) > 0 {
			var toolParts []string
			for _, tu := range r.ToolUses {
				toolParts = append(toolParts, tu.ToolType)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "      Tools:     %s\n", strings.Join(toolParts, " → "))
			const subIndent = "         " // 9 spaces (3 deeper than top-level fields)
			for i, tu := range r.ToolUses {
				if tu.ToolType == "cortex_analyst_text_to_sql" && tu.Query != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "%sQuery[%d]:  %s\n", subIndent, i, indentMultiline(tu.Query, "                    "))
				}
				if tu.SQL != "" {
					const sepWidth = 40
					header := fmt.Sprintf("── SQL[%d] ", i)
					fmt.Fprintln(cmd.OutOrStdout(), subIndent+header+strings.Repeat("─", sepWidth-len(header)))
					for _, line := range strings.Split(tu.SQL, "\n") {
						fmt.Fprintf(cmd.OutOrStdout(), "%s%s\n", subIndent, line)
					}
					fmt.Fprintln(cmd.OutOrStdout(), subIndent+strings.Repeat("─", sepWidth))
					fmt.Fprintln(cmd.OutOrStdout())
				}
				if tu.ToolStatus == "error" {
					fmt.Fprintf(cmd.OutOrStdout(), "%sStatus[%d]: ERROR\n", subIndent, i)
				}
			}
		}
	}
	if r.Sentiment == "unknown" && r.RawRecord != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "      Record:    %s\n", r.RawRecord)
	}

	fmt.Fprintln(cmd.OutOrStdout())
}

func feedbackUserDisplay(name string) string {
	if name == "" {
		return "(unknown)"
	}
	return name
}

// indentMultiline joins lines with a newline followed by continuationIndent,
// so that multi-line values are aligned under the first line.
func indentMultiline(s, continuationIndent string) string {
	lines := strings.Split(s, "\n")
	return strings.Join(lines, "\n"+continuationIndent)
}
