package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"coragent/internal/api"
)

// setHomeDir redirects os.UserHomeDir() to dir for the duration of the test by
// setting the HOME environment variable (os.UserHomeDir uses $HOME on Linux).
func setHomeDir(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
}

// TestFeedbackCacheMerge_NewRecordsAdded verifies that fresh records not yet in
// the cache are appended with Checked=false.
func TestFeedbackCacheMerge_NewRecordsAdded(t *testing.T) {
	c := &FeedbackCache{}
	fresh := []api.FeedbackRecord{
		{RecordID: "r1", Timestamp: "2024-01-01", Sentiment: "negative"},
		{RecordID: "r2", Timestamp: "2024-01-02", Sentiment: "positive"},
	}
	c.merge(fresh)

	if len(c.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(c.Records))
	}
	for _, r := range c.Records {
		if r.Checked {
			t.Errorf("record %q should have Checked=false after initial merge", r.RecordID)
		}
	}
}

// TestFeedbackCacheMerge_DuplicatesSkipped verifies that records already in the
// cache are not re-added on a second merge.
func TestFeedbackCacheMerge_DuplicatesSkipped(t *testing.T) {
	c := &FeedbackCache{}
	rec := api.FeedbackRecord{RecordID: "r1", Sentiment: "negative"}
	c.merge([]api.FeedbackRecord{rec})
	c.merge([]api.FeedbackRecord{rec}) // second merge with same record

	if len(c.Records) != 1 {
		t.Errorf("expected 1 record after duplicate merge, got %d", len(c.Records))
	}
}

// TestFeedbackCacheMerge_CheckedStatePreserved verifies that a record's Checked
// state is not reset when the same record appears in a fresh fetch.
func TestFeedbackCacheMerge_CheckedStatePreserved(t *testing.T) {
	c := &FeedbackCache{}
	c.merge([]api.FeedbackRecord{{RecordID: "r1", Sentiment: "negative"}})

	// Simulate user marking it as checked.
	c.Records[0].Checked = true

	// A second merge with the same record ID must not reset Checked.
	c.merge([]api.FeedbackRecord{{RecordID: "r1", Sentiment: "negative"}})

	if len(c.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(c.Records))
	}
	if !c.Records[0].Checked {
		t.Error("Checked state should be preserved after re-merge")
	}
}

// TestFeedbackCacheMerge_SyntheticRecordID verifies that when RecordID is empty
// a synthetic key (timestamp + "|" + user) is assigned so that uniqueness is
// maintained across merges.
func TestFeedbackCacheMerge_SyntheticRecordID(t *testing.T) {
	c := &FeedbackCache{}
	rec := api.FeedbackRecord{
		RecordID:  "", // empty — should be synthesised
		Timestamp: "2024-01-01 12:00:00",
		UserName:  "alice",
		Sentiment: "negative",
	}
	c.merge([]api.FeedbackRecord{rec})

	if len(c.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(c.Records))
	}
	if c.Records[0].RecordID == "" {
		t.Error("expected synthetic RecordID to be set")
	}

	// Second merge with the same record should be deduplicated.
	c.merge([]api.FeedbackRecord{rec})
	if len(c.Records) != 1 {
		t.Errorf("expected 1 record after duplicate merge with synthetic ID, got %d", len(c.Records))
	}
}

// TestFeedbackCacheMerge_IncrementalGrowth verifies that repeated merges with
// partly overlapping record sets grow the cache correctly.
func TestFeedbackCacheMerge_IncrementalGrowth(t *testing.T) {
	c := &FeedbackCache{}
	c.merge([]api.FeedbackRecord{
		{RecordID: "r1"},
		{RecordID: "r2"},
	})
	c.merge([]api.FeedbackRecord{
		{RecordID: "r2"}, // duplicate
		{RecordID: "r3"}, // new
	})

	if len(c.Records) != 3 {
		t.Errorf("expected 3 records, got %d", len(c.Records))
	}
}

// TestSaveAndLoadFeedbackCache verifies the round-trip: save writes to disk and
// load reads it back correctly.
func TestSaveAndLoadFeedbackCache(t *testing.T) {
	setHomeDir(t, t.TempDir())

	original := &FeedbackCache{
		Records: []CachedFeedbackRecord{
			{Checked: true, FeedbackRecord: api.FeedbackRecord{RecordID: "r1", Sentiment: "negative", Comment: "not helpful"}},
			{Checked: false, FeedbackRecord: api.FeedbackRecord{RecordID: "r2", Sentiment: "positive"}},
		},
	}

	if err := saveFeedbackCache("test-agent", original); err != nil {
		t.Fatalf("saveFeedbackCache: %v", err)
	}

	loaded, err := loadFeedbackCache("test-agent")
	if err != nil {
		t.Fatalf("loadFeedbackCache: %v", err)
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

// TestLoadFeedbackCache_Missing verifies that loading a cache for an agent with
// no existing file returns an empty FeedbackCache without error.
func TestLoadFeedbackCache_Missing(t *testing.T) {
	setHomeDir(t, t.TempDir())

	c, err := loadFeedbackCache("no-such-agent")
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

// TestLoadFeedbackCache_CorruptFile verifies that a corrupt cache file is silently
// discarded and an empty cache is returned rather than an error.
func TestLoadFeedbackCache_CorruptFile(t *testing.T) {
	home := t.TempDir()
	setHomeDir(t, home)

	// Write a corrupt (non-JSON) cache file.
	dir := filepath.Join(home, ".coragent", "feedback")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "corrupt-agent.json"), []byte("not json {{{{"), 0o600); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}

	c, err := loadFeedbackCache("corrupt-agent")
	if err != nil {
		t.Fatalf("unexpected error for corrupt file: %v", err)
	}
	if len(c.Records) != 0 {
		t.Errorf("expected empty cache after corrupt file, got %d records", len(c.Records))
	}
}

// TestFeedbackCachePath_SafeCharacters verifies that agent names with characters
// that are unsafe in file names are sanitised.
func TestFeedbackCachePath_SafeCharacters(t *testing.T) {
	setHomeDir(t, t.TempDir())

	path, err := feedbackCachePath("db/schema:my agent")
	if err != nil {
		t.Fatalf("feedbackCachePath: %v", err)
	}

	base := filepath.Base(path)
	for _, unsafe := range []string{"/", "\\", ":", " "} {
		if contains(base, unsafe) {
			t.Errorf("cache file name %q contains unsafe char %q", base, unsafe)
		}
	}
}

// TestSaveFeedbackCache_AtomicWrite verifies that the save creates a real file
// (not only a .tmp) on disk.
func TestSaveFeedbackCache_AtomicWrite(t *testing.T) {
	home := t.TempDir()
	setHomeDir(t, home)

	c := &FeedbackCache{
		Records: []CachedFeedbackRecord{
			{FeedbackRecord: api.FeedbackRecord{RecordID: "x1"}},
		},
	}
	if err := saveFeedbackCache("atomic-agent", c); err != nil {
		t.Fatalf("save: %v", err)
	}

	// The .tmp file should have been renamed away.
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

	// The main file should contain valid JSON.
	data, err := os.ReadFile(filepath.Join(dir, "atomic-agent.json"))
	if err != nil {
		t.Fatalf("readfile: %v", err)
	}
	var check FeedbackCache
	if err := json.Unmarshal(data, &check); err != nil {
		t.Fatalf("unmarshal written file: %v", err)
	}
	if len(check.Records) != 1 || check.Records[0].RecordID != "x1" {
		t.Errorf("unexpected written content: %+v", check)
	}
}

// contains is a helper that reports whether s contains substr.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
