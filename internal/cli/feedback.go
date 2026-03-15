package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"coragent/internal/api"
	"coragent/internal/auth"
	"coragent/internal/config"
	"coragent/internal/feedbackcache"
)

type feedbackClient interface {
	FeedbackTableExists(ctx context.Context, db, schema, table string) (bool, error)
	FeedbackInferenceColumnsExist(ctx context.Context, db, schema, table string) (bool, error)
	RenameFeedbackTable(ctx context.Context, db, schema, fromTable, toTable string) error
	ClearFeedbackForAgent(ctx context.Context, db, schema, table, agentName string) error
	GetLatestFeedbackEventTs(ctx context.Context, db, schema, table, agentName string) (string, error)
	SyncFeedbackFromEventsToTable(ctx context.Context, srcDB, srcSchema, agentName, dstDB, dstSchema, dstTable string, opts api.FeedbackQueryOptions) error
	GetFeedbackFromTable(ctx context.Context, db, schema, table, agentName string) ([]api.FeedbackTableRow, error)
	GetFeedback(ctx context.Context, db, schema, agentName string, opts api.FeedbackQueryOptions) ([]api.FeedbackRecord, error)
	UpdateFeedbackChecked(ctx context.Context, db, schema, table, recordID string, checked bool) error
	CreateFeedbackTable(ctx context.Context, db, schema, table string) error
}

var buildFeedbackClientAndCfg = func(opts *RootOptions) (feedbackClient, auth.Config, error) {
	return buildClientAndCfg(opts)
}

var promptWithDefaultFn = promptWithDefault
var feedbackInitNow = func() time.Time { return time.Now().UTC() }

const defaultFeedbackJudgeModel = "llama4-scout"

func resolveFeedbackJudgeModel(appCfg config.CoragentConfig) string {
	if strings.TrimSpace(appCfg.Feedback.JudgeModel) != "" {
		return appCfg.Feedback.JudgeModel
	}
	return defaultFeedbackJudgeModel
}

func feedbackQueryOptions(since string, inferNegative bool, judgeModel string) api.FeedbackQueryOptions {
	opts := api.FeedbackQueryOptions{
		Since:         since,
		InferNegative: inferNegative,
		JudgeModel:    judgeModel,
	}
	if inferNegative {
		opts.ExplicitSince = ""
		opts.RequestSince = since
	}
	return opts
}

func feedbackProgressf(cmd *cobra.Command, enabled bool, format string, args ...any) {
	if !enabled {
		return
	}
	fmt.Fprintf(cmd.OutOrStdout(), format+"\n", args...)
}

