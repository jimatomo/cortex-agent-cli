package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"coragent/internal/api"
)

// CachedFeedbackRecord wraps a FeedbackRecord with a checked state.
// The record is identified by the embedded FeedbackRecord.RecordID.
type CachedFeedbackRecord struct {
	Checked bool `json:"checked"`
	api.FeedbackRecord
}

// FeedbackCache is the on-disk structure stored at ~/.coragent/feedback/<agent>.json.
type FeedbackCache struct {
	Records []CachedFeedbackRecord `json:"records"`
}

// feedbackCachePath returns the path to the cache file for agentName and ensures
// the parent directory exists.
func feedbackCachePath(agentName string) (string, error) {
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

// loadFeedbackCache reads the cache file for agentName. Returns an empty cache
// if the file does not exist yet.
func loadFeedbackCache(agentName string) (*FeedbackCache, error) {
	path, err := feedbackCachePath(agentName)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &FeedbackCache{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read cache: %w", err)
	}
	var c FeedbackCache
	if err := json.Unmarshal(data, &c); err != nil {
		// Corrupt cache — start fresh.
		return &FeedbackCache{}, nil
	}
	return &c, nil
}

// saveFeedbackCache writes the cache to disk.
func saveFeedbackCache(agentName string, c *FeedbackCache) error {
	path, err := feedbackCachePath(agentName)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}
	// Write to a temp file then rename for atomicity.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write cache tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename cache: %w", err)
	}
	return nil
}

// merge adds any records from fresh that are not already present in c.
// Existing records retain their Checked state.
// RecordID is used as the unique key. When it is empty a synthetic key
// (timestamp + "|" + user) is written into RecordID before storing.
func (c *FeedbackCache) merge(records []api.FeedbackRecord) {
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
		c.Records = append(c.Records, CachedFeedbackRecord{
			Checked:        false,
			FeedbackRecord: r,
		})
		existing[r.RecordID] = struct{}{}
	}
}
