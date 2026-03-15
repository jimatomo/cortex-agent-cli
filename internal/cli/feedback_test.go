package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"coragent/internal/api"
	"coragent/internal/auth"
	"coragent/internal/config"
	"coragent/internal/feedbackcache"

	"github.com/spf13/cobra"
)

type stubFeedbackClient struct {
	feedbackTableExistsFn      func(ctx context.Context, db, schema, table string) (bool, error)
	feedbackInferenceColumnsFn func(ctx context.Context, db, schema, table string) (bool, error)
	renameFeedbackTableFn      func(ctx context.Context, db, schema, fromTable, toTable string) error
	clearFeedbackForAgentFn    func(ctx context.Context, db, schema, table, agentName string) error
	getLatestFeedbackEventTsFn func(ctx context.Context, db, schema, table, agentName string) (string, error)
	syncFeedbackFromEventsToFn func(ctx context.Context, srcDB, srcSchema, agentName, dstDB, dstSchema, dstTable string, opts api.FeedbackQueryOptions) error
	getFeedbackFromTableFn     func(ctx context.Context, db, schema, table, agentName string) ([]api.FeedbackTableRow, error)
	getFeedbackFn              func(ctx context.Context, db, schema, agentName string, opts api.FeedbackQueryOptions) ([]api.FeedbackRecord, error)
	updateFeedbackCheckedFn    func(ctx context.Context, db, schema, table, recordID string, checked bool) error
	createFeedbackTableFn      func(ctx context.Context, db, schema, table string) error
}

func (s *stubFeedbackClient) FeedbackTableExists(ctx context.Context, db, schema, table string) (bool, error) {
	if s.feedbackTableExistsFn != nil {
		return s.feedbackTableExistsFn(ctx, db, schema, table)
	}
	return false, nil
}

func (s *stubFeedbackClient) FeedbackInferenceColumnsExist(ctx context.Context, db, schema, table string) (bool, error) {
	if s.feedbackInferenceColumnsFn != nil {
		return s.feedbackInferenceColumnsFn(ctx, db, schema, table)
	}
	return true, nil
}

func (s *stubFeedbackClient) RenameFeedbackTable(ctx context.Context, db, schema, fromTable, toTable string) error {
	if s.renameFeedbackTableFn != nil {
		return s.renameFeedbackTableFn(ctx, db, schema, fromTable, toTable)
	}
	return nil
}

func (s *stubFeedbackClient) ClearFeedbackForAgent(ctx context.Context, db, schema, table, agentName string) error {
	if s.clearFeedbackForAgentFn != nil {
		return s.clearFeedbackForAgentFn(ctx, db, schema, table, agentName)
	}
	return nil
}

func (s *stubFeedbackClient) GetLatestFeedbackEventTs(ctx context.Context, db, schema, table, agentName string) (string, error) {
	if s.getLatestFeedbackEventTsFn != nil {
		return s.getLatestFeedbackEventTsFn(ctx, db, schema, table, agentName)
	}
	return "", nil
}

func (s *stubFeedbackClient) SyncFeedbackFromEventsToTable(ctx context.Context, srcDB, srcSchema, agentName, dstDB, dstSchema, dstTable string, opts api.FeedbackQueryOptions) error {
	if s.syncFeedbackFromEventsToFn != nil {
		return s.syncFeedbackFromEventsToFn(ctx, srcDB, srcSchema, agentName, dstDB, dstSchema, dstTable, opts)
	}
	return nil
}

func (s *stubFeedbackClient) GetFeedbackFromTable(ctx context.Context, db, schema, table, agentName string) ([]api.FeedbackTableRow, error) {
	if s.getFeedbackFromTableFn != nil {
		return s.getFeedbackFromTableFn(ctx, db, schema, table, agentName)
	}
	return nil, nil
}

func (s *stubFeedbackClient) GetFeedback(ctx context.Context, db, schema, agentName string, opts api.FeedbackQueryOptions) ([]api.FeedbackRecord, error) {
	if s.getFeedbackFn != nil {
		return s.getFeedbackFn(ctx, db, schema, agentName, opts)
	}
	return nil, nil
}

func (s *stubFeedbackClient) UpdateFeedbackChecked(ctx context.Context, db, schema, table, recordID string, checked bool) error {
	if s.updateFeedbackCheckedFn != nil {
		return s.updateFeedbackCheckedFn(ctx, db, schema, table, recordID, checked)
	}
	return nil
}

