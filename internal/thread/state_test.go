package thread

import (
	"testing"
	"time"
)

func TestStateKey(t *testing.T) {
	tests := []struct {
		name    string
		account string
		db      string
		schema  string
		agent   string
		want    string
	}{
		{"all uppercase", "ACCT", "DB", "SCH", "AGENT", "ACCT/DB/SCH/AGENT"},
		{"normalizes to upper", "acct", "db", "sch", "agent", "ACCT/DB/SCH/AGENT"},
		{"mixed case", "Acct", "myDb", "mySchema", "myAgent", "ACCT/MYDB/MYSCHEMA/MYAGENT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stateKey(tt.account, tt.db, tt.schema, tt.agent)
			if got != tt.want {
				t.Errorf("stateKey(%q,%q,%q,%q) = %q, want %q",
					tt.account, tt.db, tt.schema, tt.agent, got, tt.want)
			}
		})
	}
}

func TestLoadState_NoFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store, err := LoadState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil store")
	}
	if store.Threads == nil {
		t.Fatal("expected non-nil Threads map")
	}
	if len(store.Threads) != 0 {
		t.Errorf("expected empty Threads, got %d", len(store.Threads))
	}
}

func TestSaveAndLoadState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	store := &StateStore{
		Threads: map[string][]ThreadState{
			"ACCT/DB/SCH/AGENT": {
				{ThreadID: "t1", LastMessageID: 100, LastUsed: time.Now().Truncate(time.Second), Summary: "hello"},
			},
		},
	}
	if err := store.Save(); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	loaded, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState error: %v", err)
	}
	threads := loaded.Threads["ACCT/DB/SCH/AGENT"]
	if len(threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(threads))
	}
	if threads[0].ThreadID != "t1" {
		t.Errorf("ThreadID = %q, want %q", threads[0].ThreadID, "t1")
	}
	if threads[0].LastMessageID != 100 {
		t.Errorf("LastMessageID = %d, want %d", threads[0].LastMessageID, 100)
	}
	if threads[0].Summary != "hello" {
		t.Errorf("Summary = %q, want %q", threads[0].Summary, "hello")
	}
}

func TestGetThreads_CaseInsensitive(t *testing.T) {
	store := &StateStore{
		Threads: map[string][]ThreadState{
			"ACCT/DB/SCH/AGENT": {
				{ThreadID: "t1"},
			},
		},
	}
	// GetThreads normalizes keys via stateKey
	got := store.GetThreads("acct", "db", "sch", "agent")
	if len(got) != 1 {
		t.Errorf("expected 1 thread, got %d", len(got))
	}
}

