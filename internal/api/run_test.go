package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseSSEStream_TextDelta(t *testing.T) {
	body := "event: response.text.delta\ndata: {\"text\":\"hello\",\"content_index\":0,\"sequence_number\":1}\n\n"
	var received string
	opts := RunAgentOptions{
		OnTextDelta: func(delta string) {
			received += delta
		},
	}
	_, err := parseSSEStream(strings.NewReader(body), opts, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received != "hello" {
		t.Errorf("received = %q, want %q", received, "hello")
	}
}

func TestParseSSEStream_ThinkingDelta(t *testing.T) {
	body := "event: response.thinking.delta\ndata: {\"text\":\"thinking...\",\"content_index\":0,\"sequence_number\":1}\n\n"
	var received string
	opts := RunAgentOptions{
		OnThinkingDelta: func(delta string) {
			received += delta
		},
	}
	_, err := parseSSEStream(strings.NewReader(body), opts, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received != "thinking..." {
		t.Errorf("received = %q, want %q", received, "thinking...")
	}
}

func TestParseSSEStream_ToolUse(t *testing.T) {
	body := "event: response.tool_use\ndata: {\"name\":\"sql\",\"tool_use_id\":\"id1\",\"input\":{\"query\":\"SELECT 1\"}}\n\n"
	var toolName string
	var toolInput json.RawMessage
	opts := RunAgentOptions{
		OnToolUse: func(name string, input json.RawMessage) {
			toolName = name
			toolInput = input
		},
	}
	_, err := parseSSEStream(strings.NewReader(body), opts, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if toolName != "sql" {
		t.Errorf("toolName = %q, want %q", toolName, "sql")
	}
	if !strings.Contains(string(toolInput), "SELECT 1") {
		t.Errorf("toolInput = %s, want to contain 'SELECT 1'", toolInput)
	}
}

func TestParseSSEStream_ToolResult(t *testing.T) {
	body := "event: response.tool_result\ndata: {\"name\":\"sql\",\"tool_use_id\":\"id1\",\"status\":\"success\",\"content\":{\"data\":\"result\"}}\n\n"
	var resultName string
	opts := RunAgentOptions{
		OnToolResult: func(name string, result json.RawMessage) {
			resultName = name
		},
	}
	_, err := parseSSEStream(strings.NewReader(body), opts, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resultName != "sql" {
		t.Errorf("resultName = %q, want %q", resultName, "sql")
	}
}

func TestParseSSEStream_Error(t *testing.T) {
	body := "event: error\ndata: {\"message\":\"something failed\",\"code\":\"ERR01\"}\n\n"
	opts := RunAgentOptions{}
	_, err := parseSSEStream(strings.NewReader(body), opts, false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "something failed") {
		t.Errorf("error = %q, want to contain 'something failed'", err.Error())
	}
	if !strings.Contains(err.Error(), "ERR01") {
		t.Errorf("error = %q, want to contain 'ERR01'", err.Error())
	}
}

func TestParseSSEStream_Metadata(t *testing.T) {
	body := "event: metadata\ndata: {\"metadata\":{\"thread_id\":\"t123\",\"message_id\":456,\"role\":\"assistant\"}}\n\n"
	var tid string
	var mid int64
	opts := RunAgentOptions{
		OnMetadata: func(threadID string, messageID int64) {
			tid = threadID
			mid = messageID
		},
	}
	_, err := parseSSEStream(strings.NewReader(body), opts, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tid != "t123" {
		t.Errorf("threadID = %q, want %q", tid, "t123")
	}
	if mid != 456 {
		t.Errorf("messageID = %d, want %d", mid, 456)
	}
}

func TestParseSSEStream_Response(t *testing.T) {
	body := "event: response\ndata: {\"content\":[{\"type\":\"text\",\"text\":\"answer\"}],\"metadata\":{\"thread_id\":\"t1\",\"message_id\":99}}\n\n"
	var metaTID string
	var metaMID int64
	opts := RunAgentOptions{
		OnMetadata: func(threadID string, messageID int64) {
			metaTID = threadID
			metaMID = messageID
		},
	}
	resp, err := parseSSEStream(strings.NewReader(body), opts, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if len(resp.Content) != 1 || resp.Content[0].Text != "answer" {
		t.Errorf("response content = %+v", resp.Content)
	}
	if metaTID != "t1" {
		t.Errorf("metadata threadID = %q, want %q", metaTID, "t1")
	}
	if metaMID != 99 {
		t.Errorf("metadata messageID = %d, want %d", metaMID, 99)
	}
}

func TestParseSSEStream_Status(t *testing.T) {
	body := "event: response.status\ndata: {\"status\":\"running\",\"message\":\"Processing query\",\"sequence_number\":1}\n\n"
	var gotStatus, gotMessage string
	opts := RunAgentOptions{
		OnStatus: func(status, message string) {
			gotStatus = status
			gotMessage = message
		},
	}
	_, err := parseSSEStream(strings.NewReader(body), opts, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotStatus != "running" {
		t.Errorf("status = %q, want %q", gotStatus, "running")
	}
	if gotMessage != "Processing query" {
		t.Errorf("message = %q, want %q", gotMessage, "Processing query")
	}
}

func TestParseSSEStream_MultipleEvents(t *testing.T) {
	body := "event: response.text.delta\ndata: {\"text\":\"hello \",\"content_index\":0,\"sequence_number\":1}\n\n" +
		"event: response.text.delta\ndata: {\"text\":\"world\",\"content_index\":0,\"sequence_number\":2}\n\n"
	var received string
	opts := RunAgentOptions{
		OnTextDelta: func(delta string) {
			received += delta
		},
	}
	_, err := parseSSEStream(strings.NewReader(body), opts, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received != "hello world" {
		t.Errorf("received = %q, want %q", received, "hello world")
	}
}

func TestParseSSEStream_Comments(t *testing.T) {
	body := ": this is a comment\nevent: response.text.delta\ndata: {\"text\":\"ok\",\"content_index\":0,\"sequence_number\":1}\n\n"
	var received string
	opts := RunAgentOptions{
		OnTextDelta: func(delta string) {
			received += delta
		},
	}
	_, err := parseSSEStream(strings.NewReader(body), opts, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received != "ok" {
		t.Errorf("received = %q, want %q", received, "ok")
	}
}

func TestParseSSEStream_NilCallbacks(t *testing.T) {
	body := "event: response.text.delta\ndata: {\"text\":\"hello\",\"content_index\":0,\"sequence_number\":1}\n\n" +
		"event: response.thinking.delta\ndata: {\"text\":\"think\",\"content_index\":0,\"sequence_number\":1}\n\n" +
		"event: response.tool_use\ndata: {\"name\":\"sql\",\"tool_use_id\":\"id1\",\"input\":{}}\n\n" +
		"event: response.tool_result\ndata: {\"name\":\"sql\",\"tool_use_id\":\"id1\",\"status\":\"ok\",\"content\":{}}\n\n" +
		"event: response.status\ndata: {\"status\":\"done\",\"message\":\"ok\",\"sequence_number\":1}\n\n" +
		"event: metadata\ndata: {\"metadata\":{\"thread_id\":\"t1\",\"message_id\":1,\"role\":\"assistant\"}}\n\n"
	opts := RunAgentOptions{} // all callbacks nil
	_, err := parseSSEStream(strings.NewReader(body), opts, false)
	if err != nil {
		t.Fatalf("should not panic or error with nil callbacks: %v", err)
	}
}

func TestParseSSEStream_EmptyBody(t *testing.T) {
	opts := RunAgentOptions{}
	resp, err := parseSSEStream(strings.NewReader(""), opts, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response for empty body, got %+v", resp)
	}
}

func TestNewTextMessage(t *testing.T) {
	msg := NewTextMessage("user", "Hello world")
	if msg.Role != "user" {
		t.Errorf("Role = %q, want %q", msg.Role, "user")
	}
	if len(msg.Content) != 1 {
		t.Fatalf("Content length = %d, want 1", len(msg.Content))
	}
	if msg.Content[0].Type != "text" {
		t.Errorf("Content[0].Type = %q, want %q", msg.Content[0].Type, "text")
	}
	if msg.Content[0].Text != "Hello world" {
		t.Errorf("Content[0].Text = %q, want %q", msg.Content[0].Text, "Hello world")
	}
}

func TestNewTextMessage_AssistantRole(t *testing.T) {
	msg := NewTextMessage("assistant", "response")
	if msg.Role != "assistant" {
		t.Errorf("Role = %q, want %q", msg.Role, "assistant")
	}
}
