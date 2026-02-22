package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"coragent/internal/auth"
)

// RunAgentRequest represents the request payload for running an agent.
type RunAgentRequest struct {
	Messages        []Message `json:"messages"`
	ThreadID        string    `json:"thread_id,omitempty"`
	ParentMessageID *int64    `json:"parent_message_id,omitempty"`
}

// Message represents a chat message.
type Message struct {
	Role    string         `json:"role"` // "user" or "assistant"
	Content []ContentBlock `json:"content"`
}

// ContentBlock represents a content block within a message.
type ContentBlock struct {
	Type string `json:"type"` // "text"
	Text string `json:"text"`
}

// NewTextMessage creates a new user message with text content.
func NewTextMessage(role, text string) Message {
	return Message{
		Role: role,
		Content: []ContentBlock{
			{Type: "text", Text: text},
		},
	}
}

// TextDeltaEvent represents a streaming text delta.
type TextDeltaEvent struct {
	Text           string `json:"text"`
	ContentIndex   int    `json:"content_index"`
	SequenceNumber int    `json:"sequence_number"`
}

// ThinkingDeltaEvent represents a streaming thinking/reasoning delta.
type ThinkingDeltaEvent struct {
	Text           string `json:"text"`
	ContentIndex   int    `json:"content_index"`
	SequenceNumber int    `json:"sequence_number"`
}

// ToolUseEvent represents a tool invocation.
type ToolUseEvent struct {
	Name           string          `json:"name"`
	ToolUseID      string          `json:"tool_use_id"`
	Input          json.RawMessage `json:"input"`
	Type           string          `json:"type"`
	ContentIndex   int             `json:"content_index"`
	SequenceNumber int             `json:"sequence_number"`
}

// ToolResultEvent represents a tool result.
type ToolResultEvent struct {
	Name           string          `json:"name"`
	ToolUseID      string          `json:"tool_use_id"`
	Status         string          `json:"status"`
	Content        json.RawMessage `json:"content"`
	Type           string          `json:"type"`
	ContentIndex   int             `json:"content_index"`
	SequenceNumber int             `json:"sequence_number"`
}

// ResponseEvent represents the final complete response.
type ResponseEvent struct {
	Content  []ResponseContentBlock `json:"content"`
	Metadata *ResponseMetadata      `json:"metadata,omitempty"`
}

// ResponseMetadata contains metadata about the response including thread info.
type ResponseMetadata struct {
	ThreadID  string `json:"thread_id,omitempty"`
	MessageID int64  `json:"message_id,omitempty"`
}

// ResponseContentBlock represents a content block in the final response.
type ResponseContentBlock struct {
	Type       string          `json:"type"` // "text", "thinking", "tool_use", "tool_result"
	Text       string          `json:"text,omitempty"`
	Thinking   json.RawMessage `json:"thinking,omitempty"`
	ToolUse    json.RawMessage `json:"tool_use,omitempty"`
	ToolResult json.RawMessage `json:"tool_result,omitempty"`
}

// ErrorEvent represents an error from the agent.
type ErrorEvent struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

// StatusEvent represents a status update from the agent.
type StatusEvent struct {
	Status         string `json:"status"`
	Message        string `json:"message"`
	SequenceNumber int    `json:"sequence_number"`
}

// MetadataEvent represents thread/message metadata from the response.
// The actual metadata is nested inside a "metadata" field.
type MetadataEvent struct {
	Metadata struct {
		MessageID int64  `json:"message_id"`
		ThreadID  string `json:"thread_id"`
		Role      string `json:"role"`
	} `json:"metadata"`
}

// RunAgentOptions configures callbacks for streaming events.
type RunAgentOptions struct {
	OnStatus        func(status, message string)
	OnTextDelta     func(delta string)
	OnThinkingDelta func(delta string)
	OnToolUse       func(name string, input json.RawMessage)
	OnToolResult    func(name string, result json.RawMessage)
	OnMetadata      func(threadID string, messageID int64)
	OnProgress      func(phase string) // Called during pre-SSE phases (auth, sending, etc.)
}

// RunAgent executes an agent with SSE streaming.
func (c *Client) RunAgent(ctx context.Context, db, schema, name string, req RunAgentRequest, opts RunAgentOptions) (*ResponseEvent, error) {
	urlStr := c.agentRunURL(db, schema, name)

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set authorization header
	if opts.OnProgress != nil {
		opts.OnProgress("Authenticating...")
	}
	token, tokenType, err := auth.BearerToken(ctx, c.authCfg)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("X-Snowflake-Authorization-Token-Type", tokenType)
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", c.userAgent)
	if c.role != "" {
		httpReq.Header.Set("X-Snowflake-Role", c.role)
	}

	// Use a client with longer timeout for streaming
	httpClient := &http.Client{Timeout: 15 * time.Minute}

	c.debugf("HTTP POST %s", urlStr)
	c.debugf("request body: %s", truncateDebug(data))

	if opts.OnProgress != nil {
		opts.OnProgress("Sending request...")
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	c.debugf("response status: %d", resp.StatusCode)

	if resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, APIError{StatusCode: resp.StatusCode, Body: string(bodyBytes)}
	}

	if opts.OnProgress != nil {
		opts.OnProgress("Waiting for response...")
	}
	return parseSSEStream(resp.Body, opts, c.debug)
}