func (s *stubFeedbackClient) CreateFeedbackTable(ctx context.Context, db, schema, table string) error {
	if s.createFeedbackTableFn != nil {
		return s.createFeedbackTableFn(ctx, db, schema, table)
	}
	return nil
}

func TestResolveFeedbackRemote(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.CoragentConfig
		wantDb  string
		wantSch string
		wantTbl string
	}{
		{"empty", config.CoragentConfig{}, "", "", ""},
		{"enabled with values", config.CoragentConfig{
			Feedback: config.FeedbackSettings{
				Remote: config.FeedbackRemoteSettings{
					Enabled:  true,
					Database: "D1",
					Schema:   "S1",
					Table:    "T1",
				},
			},
		}, "D1", "S1", "T1"},
		{"trimmed", config.CoragentConfig{
			Feedback: config.FeedbackSettings{
				Remote: config.FeedbackRemoteSettings{
					Enabled:  true,
					Database: "  DB ",
					Schema:   " SC ",
					Table:    " T ",
				},
			},
		}, "DB", "SC", "T"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sch, tbl := resolveFeedbackRemote(tt.cfg)
			if db != tt.wantDb || sch != tt.wantSch || tbl != tt.wantTbl {
				t.Errorf("resolveFeedbackRemote() = %q, %q, %q; want %q, %q, %q", db, sch, tbl, tt.wantDb, tt.wantSch, tt.wantTbl)
			}
		})
	}
}

func TestResolveFeedbackJudgeModel(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		got := resolveFeedbackJudgeModel(config.CoragentConfig{})
		if got != defaultFeedbackJudgeModel {
			t.Fatalf("resolveFeedbackJudgeModel() = %q, want %q", got, defaultFeedbackJudgeModel)
		}
	})

	t.Run("config override", func(t *testing.T) {
		got := resolveFeedbackJudgeModel(config.CoragentConfig{
			Feedback: config.FeedbackSettings{
				JudgeModel: "custom-feedback-model",
			},
		})
		if got != "custom-feedback-model" {
			t.Fatalf("resolveFeedbackJudgeModel() = %q, want custom-feedback-model", got)
		}
	})
}

func TestRunFeedbackInit_RequiresRemoteConfig(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })
	os.Chdir(dir)
	// No .coragent.toml so LoadCoragentConfig returns zero value
	t.Setenv("HOME", dir)
	os.MkdirAll(filepath.Join(dir, ".coragent"), 0o755)

	cmd := newFeedbackCmd(&RootOptions{})
	cmd.SetArgs([]string{"--init"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --init without feedback.remote config")
	}
	if !IsUserError(err) {
		t.Errorf("expected UserError for missing config, got: %v", err)
	}
	if err.Error() == "" || len(err.Error()) < 10 {
		t.Errorf("error message too short: %q", err.Error())
	}
}

