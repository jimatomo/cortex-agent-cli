package cli

import (
	"os"
	"path/filepath"
	"testing"

	"coragent/internal/api"
	"coragent/internal/config"
)

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

func TestMergeRemoteRows_FilterCheckedByDefault(t *testing.T) {
	rows := []api.FeedbackTableRow{
		{
			FeedbackRecord: api.FeedbackRecord{RecordID: "r1", Comment: "unchecked"},
			Checked:        false,
		},
		{
			FeedbackRecord: api.FeedbackRecord{RecordID: "r2", Comment: "checked"},
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