func (c *Client) agentRunURL(db, schema, name string) string {
	u := *c.baseURL
	u.Path = path.Join(
		u.Path,
		"api/v2/databases",
		identifierSegment(db),
		"schemas",
		identifierSegment(schema),
		"agents",
		identifierSegment(name)+":run",
	)
	return u.String()
}

// parseSSEStream parses Server-Sent Events from the response body.
func parseSSEStream(body io.Reader, opts RunAgentOptions, debug bool) (*ResponseEvent, error) {
	reader := bufio.NewReader(body)
	var currentEvent string
	var dataBuffer strings.Builder
	var finalResponse *ResponseEvent

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return finalResponse, fmt.Errorf("read SSE: %w", err)
		}

		line = strings.TrimRight(line, "\r\n")

		// Empty line signals end of event
		if line == "" {
			if currentEvent != "" && dataBuffer.Len() > 0 {
				if err := processSSEEvent(currentEvent, dataBuffer.String(), opts, &finalResponse, debug); err != nil {
					return finalResponse, err
				}
			}
			currentEvent = ""
			dataBuffer.Reset()
			continue
		}

		// Parse event type
		if after, found := strings.CutPrefix(line, "event:"); found {
			currentEvent = strings.TrimSpace(after)
			continue
		}

		// Parse data
		if after, found := strings.CutPrefix(line, "data:"); found {
			data := strings.TrimPrefix(after, " ") // Remove optional leading space
			dataBuffer.WriteString(data)
			continue
		}

		// Handle comment lines (starting with :)
		if strings.HasPrefix(line, ":") { //nolint:staticcheck // False positive: return value is used as if-condition.
			continue
		}
	}

	// Process any remaining buffered event
	if currentEvent != "" && dataBuffer.Len() > 0 {
		if err := processSSEEvent(currentEvent, dataBuffer.String(), opts, &finalResponse, debug); err != nil {
			return finalResponse, err
		}
	}

	return finalResponse, nil
}

func processSSEEvent(eventType, data string, opts RunAgentOptions, finalResponse **ResponseEvent, debug bool) error {
	if debug {
		fmt.Printf("DEBUG: SSE event=%s data=%s\n", eventType, truncateDebug([]byte(data)))
	}

	switch eventType {
	case "response.status":
		var evt StatusEvent
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			return fmt.Errorf("parse status: %w", err)
		}
		if opts.OnStatus != nil {
			opts.OnStatus(evt.Status, evt.Message)
		}

	case "response.text.delta":
		var evt TextDeltaEvent
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			return fmt.Errorf("parse text delta: %w", err)
		}
		if opts.OnTextDelta != nil {
			opts.OnTextDelta(evt.Text)
		}

	case "response.thinking.delta":
		var evt ThinkingDeltaEvent
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			return fmt.Errorf("parse thinking delta: %w", err)
		}
		if opts.OnThinkingDelta != nil {
			opts.OnThinkingDelta(evt.Text)
		}

	case "response.tool_use":
		var evt ToolUseEvent
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			return fmt.Errorf("parse tool use: %w", err)
		}
		if opts.OnToolUse != nil {
			opts.OnToolUse(evt.Name, evt.Input)
		}

	case "response.tool_result":
		var evt ToolResultEvent
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			return fmt.Errorf("parse tool result: %w", err)
		}
		if opts.OnToolResult != nil {
			opts.OnToolResult(evt.Name, evt.Content)
		}

	case "response":
		var evt ResponseEvent
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			return fmt.Errorf("parse response: %w", err)
		}
		*finalResponse = &evt
		// Also extract thread metadata from response if available
		if evt.Metadata != nil && opts.OnMetadata != nil {
			opts.OnMetadata(evt.Metadata.ThreadID, evt.Metadata.MessageID)
		}

	case "error":
		var evt ErrorEvent
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			return fmt.Errorf("parse error event: %w", err)
		}
		return fmt.Errorf("agent error [%s]: %s", evt.Code, evt.Message)

	case "metadata":
		var evt MetadataEvent
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			return fmt.Errorf("parse metadata: %w", err)
		}
		if opts.OnMetadata != nil {
			opts.OnMetadata(evt.Metadata.ThreadID, evt.Metadata.MessageID)
		}
	}

	return nil
}