func TestFindThread(t *testing.T) {
	store := &StateStore{
		Threads: map[string][]ThreadState{
			"ACCT/DB/SCH/AGENT": {
				{ThreadID: "t1", Summary: "first"},
				{ThreadID: "t2", Summary: "second"},
			},
		},
	}

	t.Run("found", func(t *testing.T) {
		got := store.FindThread("ACCT", "DB", "SCH", "AGENT", "t2")
		if got == nil {
			t.Fatal("expected non-nil")
		}
		if got.Summary != "second" {
			t.Errorf("Summary = %q, want %q", got.Summary, "second")
		}
	})

	t.Run("not found", func(t *testing.T) {
		got := store.FindThread("ACCT", "DB", "SCH", "AGENT", "t99")
		if got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("different agent", func(t *testing.T) {
		got := store.FindThread("ACCT", "DB", "SCH", "OTHER", "t1")
		if got != nil {
			t.Errorf("expected nil for different agent, got %+v", got)
		}
	})
}

func TestAddOrUpdateThread_Update(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	store := &StateStore{
		Threads: map[string][]ThreadState{
			"ACCT/DB/SCH/AGENT": {
				{ThreadID: "t1", LastMessageID: 10, LastUsed: now, Summary: "original"},
			},
		},
	}

	later := now.Add(time.Hour)
	store.AddOrUpdateThread("ACCT", "DB", "SCH", "AGENT", ThreadState{
		ThreadID:      "t1",
		LastMessageID: 20,
		LastUsed:      later,
		Summary:       "", // empty summary should keep original
	})

	threads := store.Threads["ACCT/DB/SCH/AGENT"]
	if len(threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(threads))
	}
	if threads[0].LastMessageID != 20 {
		t.Errorf("LastMessageID = %d, want 20", threads[0].LastMessageID)
	}
	if threads[0].Summary != "original" {
		t.Errorf("Summary = %q, want %q (should keep original)", threads[0].Summary, "original")
	}
}

func TestAddOrUpdateThread_NewSummary(t *testing.T) {
	store := &StateStore{
		Threads: map[string][]ThreadState{
			"ACCT/DB/SCH/AGENT": {
				{ThreadID: "t1", Summary: "original"},
			},
		},
	}

	store.AddOrUpdateThread("ACCT", "DB", "SCH", "AGENT", ThreadState{
		ThreadID: "t1",
		Summary:  "updated",
	})

	threads := store.Threads["ACCT/DB/SCH/AGENT"]
	if threads[0].Summary != "updated" {
		t.Errorf("Summary = %q, want %q", threads[0].Summary, "updated")
	}
}

func TestAddOrUpdateThread_NewThread(t *testing.T) {
	store := &StateStore{
		Threads: make(map[string][]ThreadState),
	}

	store.AddOrUpdateThread("ACCT", "DB", "SCH", "AGENT", ThreadState{
		ThreadID: "t1",
		Summary:  "new thread",
	})

	threads := store.Threads["ACCT/DB/SCH/AGENT"]
	if len(threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(threads))
	}
	if threads[0].ThreadID != "t1" {
		t.Errorf("ThreadID = %q, want %q", threads[0].ThreadID, "t1")
	}
}

func TestDeleteThread(t *testing.T) {
	store := &StateStore{
		Threads: map[string][]ThreadState{
			"ACCT/DB/SCH/AGENT": {
				{ThreadID: "t1"},
				{ThreadID: "t2"},
				{ThreadID: "t3"},
			},
		},
	}

	store.DeleteThread("ACCT", "DB", "SCH", "AGENT", "t2")

	threads := store.Threads["ACCT/DB/SCH/AGENT"]
	if len(threads) != 2 {
		t.Fatalf("expected 2 threads, got %d", len(threads))
	}
	for _, ts := range threads {
		if ts.ThreadID == "t2" {
			t.Error("t2 should have been deleted")
		}
	}
}

func TestDeleteThread_NotFound(t *testing.T) {
	store := &StateStore{
		Threads: map[string][]ThreadState{
			"ACCT/DB/SCH/AGENT": {
				{ThreadID: "t1"},
			},
		},
	}

	// Should not panic or error
	store.DeleteThread("ACCT", "DB", "SCH", "AGENT", "t99")

	threads := store.Threads["ACCT/DB/SCH/AGENT"]
	if len(threads) != 1 {
		t.Errorf("expected 1 thread unchanged, got %d", len(threads))
	}
}

func TestGetAllThreads(t *testing.T) {
	store := &StateStore{
		Threads: map[string][]ThreadState{
			"ACCT/DB1/SCH1/AGENT1": {{ThreadID: "t1"}},
			"ACCT/DB2/SCH2/AGENT2": {{ThreadID: "t2"}, {ThreadID: "t3"}},
		},
	}

	got := store.GetAllThreads()
	if len(got) != 2 {
		t.Errorf("expected 2 keys, got %d", len(got))
	}
	if len(got["ACCT/DB1/SCH1/AGENT1"]) != 1 {
		t.Errorf("expected 1 thread for AGENT1")
	}
	if len(got["ACCT/DB2/SCH2/AGENT2"]) != 2 {
		t.Errorf("expected 2 threads for AGENT2")
	}
}
