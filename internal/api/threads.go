package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"time"
)

// Thread represents a conversation thread from the Threads API.
type Thread struct {
	ThreadID          string    `json:"thread_id"`
	ThreadName        string    `json:"thread_name"`
	OriginApplication string    `json:"origin_application"`
	CreatedOn         int64     `json:"created_on"`  // milliseconds since UNIX epoch
	UpdatedOn         int64     `json:"updated_on"`  // milliseconds since UNIX epoch
	CreatedAt         time.Time `json:"-"`           // parsed from CreatedOn
	UpdatedAt         time.Time `json:"-"`           // parsed from UpdatedOn
}

// ThreadMessage represents a message within a thread.
type ThreadMessage struct {
	MessageID      int64  `json:"message_id"`
	ParentID       int64  `json:"parent_id"`
	CreatedOn      int64  `json:"created_on"`
	Role           string `json:"role"`
	MessagePayload string `json:"message_payload"`
	RequestID      string `json:"request_id"`
}

// CreateThreadRequest represents the request to create a new thread.
type CreateThreadRequest struct {
	OriginApplication string `json:"origin_application,omitempty"`
}

// CreateThread creates a new conversation thread.
// Returns the thread_id as a string.
func (c *Client) CreateThread(ctx context.Context) (string, error) {
	req := CreateThreadRequest{
		OriginApplication: "coragent",
	}

	var thread Thread
	if err := c.doJSON(ctx, http.MethodPost, c.threadsURL(), req, &thread); err != nil {
		return "", fmt.Errorf("create thread: %w", err)
	}

	return thread.ThreadID, nil
}

// ListThreads retrieves all threads for the current user.
func (c *Client) ListThreads(ctx context.Context) ([]Thread, error) {
	var threads []Thread
	if err := c.doJSON(ctx, http.MethodGet, c.threadsURL(), nil, &threads); err != nil {
		return nil, fmt.Errorf("list threads: %w", err)
	}

	// Parse timestamps
	for i := range threads {
		if threads[i].CreatedOn > 0 {
			threads[i].CreatedAt = time.UnixMilli(threads[i].CreatedOn)
		}
		if threads[i].UpdatedOn > 0 {
			threads[i].UpdatedAt = time.UnixMilli(threads[i].UpdatedOn)
		}
	}

	return threads, nil
}

// GetThread retrieves a specific thread by ID.
func (c *Client) GetThread(ctx context.Context, threadID string) (*Thread, error) {
	var thread Thread
	if err := c.doJSON(ctx, http.MethodGet, c.threadURL(threadID), nil, &thread); err != nil {
		return nil, fmt.Errorf("get thread: %w", err)
	}

	if thread.CreatedOn > 0 {
		thread.CreatedAt = time.UnixMilli(thread.CreatedOn)
	}
	if thread.UpdatedOn > 0 {
		thread.UpdatedAt = time.UnixMilli(thread.UpdatedOn)
	}

	return &thread, nil
}

// DeleteThread deletes a thread by ID.
func (c *Client) DeleteThread(ctx context.Context, threadID string) error {
	if err := c.doJSON(ctx, http.MethodDelete, c.threadURL(threadID), nil, nil); err != nil {
		return fmt.Errorf("delete thread: %w", err)
	}
	return nil
}

// threadsURL returns the URL for the threads collection endpoint.
func (c *Client) threadsURL() string {
	u := *c.baseURL
	u.Path = path.Join(u.Path, "api/v2/cortex/threads")
	return u.String()
}

// threadURL returns the URL for a specific thread.
func (c *Client) threadURL(threadID string) string {
	u := *c.baseURL
	u.Path = path.Join(u.Path, "api/v2/cortex/threads", threadID)
	return u.String()
}

// ThreadIDToInt64 converts a thread ID string to int64.
func ThreadIDToInt64(threadID string) (int64, error) {
	return strconv.ParseInt(threadID, 10, 64)
}

// Int64ToThreadID converts an int64 to thread ID string.
func Int64ToThreadID(id int64) string {
	return strconv.FormatInt(id, 10)
}

// ThreadListResponse is used for parsing the list threads response.
type ThreadListResponse struct {
	Threads []Thread `json:"threads"`
}

// UnmarshalJSON implements custom JSON unmarshaling for Thread to handle
// both string and integer thread_id formats.
func (t *Thread) UnmarshalJSON(data []byte) error {
	type Alias Thread
	aux := &struct {
		ThreadID json.RawMessage `json:"thread_id"`
		*Alias
	}{
		Alias: (*Alias)(t),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Handle thread_id as either string or integer
	if aux.ThreadID != nil {
		var strID string
		if err := json.Unmarshal(aux.ThreadID, &strID); err != nil {
			// Try as integer
			var intID int64
			if err := json.Unmarshal(aux.ThreadID, &intID); err != nil {
				return fmt.Errorf("thread_id must be string or integer: %w", err)
			}
			t.ThreadID = strconv.FormatInt(intID, 10)
		} else {
			t.ThreadID = strID
		}
	}

	return nil
}