func TestRunFeedbackInit_RenamesExistingTableBeforeRecreate(t *testing.T) {
	origBuild := buildFeedbackClientAndCfg
	origPrompt := promptWithDefaultFn
	origNow := feedbackInitNow
	t.Cleanup(func() {
		buildFeedbackClientAndCfg = origBuild
		promptWithDefaultFn = origPrompt
		feedbackInitNow = origNow
	})

	var renamedFrom, renamedTo string
	createCalled := false
	client := &stubFeedbackClient{
		feedbackTableExistsFn: func(ctx context.Context, db, schema, table string) (bool, error) {
			return true, nil
		},
		renameFeedbackTableFn: func(ctx context.Context, db, schema, fromTable, toTable string) error {
			renamedFrom = fromTable
			renamedTo = toTable
			return nil
		},
		createFeedbackTableFn: func(ctx context.Context, db, schema, table string) error {
			createCalled = true
			return nil
		},
	}
	buildFeedbackClientAndCfg = func(opts *RootOptions) (feedbackClient, auth.Config, error) {
		return client, auth.Config{}, nil
	}

	promptCalls := 0
	promptWithDefaultFn = func(label, defaultVal string) (string, error) {
		promptCalls++
		switch promptCalls {
		case 1:
			return "y", nil
		case 2:
			return defaultVal, nil
		default:
			t.Fatalf("unexpected prompt %q", label)
			return "", nil
		}
	}
	feedbackInitNow = func() time.Time {
		return time.Date(2026, time.March, 12, 10, 11, 12, 0, time.UTC)
	}

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)

	err := runFeedbackInit(cmd, &RootOptions{}, config.CoragentConfig{
		Feedback: config.FeedbackSettings{
			Remote: config.FeedbackRemoteSettings{
				Enabled:  true,
				Database: "REMOTE_DB",
				Schema:   "REMOTE_SCHEMA",
				Table:    "AGENT_FEEDBACK",
			},
		},
	})
	if err != nil {
		t.Fatalf("runFeedbackInit() error = %v", err)
	}
	if renamedFrom != "AGENT_FEEDBACK" {
		t.Fatalf("renamedFrom = %q, want AGENT_FEEDBACK", renamedFrom)
	}
	if renamedTo != "AGENT_FEEDBACK_bak_20260312_101112" {
		t.Fatalf("renamedTo = %q, want default backup table name", renamedTo)
	}
	if !createCalled {
		t.Fatal("expected CreateFeedbackTable to be called")
	}
	if !strings.Contains(out.String(), "Renamed existing feedback table REMOTE_DB.REMOTE_SCHEMA.AGENT_FEEDBACK") {
		t.Fatalf("expected rename message, got:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Recreated feedback table REMOTE_DB.REMOTE_SCHEMA.AGENT_FEEDBACK.") {
		t.Fatalf("expected recreate message, got:\n%s", out.String())
	}
}

func TestRunFeedbackInit_SkipsWhenUserDeclinesRenameAndRecreate(t *testing.T) {
	origBuild := buildFeedbackClientAndCfg
	origPrompt := promptWithDefaultFn
	t.Cleanup(func() {
		buildFeedbackClientAndCfg = origBuild
		promptWithDefaultFn = origPrompt
	})

	client := &stubFeedbackClient{
		feedbackTableExistsFn: func(ctx context.Context, db, schema, table string) (bool, error) {
			return true, nil
		},
		renameFeedbackTableFn: func(ctx context.Context, db, schema, fromTable, toTable string) error {
			t.Fatal("RenameFeedbackTable should not be called")
			return nil
		},
		createFeedbackTableFn: func(ctx context.Context, db, schema, table string) error {
			t.Fatal("CreateFeedbackTable should not be called")
			return nil
		},
	}
	buildFeedbackClientAndCfg = func(opts *RootOptions) (feedbackClient, auth.Config, error) {
		return client, auth.Config{}, nil
	}

	promptCalls := 0
	promptWithDefaultFn = func(label, defaultVal string) (string, error) {
		promptCalls++
		if promptCalls == 1 {
			return "n", nil
		}
		if promptCalls == 2 {
			return "n", nil
		}
		t.Fatalf("unexpected prompt %q", label)
		return "", nil
	}

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)

	err := runFeedbackInit(cmd, &RootOptions{}, config.CoragentConfig{
		Feedback: config.FeedbackSettings{
			Remote: config.FeedbackRemoteSettings{
				Enabled:  true,
				Database: "REMOTE_DB",
				Schema:   "REMOTE_SCHEMA",
				Table:    "AGENT_FEEDBACK",
			},
		},
	})
	if err != nil {
		t.Fatalf("runFeedbackInit() error = %v", err)
	}
	if !strings.Contains(out.String(), "Skipped re-creating feedback table.") {
		t.Fatalf("expected skip message, got:\n%s", out.String())
	}
}

func TestFeedbackNoRefresh_LocalModeSkipsFetch(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Setenv("HOME", dir)

	if err := feedbackcache.Save("my-agent", &feedbackcache.Cache{
		Records: []feedbackcache.Record{
			{
				FeedbackRecord: api.FeedbackRecord{
					RecordID:  "cached-1",
					Timestamp: "2026-03-08 00:00:00.000 UTC",
					UserName:  "alice",
					Sentiment: "negative",
				},
			},
		},
	}); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	origBuild := buildFeedbackClientAndCfg
	t.Cleanup(func() { buildFeedbackClientAndCfg = origBuild })
	buildFeedbackClientAndCfg = func(opts *RootOptions) (feedbackClient, auth.Config, error) {
		t.Fatal("buildFeedbackClientAndCfg should not be called in local --no-refresh mode")
		return nil, auth.Config{}, nil
	}

	var out bytes.Buffer
	cmd := newFeedbackCmd(&RootOptions{})
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"my-agent", "--no-refresh", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), `"record_id": "cached-1"`) {
		t.Fatalf("expected cached record in output, got:\n%s", out.String())
	}
}