func newFeedbackCmd(opts *RootOptions) *cobra.Command {
	var showAll bool
	var limit int
	var jsonOut bool
	var yes bool
	var includeChecked bool
	var noTools bool
	var noRefresh bool
	var clearCache bool
	var initTable bool
	var inferNegative bool

	cmd := &cobra.Command{
		Use:   "feedback [agent-name]",
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

  # Read only the existing saved state (skip refresh/sync)
  coragent feedback my-agent --no-refresh

  # JSON output
  coragent feedback my-agent --json | jq .

  # Infer negative interactions without explicit feedback
  coragent feedback my-agent --infer-negative

  # Ensure remote feedback table exists (when feedback.remote.enabled in config)
  coragent feedback --init`,
		Args: func(cmd *cobra.Command, args []string) error {
			initMode, err := cmd.Flags().GetBool("init")
			if err != nil {
				return err
			}
			if initMode {
				return cobra.MaximumNArgs(1)(cmd, args)
			}
			return cobra.ExactArgs(1)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			appCfg := config.LoadCoragentConfig()
			feedbackJudgeModel := resolveFeedbackJudgeModel(appCfg)
			remoteDb, remoteSchema, remoteTable := resolveFeedbackRemote(appCfg)
			useRemote := appCfg.Feedback.Remote.Enabled && remoteDb != "" && remoteSchema != "" && remoteTable != ""

			if initTable {
				return runFeedbackInit(cmd, opts, appCfg)
			}
			agentName := args[0]

			if clearCache {
				if useRemote {
					client, _, err := buildFeedbackClientAndCfg(opts)
					if err != nil {
						return err
					}
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					exists, err := client.FeedbackTableExists(ctx, remoteDb, remoteSchema, remoteTable)
					if err != nil {
						return fmt.Errorf("check remote feedback table: %w", err)
					}
					if !exists {
						return UserErr(fmt.Errorf(
							"remote feedback table %s.%s.%s not found; run `coragent feedback --init` first",
							remoteDb, remoteSchema, remoteTable,
						))
					}
					if err := client.ClearFeedbackForAgent(ctx, remoteDb, remoteSchema, remoteTable, agentName); err != nil {
						return fmt.Errorf("clear remote feedback records: %w", err)
					}
					fmt.Fprintf(
						cmd.OutOrStdout(),
						"Remote feedback records cleared for agent %q in %s.%s.%s.\n",
						agentName, remoteDb, remoteSchema, remoteTable,
					)
					return nil
				}
				path, err := feedbackcache.CachePath(agentName)
				if err != nil {
					return err
				}
				if err := os.Remove(path); err != nil {
					if os.IsNotExist(err) {
						fmt.Fprintf(cmd.OutOrStdout(), "No cache found for agent %q.\n", agentName)
						return nil
					}
					return fmt.Errorf("remove cache: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Feedback cache cleared for agent %q.\n", agentName)
				return nil
			}

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			var remoteClient feedbackClient
			var toShow []feedbackcache.Record
			var localCache *feedbackcache.Cache
			progressEnabled := !jsonOut
			if useRemote {
				feedbackProgressf(cmd, progressEnabled, "Loading remote feedback state...")
				client, cfg, err := buildFeedbackClientAndCfg(opts)
				if err != nil {
					return err
				}
				remoteClient = client
				exists, err := client.FeedbackTableExists(ctx, remoteDb, remoteSchema, remoteTable)
				if err != nil {
					return fmt.Errorf("check remote feedback table: %w", err)
				}
				if !exists {
					return UserErr(fmt.Errorf(
						"remote feedback table %s.%s.%s not found; run `coragent feedback %s --init` first",
						remoteDb, remoteSchema, remoteTable, agentName,
					))
				}

				if !noRefresh {
					target, err := ResolveTargetForExport(opts, cfg)
					if err != nil {
						return err
					}
					if inferNegative {
						ready, err := client.FeedbackInferenceColumnsExist(ctx, remoteDb, remoteSchema, remoteTable)
						if err != nil {
							return fmt.Errorf("check infer-negative columns: %w", err)
						}
						if !ready {
							return UserErr(fmt.Errorf(
								"remote feedback table %s.%s.%s is missing infer-negative columns; run `coragent feedback --init` to recreate it",
								remoteDb, remoteSchema, remoteTable,
							))
						}
					}
					// 1. Get latest event_ts from remote table for diff sync.
					since, err := client.GetLatestFeedbackEventTs(ctx, remoteDb, remoteSchema, remoteTable, agentName)
					if err != nil {
						return fmt.Errorf("get latest feedback timestamp: %w", err)
					}
					if since == "" {
						feedbackProgressf(cmd, progressEnabled, "Syncing feedback from observability events into %s.%s.%s...", remoteDb, remoteSchema, remoteTable)
					} else {
						feedbackProgressf(cmd, progressEnabled, "Syncing feedback updates since %s into %s.%s.%s...", since, remoteDb, remoteSchema, remoteTable)
					}
					if err := client.SyncFeedbackFromEventsToTable(ctx, target.Database, target.Schema, agentName, remoteDb, remoteSchema, remoteTable, feedbackQueryOptions(since, inferNegative, feedbackJudgeModel)); err != nil {
						return fmt.Errorf("sync feedback to remote table: %w", err)
					}
				}
				// 2. Load records with checked state from remote table.
				feedbackProgressf(cmd, progressEnabled, "Loading feedback records from %s.%s.%s...", remoteDb, remoteSchema, remoteTable)
				rows, err := client.GetFeedbackFromTable(ctx, remoteDb, remoteSchema, remoteTable, agentName)
				if err != nil {
					return fmt.Errorf("load feedback from remote table: %w", err)
				}
				toShow = mergeRemoteRows(rows, includeChecked, nil)
			} else {
				// 1. Load cache and determine since (latest timestamp) for diff fetch.
				feedbackProgressf(cmd, progressEnabled, "Loading local feedback cache...")
				cache, err := feedbackcache.Load(agentName)
				if err != nil {
					return fmt.Errorf("load feedback cache: %w", err)
				}
				localCache = cache
				if !noRefresh {
					client, cfg, err := buildFeedbackClientAndCfg(opts)
					if err != nil {
						return err
					}
					target, err := ResolveTargetForExport(opts, cfg)
					if err != nil {
						return err
					}
					since := localCache.LatestTimestamp()
					// 2. Fetch only new records from Snowflake (since cache latest).
					if since == "" {
						feedbackProgressf(cmd, progressEnabled, "Fetching feedback from observability events...")
					} else {
						feedbackProgressf(cmd, progressEnabled, "Fetching feedback updates since %s...", since)
					}
					fresh, err := client.GetFeedback(ctx, target.Database, target.Schema, agentName, feedbackQueryOptions(since, inferNegative, feedbackJudgeModel))
					if err != nil {
						return err
					}
					localCache.Merge(fresh)
					feedbackProgressf(cmd, progressEnabled, "Saving refreshed feedback cache...")
					if err := feedbackcache.Save(agentName, localCache); err != nil {
						return fmt.Errorf("save feedback cache: %w", err)
					}
				}
				// 3. Select records to display.
				if includeChecked {
					toShow = localCache.Records
				} else {
					for _, r := range localCache.Records {
						if !r.Checked {
							toShow = append(toShow, r)
						}
					}
				}
			}
			feedbackProgressf(cmd, progressEnabled, "Preparing feedback records for display...")

			// Apply sentiment filter (--all) and --limit.
			if !showAll {
				var filtered []feedbackcache.Record
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
				data, err := marshalFeedbackJSON(toShow)
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
				if inferNegative {
					fmt.Fprintf(cmd.OutOrStdout(), "No feedback or inferred negative interactions found for agent %q.\n", agentName)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "No feedback found for agent %q.\n", agentName)
				}
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
					if useRemote {
						if err := remoteClient.UpdateFeedbackChecked(ctx, remoteDb, remoteSchema, remoteTable, r.RecordID, true); err != nil {
							return fmt.Errorf("update checked in remote table: %w", err)
						}
						toShow[i].Checked = true
					} else {
						for j := range localCache.Records {
							if localCache.Records[j].RecordID == r.RecordID {
								localCache.Records[j].Checked = true
								break
							}
						}
						if err := feedbackcache.Save(agentName, localCache); err != nil {
							return fmt.Errorf("save feedback cache: %w", err)
						}
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
	cmd.Flags().BoolVar(&noTools, "no-tools", false, "Hide tool invocation details (Tools, Query, SQL)")
	cmd.Flags().BoolVar(&noRefresh, "no-refresh", false, "Read only existing saved feedback state; skip API fetch or remote sync")
	cmd.Flags().BoolVar(&clearCache, "clear", false, "Clear feedback state for the agent and exit (local cache or remote table)")
	cmd.Flags().BoolVar(&initTable, "init", false, "Ensure the remote feedback table exists (create if missing); requires feedback.remote in config")
	cmd.Flags().BoolVar(&inferNegative, "infer-negative", false, "Infer negative interactions from request/response pairs when explicit feedback is absent")

	return cmd
}

// resolveFeedbackRemote returns database, schema, table from config for remote feedback storage.
func resolveFeedbackRemote(appCfg config.CoragentConfig) (db, schema, table string) {
	r := appCfg.Feedback.Remote
	return strings.TrimSpace(r.Database), strings.TrimSpace(r.Schema), strings.TrimSpace(r.Table)
}

// runFeedbackInit ensures the remote feedback table exists; creates it if missing.
func runFeedbackInit(cmd *cobra.Command, opts *RootOptions, appCfg config.CoragentConfig) error {
	db, schema, table := resolveFeedbackRemote(appCfg)
	if !appCfg.Feedback.Remote.Enabled || db == "" || schema == "" || table == "" {
		return UserErr(fmt.Errorf("feedback --init requires [feedback.remote] in config with enabled = true, database, schema, and table"))
	}
	client, _, err := buildFeedbackClientAndCfg(opts)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	exists, err := client.FeedbackTableExists(ctx, db, schema, table)
	if err != nil {
		return fmt.Errorf("check feedback table: %w", err)
	}
	if exists {
		fqTable := fmt.Sprintf("%s.%s.%s", db, schema, table)
		fmt.Fprintf(cmd.OutOrStdout(), "Feedback table %s already exists.\n", fqTable)

		renameExisting, err := promptYesNo(
			"Rename the existing table before re-creating it? This keeps the current rows under a backup table name. [Y/n]",
			true,
		)
		if err != nil {
			return err
		}
		if renameExisting {
			backupTable, err := promptWithDefaultFn("Backup table name", defaultFeedbackBackupTableName(table, feedbackInitNow()))
			if err != nil {
				return err
			}
			if strings.EqualFold(strings.Trim(backupTable, `"`), strings.Trim(table, `"`)) {
				return UserErr(fmt.Errorf("backup table name must differ from the current feedback table name"))
			}
			if err := client.RenameFeedbackTable(ctx, db, schema, table, backupTable); err != nil {
				return fmt.Errorf("rename feedback table: %w", err)
			}
			fmt.Fprintf(
				cmd.OutOrStdout(),
				"Renamed existing feedback table %s to %s.%s.%s.\n",
				fqTable, db, schema, backupTable,
			)
		} else {
			confirmed, err := promptYesNo(
				"Recreate with CREATE OR REPLACE TABLE? This will drop existing rows. [y/N]",
				false,
			)
			if err != nil {
				return err
			}
			if !confirmed {
				fmt.Fprintln(cmd.OutOrStdout(), "Skipped re-creating feedback table.")
				return nil
			}
		}
	}
	if err := client.CreateFeedbackTable(ctx, db, schema, table); err != nil {
		return fmt.Errorf("create feedback table: %w", err)
	}
	if exists {
		fmt.Fprintf(cmd.OutOrStdout(), "Recreated feedback table %s.%s.%s.\n", db, schema, table)
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Created feedback table %s.%s.%s.\n", db, schema, table)
	return nil
}

func promptYesNo(label string, defaultYes bool) (bool, error) {
	defaultValue := "N"
	if defaultYes {
		defaultValue = "Y"
	}
	answer, err := promptWithDefaultFn(label, defaultValue)
	if err != nil {
		return false, err
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}

func defaultFeedbackBackupTableName(table string, now time.Time) string {
	base := strings.TrimSpace(strings.Trim(table, `"`))
	if base == "" {
		base = "AGENT_FEEDBACK"
	}
	suffix := "_bak_" + now.UTC().Format("20060102_150405")
	const maxIdentifierLen = 255
	if len(base)+len(suffix) > maxIdentifierLen {
		base = base[:maxIdentifierLen-len(suffix)]
	}
	return base + suffix
}

func mergeRemoteRows(rows []api.FeedbackTableRow, includeChecked bool, toolByRecord map[string][]api.ToolUseInfo) []feedbackcache.Record {
	out := make([]feedbackcache.Record, 0, len(rows))
	for _, row := range rows {
		if fallback, ok := toolByRecord[row.RecordID]; ok {
			needsFallback := len(row.ToolUses) == 0
			if !needsFallback {
				allMissingSQL := true
				for _, tu := range row.ToolUses {
					if tu.SQL != "" || tu.ToolStatus != "" {
						allMissingSQL = false
						break
					}
				}
				needsFallback = allMissingSQL
			}
			if needsFallback {
				row.ToolUses = fallback
			}
		}
		if !includeChecked && row.Checked {
			continue
		}
		out = append(out, feedbackcache.Record{
			Checked:        row.Checked,
			FeedbackRecord: row.FeedbackRecord,
		})
	}
	return out
}

func marshalFeedbackJSON(records []feedbackcache.Record) ([]byte, error) {
	if len(records) == 0 {
		return []byte("[]"), nil
	}
	return json.MarshalIndent(records, "", "  ")
}

// printOneRecord prints a single feedback record with its index out of total.
func printOneRecord(cmd *cobra.Command, idx, total int, r feedbackcache.Record, includeChecked bool, noTools bool) {
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
	if r.SentimentSource == "inferred" || r.SentimentReason != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "      Source:    %s\n", r.SentimentSource)
	}
	if r.SentimentReason != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "      Reason:    %s\n", indentMultiline(r.SentimentReason, "                 "))
	}

	if r.FeedbackMessage != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "      FeedbackMessage: %q\n", r.FeedbackMessage)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "      FeedbackMessage: (not identified)\n")
	}
	if len(r.Categories) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "      Categories: %s\n", strings.Join(r.Categories, ", "))
	}

	if r.Question != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "      Question:  %s\n", indentMultiline(r.Question, "               "))
	}
	if r.Response != "" {
		const indent = "      "
		const sepWidth = 40
		fmt.Fprintln(cmd.OutOrStdout(), indent+"── Response "+strings.Repeat("─", sepWidth-len("── Response ")))
		for _, line := range strings.Split(r.Response, "\n") {
			fmt.Fprintf(cmd.OutOrStdout(), "%s%s\n", indent, line)
		}
		fmt.Fprintln(cmd.OutOrStdout(), indent+strings.Repeat("─", sepWidth))
	}
	if r.ResponseTimeMs > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "      RespTime:  %s\n", formatDuration(time.Duration(r.ResponseTimeMs)*time.Millisecond))
	}
	if !noTools {
		if len(r.ToolUses) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "      Tools:     %s\n", formatToolChain(r.ToolUses))
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

func formatToolChain(toolUses []api.ToolUseInfo) string {
	parts := make([]string, 0, len(toolUses))
	for _, tu := range toolUses {
		toolType := strings.TrimSpace(tu.ToolType)
		toolName := strings.TrimSpace(tu.ToolName)
		switch {
		case toolType != "" && toolName != "":
			parts = append(parts, fmt.Sprintf("%s (%s)", toolType, toolName))
		case toolName != "":
			parts = append(parts, toolName)
		default:
			parts = append(parts, toolType)
		}
	}
	return strings.Join(parts, " → ")
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
