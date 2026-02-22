// Package feedbackcache manages the on-disk feedback record cache stored at
// ~/.coragent/feedback/<agent>.json.
package feedbackcache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"coragent/internal/api"
)

// Record wraps a FeedbackRecord with a user-managed checked state.
// The record is uniquely identified by the embedded FeedbackRecord.RecordID.
type Record struct {
	Checked bool `json:"checked"`
	api.FeedbackRecord
}

// Cache is the on-disk structure stored at ~/.coragent/feedback/<agent>.json.
type Cache struct {
	Records []Record `json:"records"`
}

// CachePath returns the path to the cache file for agentName and ensures the
// parent directory exists.
func CachePath(agentName string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	dir := filepath.Join(home, ".coragent", "feedback")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}
	// Sanitise agent name so it is safe to use as a file name.
	safe := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_").Replace(agentName)
	return filepath.Join(dir, safe+".json"), nil
}

// Load reads the cache file for agentName. Returns an empty Cache if the file
// does not exist yet.
func Load(agentName string) (*Cache, error) {
	path, err := CachePath(agentName)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Cache{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read cache: %w", err)
	}
	var c Cache
	if err := json.Unmarshal(data, &c); err != nil {
		// Corrupt cache — start fresh.
		return &Cache{}, nil
	}
	return &c, nil
}

// Save writes the cache to disk atomically (write to .tmp, then rename).
func Save(agentName string, c *Cache) error {
	path, err := CachePath(agentName)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write cache tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename cache: %w", err)
	}
	return nil
}

// Merge adds any records from fresh that are not already present in c.
// Existing records retain their Checked state.
// RecordID is used as the unique key. When it is empty a synthetic key
// (timestamp + "|" + user) is written into RecordID before storing.
func (c *Cache) Merge(records []api.FeedbackRecord) {
	existing := make(map[string]struct{}, len(c.Records))
	for _, r := range c.Records {
		existing[r.RecordID] = struct{}{}
	}
	for _, r := range records {
		if r.RecordID == "" {
			r.RecordID = r.Timestamp + "|" + r.UserName
		}
		if _, found := existing[r.RecordID]; found {
			continue
		}
		c.Records = append(c.Records, Record{
			Checked:        false,
			FeedbackRecord: r,
		})
		existing[r.RecordID] = struct{}{}
	}
}
