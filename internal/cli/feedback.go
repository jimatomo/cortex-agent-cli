package cli

import (
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

	cmd := &cobra.Command{
		Use:   "feedback <agent-name>",
		Short: "Show user feedback for a Cortex Agent",
		Long: `Retrieve user feedback events for a Cortex Agent from observability data.

By default, only negative feedback is shown. Use --all to show all feedback.`,
		Example: `  # Show negative feedback (default)
  coragent feedback my-agent -d MY_DB -s MY_SCHEMA

  # Show all feedback
  coragent feedback my-agent --all

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

			records, err := client.GetFeedback(ctx, target.Database, target.Schema, agentName)
			if err != nil {
				return err
			}

			// Filter to negative only unless --all
			if !showAll {
				var filtered []api.FeedbackRecord
				for _, r := range records {
					if r.Sentiment == "negative" {
						filtered = append(filtered, r)
					}
				}
				records = filtered
			}

			// Apply limit
			if limit > 0 && len(records) > limit {
				records = records[:limit]
			}

			if jsonOut {
				data, err := json.MarshalIndent(records, "", "  ")
				if err != nil {
					return fmt.Errorf("marshal JSON: %w", err)
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return err
			}

			return printFeedback(cmd, agentName, records, showAll)
		},
	}

	cmd.Flags().BoolVar(&showAll, "all", false, "Show all feedback (default: negative only)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum number of records to show (0 = unlimited)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON array")

	return cmd
}

func printFeedback(cmd *cobra.Command, agentName string, records []api.FeedbackRecord, showAll bool) error {
	filter := "negative only"
	if showAll {
		filter = "all"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Feedback for agent %q (%s):\n\n", agentName, filter)

	if len(records) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "No feedback found for agent %q.\n", agentName)
		return nil
	}

	allUnknown := true
	for _, r := range records {
		if r.Sentiment != "unknown" {
			allUnknown = false
			break
		}
	}
	if allUnknown {
		fmt.Fprintln(os.Stderr, "Warning: sentiment could not be determined from RECORD data; showing raw JSON.")
	}

	for i, r := range records {
		fmt.Fprintf(cmd.OutOrStdout(), "  [%d] %s  user: %s\n", i+1, r.Timestamp, feedbackUserDisplay(r.UserName))

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

		if r.Sentiment == "unknown" && r.RawRecord != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "      Record:    %s\n", r.RawRecord)
		}

		fmt.Fprintln(cmd.OutOrStdout())
	}

	fmt.Fprintf(cmd.OutOrStdout(), "%d record(s) shown.\n", len(records))
	return nil
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