func TestFeedbackNoRefresh_RemoteModeSkipsSync(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Setenv("HOME", dir)

	if err := os.WriteFile(".coragent.toml", []byte(`[feedback.remote]
enabled = true
database = "REMOTE_DB"
schema = "REMOTE_SCHEMA"
table = "AGENT_FEEDBACK"
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	client := &stubFeedbackClient{
		feedbackTableExistsFn: func(ctx context.Context, db, schema, table string) (bool, error) {
			return true, nil
		},
		getLatestFeedbackEventTsFn: func(ctx context.Context, db, schema, table, agentName string) (string, error) {
			t.Fatal("GetLatestFeedbackEventTs should not be called when --no-refresh is set")
			return "", nil
		},
		syncFeedbackFromEventsToFn: func(ctx context.Context, srcDB, srcSchema, agentName, dstDB, dstSchema, dstTable string, opts api.FeedbackQueryOptions) error {
			t.Fatal("SyncFeedbackFromEventsToTable should not be called when --no-refresh is set")
			return nil
		},
		getFeedbackFromTableFn: func(ctx context.Context, db, schema, table, agentName string) ([]api.FeedbackTableRow, error) {
			return []api.FeedbackTableRow{
				{
					FeedbackRecord: api.FeedbackRecord{
						RecordID:  "remote-1",
						Timestamp: "2026-03-08 00:00:00.000 UTC",
						UserName:  "bob",
						Sentiment: "negative",
					},
				},
			}, nil
		},
	}

	origBuild := buildFeedbackClientAndCfg
	t.Cleanup(func() { buildFeedbackClientAndCfg = origBuild })
	buildFeedbackClientAndCfg = func(opts *RootOptions) (feedbackClient, auth.Config, error) {
		return client, auth.Config{Database: "DB", Schema: "SC"}, nil
	}

	var out bytes.Buffer
	cmd := newFeedbackCmd(&RootOptions{Database: "DB", Schema: "SC"})
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"my-agent", "--no-refresh", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), `"record_id": "remote-1"`) {
		t.Fatalf("expected remote record in output, got:\n%s", out.String())
	}
}

func TestMergeRemoteRows_FilterCheckedByDefault(t *testing.T) {
	rows := []api.FeedbackTableRow{
		{
			FeedbackRecord: api.FeedbackRecord{RecordID: "r1", FeedbackMessage: "unchecked"},
			Checked:        false,
		},
		{
			FeedbackRecord: api.FeedbackRecord{RecordID: "r2", FeedbackMessage: "checked"},
			Checked:        true,
		},
	}

	got := mergeRemoteRows(rows, false, map[string][]api.ToolUseInfo{})
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].RecordID != "r1" {
		t.Fatalf("got[0].RecordID = %q, want r1", got[0].RecordID)
	}
}

func TestMergeRemoteRows_IncludeCheckedWhenRequested(t *testing.T) {
	rows := []api.FeedbackTableRow{
		{
			FeedbackRecord: api.FeedbackRecord{RecordID: "r1"},
			Checked:        false,
		},
		{
			FeedbackRecord: api.FeedbackRecord{RecordID: "r2"},
			Checked:        true,
		},
	}

	got := mergeRemoteRows(rows, true, map[string][]api.ToolUseInfo{})
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
}

func TestMergeRemoteRows_AppliesToolFallback(t *testing.T) {
	rows := []api.FeedbackTableRow{
		{
			FeedbackRecord: api.FeedbackRecord{
				RecordID: "r1",
				ToolUses: []api.ToolUseInfo{
					{ToolType: "cortex_analyst_text_to_sql"},
				},
			},
			Checked: false,
		},
	}
	fallback := map[string][]api.ToolUseInfo{
		"r1": {
			{
				ToolType:   "cortex_analyst_text_to_sql",
				ToolStatus: "success",
				SQL:        "select 1",
			},
		},
	}

	got := mergeRemoteRows(rows, false, fallback)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if len(got[0].ToolUses) != 1 || got[0].ToolUses[0].SQL != "select 1" || got[0].ToolUses[0].ToolStatus != "success" {
		t.Fatalf("fallback tool info not applied: %+v", got[0].ToolUses)
	}
}

func TestFeedbackInferNegativePassesOptionToGetFeedback(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Setenv("HOME", dir)

	var gotOpts api.FeedbackQueryOptions
	client := &stubFeedbackClient{
		getFeedbackFn: func(ctx context.Context, db, schema, agentName string, opts api.FeedbackQueryOptions) ([]api.FeedbackRecord, error) {
			gotOpts = opts
			return []api.FeedbackRecord{{
				RecordID:        "r1",
				Timestamp:       "2026-03-08 00:00:00.000 UTC",
				Sentiment:       "negative",
				SentimentSource: "inferred",
				SentimentReason: "goal not achieved",
			}}, nil
		},
	}

	origBuild := buildFeedbackClientAndCfg
	t.Cleanup(func() { buildFeedbackClientAndCfg = origBuild })
	buildFeedbackClientAndCfg = func(opts *RootOptions) (feedbackClient, auth.Config, error) {
		return client, auth.Config{Database: "DB", Schema: "SC"}, nil
	}

	var out bytes.Buffer
	cmd := newFeedbackCmd(&RootOptions{Database: "DB", Schema: "SC"})
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"my-agent", "--infer-negative", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !gotOpts.InferNegative {
		t.Fatal("InferNegative = false, want true")
	}
	if gotOpts.Since != "" {
		t.Fatalf("Since = %q, want empty for empty cache", gotOpts.Since)
	}
	if gotOpts.JudgeModel != defaultFeedbackJudgeModel {
		t.Fatalf("JudgeModel = %q, want %q", gotOpts.JudgeModel, defaultFeedbackJudgeModel)
	}
	if !strings.Contains(out.String(), `"sentiment_source": "inferred"`) {
		t.Fatalf("expected inferred source in JSON output, got:\n%s", out.String())
	}
}

func TestFeedbackInferNegativeUsesLatestTimestampForDiffFetch(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Setenv("HOME", dir)

	if err := feedbackcache.Save("my-agent", &feedbackcache.Cache{
		Records: []feedbackcache.Record{
			{
				FeedbackRecord: api.FeedbackRecord{
					RecordID:  "cached-1",
					Timestamp: "2026-03-08 12:34:56.000 UTC",
					Sentiment: "negative",
				},
			},
		},
	}); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	var gotOpts api.FeedbackQueryOptions
	client := &stubFeedbackClient{
		getFeedbackFn: func(ctx context.Context, db, schema, agentName string, opts api.FeedbackQueryOptions) ([]api.FeedbackRecord, error) {
			gotOpts = opts
			return []api.FeedbackRecord{}, nil
		},
	}

	origBuild := buildFeedbackClientAndCfg
	t.Cleanup(func() { buildFeedbackClientAndCfg = origBuild })
	buildFeedbackClientAndCfg = func(opts *RootOptions) (feedbackClient, auth.Config, error) {
		return client, auth.Config{Database: "DB", Schema: "SC"}, nil
	}

	cmd := newFeedbackCmd(&RootOptions{Database: "DB", Schema: "SC"})
	cmd.SetArgs([]string{"my-agent", "--infer-negative", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if gotOpts.Since != "2026-03-08 12:34:56.000 UTC" {
		t.Fatalf("Since = %q, want cached latest timestamp", gotOpts.Since)
	}
	if gotOpts.ExplicitSince != "" {
		t.Fatalf("ExplicitSince = %q, want empty so explicit feedback is reloaded", gotOpts.ExplicitSince)
	}
	if gotOpts.RequestSince != "2026-03-08 12:34:56.000 UTC" {
		t.Fatalf("RequestSince = %q, want cached latest timestamp", gotOpts.RequestSince)
	}
}

func TestFeedbackInferNegativeRemoteUsesRequestOnlyDiffCursor(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Setenv("HOME", dir)

	if err := os.WriteFile(".coragent.toml", []byte(`[feedback.remote]
enabled = true
database = "REMOTE_DB"
schema = "REMOTE_SCHEMA"
table = "AGENT_FEEDBACK"
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var gotOpts api.FeedbackQueryOptions
	client := &stubFeedbackClient{
		feedbackTableExistsFn: func(ctx context.Context, db, schema, table string) (bool, error) {
			return true, nil
		},
		feedbackInferenceColumnsFn: func(ctx context.Context, db, schema, table string) (bool, error) {
			return true, nil
		},
		getLatestFeedbackEventTsFn: func(ctx context.Context, db, schema, table, agentName string) (string, error) {
			return "2026-03-08 12:34:56.000 UTC", nil
		},
		syncFeedbackFromEventsToFn: func(ctx context.Context, srcDB, srcSchema, agentName, dstDB, dstSchema, dstTable string, opts api.FeedbackQueryOptions) error {
			gotOpts = opts
			return nil
		},
		getFeedbackFromTableFn: func(ctx context.Context, db, schema, table, agentName string) ([]api.FeedbackTableRow, error) {
			return nil, nil
		},
	}

	origBuild := buildFeedbackClientAndCfg
	t.Cleanup(func() { buildFeedbackClientAndCfg = origBuild })
	buildFeedbackClientAndCfg = func(opts *RootOptions) (feedbackClient, auth.Config, error) {
		return client, auth.Config{Database: "DB", Schema: "SC"}, nil
	}

	cmd := newFeedbackCmd(&RootOptions{Database: "DB", Schema: "SC"})
	cmd.SetArgs([]string{"my-agent", "--infer-negative", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if gotOpts.ExplicitSince != "" {
		t.Fatalf("ExplicitSince = %q, want empty so explicit feedback is reloaded", gotOpts.ExplicitSince)
	}
	if gotOpts.RequestSince != "2026-03-08 12:34:56.000 UTC" {
		t.Fatalf("RequestSince = %q, want remote latest timestamp", gotOpts.RequestSince)
	}
}

func TestFeedbackInferNegativeUsesConfiguredJudgeModel(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Setenv("HOME", dir)

	if err := os.WriteFile(".coragent.toml", []byte(`[feedback]
judge_model = "custom-feedback-model"
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var gotOpts api.FeedbackQueryOptions
	client := &stubFeedbackClient{
		getFeedbackFn: func(ctx context.Context, db, schema, agentName string, opts api.FeedbackQueryOptions) ([]api.FeedbackRecord, error) {
			gotOpts = opts
			return []api.FeedbackRecord{}, nil
		},
	}

	origBuild := buildFeedbackClientAndCfg
	t.Cleanup(func() { buildFeedbackClientAndCfg = origBuild })
	buildFeedbackClientAndCfg = func(opts *RootOptions) (feedbackClient, auth.Config, error) {
		return client, auth.Config{Database: "DB", Schema: "SC"}, nil
	}

	cmd := newFeedbackCmd(&RootOptions{Database: "DB", Schema: "SC"})
	cmd.SetArgs([]string{"my-agent", "--infer-negative", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if gotOpts.JudgeModel != "custom-feedback-model" {
		t.Fatalf("JudgeModel = %q, want custom-feedback-model", gotOpts.JudgeModel)
	}
}

func TestFeedbackInferNegativeCachesPositiveInferredRecord(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Setenv("HOME", dir)

	client := &stubFeedbackClient{
		getFeedbackFn: func(ctx context.Context, db, schema, agentName string, opts api.FeedbackQueryOptions) ([]api.FeedbackRecord, error) {
			return []api.FeedbackRecord{{
				RecordID:        "inferred-positive-1",
				Timestamp:       "2026-03-09 10:00:00.000 UTC",
				Sentiment:       "positive",
				SentimentSource: "inferred",
				SentimentReason: "the answer substantially met the user's goal",
				Question:        "show me sales",
				Response:        "here are the sales figures",
			}}, nil
		},
	}

	origBuild := buildFeedbackClientAndCfg
	t.Cleanup(func() { buildFeedbackClientAndCfg = origBuild })
	buildFeedbackClientAndCfg = func(opts *RootOptions) (feedbackClient, auth.Config, error) {
		return client, auth.Config{Database: "DB", Schema: "SC"}, nil
	}

	cmd := newFeedbackCmd(&RootOptions{Database: "DB", Schema: "SC"})
	cmd.SetArgs([]string{"my-agent", "--infer-negative", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	cache, err := feedbackcache.Load("my-agent")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cache.Records) != 1 {
		t.Fatalf("len(cache.Records) = %d, want 1", len(cache.Records))
	}
	if cache.Records[0].Sentiment != "positive" {
		t.Fatalf("cached sentiment = %q, want positive", cache.Records[0].Sentiment)
	}
	if cache.Records[0].SentimentSource != "inferred" {
		t.Fatalf("cached sentiment_source = %q, want inferred", cache.Records[0].SentimentSource)
	}
}

func TestFeedbackInferNegativeRemoteRequiresInitWhenColumnsMissing(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Setenv("HOME", dir)

	if err := os.WriteFile(".coragent.toml", []byte(`[feedback.remote]
enabled = true
database = "REMOTE_DB"
schema = "REMOTE_SCHEMA"
table = "AGENT_FEEDBACK"
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	client := &stubFeedbackClient{
		feedbackTableExistsFn: func(ctx context.Context, db, schema, table string) (bool, error) {
			return true, nil
		},
		feedbackInferenceColumnsFn: func(ctx context.Context, db, schema, table string) (bool, error) {
			return false, nil
		},
		syncFeedbackFromEventsToFn: func(ctx context.Context, srcDB, srcSchema, agentName, dstDB, dstSchema, dstTable string, opts api.FeedbackQueryOptions) error {
			t.Fatal("SyncFeedbackFromEventsToTable should not be called when infer-negative columns are missing")
			return nil
		},
	}

	origBuild := buildFeedbackClientAndCfg
	t.Cleanup(func() { buildFeedbackClientAndCfg = origBuild })
	buildFeedbackClientAndCfg = func(opts *RootOptions) (feedbackClient, auth.Config, error) {
		return client, auth.Config{Database: "DB", Schema: "SC"}, nil
	}

	cmd := newFeedbackCmd(&RootOptions{Database: "DB", Schema: "SC"})
	cmd.SetArgs([]string{"my-agent", "--infer-negative", "--json"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when infer-negative columns are missing")
	}
	if !IsUserError(err) {
		t.Fatalf("expected UserError, got: %v", err)
	}
	if !strings.Contains(err.Error(), "run `coragent feedback --init` to recreate it") {
		t.Fatalf("expected init guidance in error, got: %v", err)
	}
}

func TestFeedbackShowsProgressInLocalMode(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Setenv("HOME", dir)

	client := &stubFeedbackClient{
		getFeedbackFn: func(ctx context.Context, db, schema, agentName string, opts api.FeedbackQueryOptions) ([]api.FeedbackRecord, error) {
			return []api.FeedbackRecord{{
				RecordID:  "r1",
				Timestamp: "2026-03-08 00:00:00.000 UTC",
				UserName:  "alice",
				Sentiment: "negative",
			}}, nil
		},
	}

	origBuild := buildFeedbackClientAndCfg
	t.Cleanup(func() { buildFeedbackClientAndCfg = origBuild })
	buildFeedbackClientAndCfg = func(opts *RootOptions) (feedbackClient, auth.Config, error) {
		return client, auth.Config{Database: "DB", Schema: "SC"}, nil
	}

	var out bytes.Buffer
	cmd := newFeedbackCmd(&RootOptions{Database: "DB", Schema: "SC"})
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"my-agent", "-y"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Loading local feedback cache...") {
		t.Fatalf("expected local cache progress message, got:\n%s", got)
	}
	if !strings.Contains(got, "Fetching feedback from observability events...") {
		t.Fatalf("expected fetch progress message, got:\n%s", got)
	}
	if !strings.Contains(got, "Saving refreshed feedback cache...") {
		t.Fatalf("expected save progress message, got:\n%s", got)
	}
	if !strings.Contains(got, "Preparing feedback records for display...") {
		t.Fatalf("expected display progress message, got:\n%s", got)
	}
}

func TestFeedbackJSONSuppressesProgressOutput(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Setenv("HOME", dir)

	client := &stubFeedbackClient{
		getFeedbackFn: func(ctx context.Context, db, schema, agentName string, opts api.FeedbackQueryOptions) ([]api.FeedbackRecord, error) {
			return []api.FeedbackRecord{{
				RecordID:  "r1",
				Timestamp: "2026-03-08 00:00:00.000 UTC",
				UserName:  "alice",
				Sentiment: "negative",
			}}, nil
		},
	}

	origBuild := buildFeedbackClientAndCfg
	t.Cleanup(func() { buildFeedbackClientAndCfg = origBuild })
	buildFeedbackClientAndCfg = func(opts *RootOptions) (feedbackClient, auth.Config, error) {
		return client, auth.Config{Database: "DB", Schema: "SC"}, nil
	}

	var out bytes.Buffer
	cmd := newFeedbackCmd(&RootOptions{Database: "DB", Schema: "SC"})
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"my-agent", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var decoded []map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("expected pure JSON output, got error %v\noutput=%s", err, out.String())
	}
	if len(decoded) != 1 {
		t.Fatalf("len(decoded) = %d, want 1", len(decoded))
	}
}

func TestMarshalFeedbackJSON_EmptyReturnsArray(t *testing.T) {
	tests := []struct {
		name    string
		records []feedbackcache.Record
	}{
		{name: "nil slice", records: nil},
		{name: "empty slice", records: []feedbackcache.Record{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := marshalFeedbackJSON(tt.records)
			if err != nil {
				t.Fatalf("marshalFeedbackJSON() error = %v", err)
			}
			if string(got) != "[]" {
				t.Fatalf("marshalFeedbackJSON() = %s, want []", got)
			}
		})
	}
}

func TestMarshalFeedbackJSON_NonEmptyReturnsArray(t *testing.T) {
	got, err := marshalFeedbackJSON([]feedbackcache.Record{
		{
			Checked: false,
			FeedbackRecord: api.FeedbackRecord{
				RecordID:  "r1",
				Timestamp: "2026-03-08 00:00:00.000 UTC",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshalFeedbackJSON() error = %v", err)
	}

	var decoded []map[string]any
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("unmarshal output: %v\noutput=%s", err, got)
	}
	if len(decoded) != 1 {
		t.Fatalf("len(decoded) = %d, want 1", len(decoded))
	}
	if decoded[0]["record_id"] != "r1" {
		t.Fatalf("decoded[0][record_id] = %v, want r1", decoded[0]["record_id"])
	}
}

func TestFormatToolChain(t *testing.T) {
	got := formatToolChain([]api.ToolUseInfo{
		{ToolType: "cortex_analyst_text_to_sql", ToolName: "sample_semantic_view"},
		{ToolType: "search"},
		{ToolName: "custom_tool"},
	})
	want := "cortex_analyst_text_to_sql (sample_semantic_view) → search → custom_tool"
	if got != want {
		t.Fatalf("formatToolChain() = %q, want %q", got, want)
	}
}

func TestPrintOneRecord_ShowsFullResponseWithoutTruncation(t *testing.T) {
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)

	longResponse := strings.Repeat("a", 1200) + "\n" + strings.Repeat("b", 1200)
	firstLine := strings.Repeat("a", 1200)
	secondLine := strings.Repeat("b", 1200)
	printOneRecord(cmd, 1, 1, feedbackcache.Record{
		FeedbackRecord: api.FeedbackRecord{
			Timestamp: "2026-03-08 00:00:00.000 UTC",
			UserName:  "alice",
			Sentiment: "negative",
			Response:  longResponse,
		},
	}, false, true)

	got := out.String()
	if !strings.Contains(got, "      "+firstLine) || !strings.Contains(got, "      "+secondLine) {
		t.Fatalf("expected full response in output, got:\n%s", got)
	}
	if strings.Contains(got, "...(truncated)") {
		t.Fatalf("did not expect truncation marker, got:\n%s", got)
	}
}

func TestPrintOneRecord_ShowsInferenceMetadata(t *testing.T) {
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)

	printOneRecord(cmd, 1, 1, feedbackcache.Record{
		FeedbackRecord: api.FeedbackRecord{
			Timestamp:       "2026-03-08 00:00:00.000 UTC",
			UserName:        "alice",
			Sentiment:       "negative",
			SentimentSource: "inferred",
			SentimentReason: "The response did not answer the user's request.",
		},
	}, false, true)

	got := out.String()
	if !strings.Contains(got, "Source:    inferred") {
		t.Fatalf("expected inferred source in output, got:\n%s", got)
	}
	if !strings.Contains(got, "Reason:    The response did not answer the user's request.") {
		t.Fatalf("expected inference reason in output, got:\n%s", got)
	}
}
