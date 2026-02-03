package thread

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ThreadState tracks a conversation thread for an agent.
type ThreadState struct {
	ThreadID      string    `json:"thread_id"`
	LastMessageID int64     `json:"last_message_id"`
	LastUsed      time.Time `json:"last_used"`
	Summary       string    `json:"summary"` // First message or auto-generated summary
}

// StateStore holds thread state for all agents.
type StateStore struct {
	// Key: "ACCOUNT/DATABASE/SCHEMA/AGENT"
	Threads map[string][]ThreadState `json:"threads"`
}

// LoadState loads the thread state from disk.
func LoadState() (*StateStore, error) {
	path := stateFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &StateStore{Threads: make(map[string][]ThreadState)}, nil
		}
		return nil, err
	}

	var store StateStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, err
	}
	if store.Threads == nil {
		store.Threads = make(map[string][]ThreadState)
	}
	return &store, nil
}

// Save persists the thread state to disk.
func (s *StateStore) Save() error {
	path := stateFilePath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// GetThreads returns all threads for a specific agent.
func (s *StateStore) GetThreads(account, db, schema, agent string) []ThreadState {
	key := stateKey(account, db, schema, agent)
	return s.Threads[key]
}

// FindThread finds a specific thread by ID for an agent.
func (s *StateStore) FindThread(account, db, schema, agent string, threadID string) *ThreadState {
	key := stateKey(account, db, schema, agent)
	threads := s.Threads[key]
	for i := range threads {
		if threads[i].ThreadID == threadID {
			return &threads[i]
		}
	}
	return nil
}

// AddOrUpdateThread adds a new thread or updates an existing one.
func (s *StateStore) AddOrUpdateThread(account, db, schema, agent string, state ThreadState) {
	key := stateKey(account, db, schema, agent)
	threads := s.Threads[key]

	// Look for existing thread to update
	for i := range threads {
		if threads[i].ThreadID == state.ThreadID {
			threads[i].LastMessageID = state.LastMessageID
			threads[i].LastUsed = state.LastUsed
			// Keep original summary unless new one is provided
			if state.Summary != "" {
				threads[i].Summary = state.Summary
			}
			s.Threads[key] = threads
			return
		}
	}

	// Add new thread
	s.Threads[key] = append(threads, state)
}

// DeleteThread removes a thread from the state.
func (s *StateStore) DeleteThread(account, db, schema, agent string, threadID string) {
	key := stateKey(account, db, schema, agent)
	threads := s.Threads[key]

	for i := range threads {
		if threads[i].ThreadID == threadID {
			s.Threads[key] = append(threads[:i], threads[i+1:]...)
			return
		}
	}
}

// stateKey generates a unique key for an agent.
func stateKey(account, db, schema, agent string) string {
	return strings.ToUpper(account) + "/" +
		strings.ToUpper(db) + "/" +
		strings.ToUpper(schema) + "/" +
		strings.ToUpper(agent)
}

// stateFilePath returns the path to the state file.
func stateFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".coragent", "threads.json")
}
