package feedbackcache_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"coragent/internal/api"
	"coragent/internal/feedbackcache"
)

// setHomeDir redirects os.UserHomeDir() to dir for the duration of the test by
// setting the HOME environment variable.
func setHomeDir(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
}

func TestMerge_NewRecordsAdded(t *testing.T) {
	c := &feedbackcache.Cache{}
	fresh := []api.FeedbackRecord{
		{RecordID: "r1", Timestamp: "2024-01-01", Sentiment: "negative"},
		{RecordID: "r2", Timestamp: "2024-01-02", Sentiment: "positive"},
	}
	c.Merge(fresh)

	if len(c.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(c.Records))
	}
	for _, r := range c.Records {
		if r.Checked {
			t.Errorf("record %q should have Checked=false after initial merge", r.RecordID)
		}
	}
}

func TestMerge_DuplicatesSkipped(t *testing.T) {
	c := &feedbackcache.Cache{}
	rec := api.FeedbackRecord{RecordID: "r1", Sentiment: "negative"}
	c.Merge([]api.FeedbackRecord{rec})
	c.Merge([]api.FeedbackRecord{rec})

	if len(c.Records) != 1 {
		t.Errorf("expected 1 record after duplicate merge, got %d", len(c.Records))
	}
}

func TestMerge_CheckedStatePreserved(t *testing.T) {
	c := &feedbackcache.Cache{}
	c.Merge([]api.FeedbackRecord{{RecordID: "r1", Sentiment: "negative"}})
	c.Records[0].Checked = true
	c.Merge([]api.FeedbackRecord{{RecordID: "r1", Sentiment: "negative"}})

	if len(c.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(c.Records))
	}
	if !c.Records[0].Checked {
		t.Error("Checked state should be preserved after re-merge")
	}
}

func TestMerge_SyntheticRecordID(t *testing.T) {
	c := &feedbackcache.Cache{}
	rec := api.FeedbackRecord{
		RecordID:  "",
		Timestamp: "2024-01-01 12:00:00",
		UserName:  "alice",
		Sentiment: "negative",
	}
	c.Merge([]api.FeedbackRecord{rec})

	if len(c.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(c.Records))
	}
	if c.Records[0].RecordID == "" {
		t.Error("expected synthetic RecordID to be set")
	}
	c.Merge([]api.FeedbackRecord{rec})
	if len(c.Records) != 1 {
		t.Errorf("expected 1 record after duplicate merge with synthetic ID, got %d", len(c.Records))
	}
}

func TestMerge_IncrementalGrowth(t *testing.T) {
	c := &feedbackcache.Cache{}
	c.Merge([]api.FeedbackRecord{{RecordID: "r1"}, {RecordID: "r2"}})
	c.Merge([]api.FeedbackRecord{{RecordID: "r2"}, {RecordID: "r3"}})

	if len(c.Records) != 3 {
		t.Errorf("expected 3 records, got %d", len(c.Records))
	}
}

func TestSaveAndLoad(t *testing.T) {
	setHomeDir(t, t.TempDir())

	original := &feedbackcache.Cache{
		Records: []feedbackcache.Record{
			{Checked: true, FeedbackRecord: api.FeedbackRecord{RecordID: "r1", Sentiment: "negative", Comment: "not helpful"}},
			{Checked: false, FeedbackRecord: api.FeedbackRecord{RecordID: "r2", Sentiment: "positive"}},
		},
	}

	if err := feedbackcache.Save("test-agent", original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := feedbackcache.Load("test-agent")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(loaded.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(loaded.Records))
	}
	if loaded.Records[0].RecordID != "r1" {
		t.Errorf("record[0].RecordID = %q, want %q", loaded.Records[0].RecordID, "r1")
	}
	if !loaded.Records[0].Checked {
		t.Error("record[0].Checked should be true")
	}
	if loaded.Records[1].Checked {
		t.Error("record[1].Checked should be false")
	}
}

func TestLoad_Missing(t *testing.T) {
	setHomeDir(t, t.TempDir())

	c, err := feedbackcache.Load("no-such-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil cache")
	}
	if len(c.Records) != 0 {
		t.Errorf("expected 0 records, got %d", len(c.Records))
	}
}

func TestLoad_CorruptFile(t *testing.T) {
	home := t.TempDir()
	setHomeDir(t, home)

	dir := filepath.Join(home, ".coragent", "feedback")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "corrupt-agent.json"), []byte("not json {{{{"), 0o600); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}

	c, err := feedbackcache.Load("corrupt-agent")
	if err != nil {
		t.Fatalf("unexpected error for corrupt file: %v", err)
	}
	if len(c.Records) != 0 {
		t.Errorf("expected empty cache after corrupt file, got %d records", len(c.Records))
	}
}

func TestCachePath_SafeCharacters(t *testing.T) {
	setHomeDir(t, t.TempDir())

	path, err := feedbackcache.CachePath("db/schema:my agent")
	if err != nil {
		t.Fatalf("CachePath: %v", err)
	}

	base := filepath.Base(path)
	for _, unsafe := range []string{"/", "\\", ":", " "} {
		if containsStr(base, unsafe) {
			t.Errorf("cache file name %q contains unsafe char %q", base, unsafe)
		}
	}
}

func TestSave_AtomicWrite(t *testing.T) {
	home := t.TempDir()
	setHomeDir(t, home)

	c := &feedbackcache.Cache{
		Records: []feedbackcache.Record{
			{FeedbackRecord: api.FeedbackRecord{RecordID: "x1"}},
		},
	}
	if err := feedbackcache.Save("atomic-agent", c); err != nil {
		t.Fatalf("save: %v", err)
	}

	dir := filepath.Join(home, ".coragent", "feedback")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("leftover tmp file found: %s", e.Name())
		}
	}

	data, err := os.ReadFile(filepath.Join(dir, "atomic-agent.json"))
	if err != nil {
		t.Fatalf("readfile: %v", err)
	}
	var check feedbackcache.Cache
	if err := json.Unmarshal(data, &check); err != nil {
		t.Fatalf("unmarshal written file: %v", err)
	}
	if len(check.Records) != 1 || check.Records[0].RecordID != "x1" {
		t.Errorf("unexpected written content: %+v", check)
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
