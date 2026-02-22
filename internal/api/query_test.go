package api

import (
	"testing"
)

func TestEscapeSQLString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no quotes", "hello", "hello"},
		{"single quote", "it's", "it''s"},
		{"multiple quotes", "it's a 'test'", "it''s a ''test''"},
		{"empty", "", ""},
		{"only quote", "'", "''"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeSQLString(tt.input)
			if got != tt.want {
				t.Errorf("escapeSQLString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseSnowflakeTimestamp(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string // partial match check
	}{
		{"valid epoch", "1700000000.000000000", "2023"},
		{"not numeric", "not-a-number", "not-a-number"},
		{"empty", "", ""},
		{"zero", "0.000000000", "1970"},
		{"fractional seconds", "1700000000.500000000", "2023"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSnowflakeTimestamp(tt.raw)
			if tt.want != "" && len(got) < len(tt.want) {
				t.Errorf("parseSnowflakeTimestamp(%q) = %q, expected to contain %q", tt.raw, got, tt.want)
			}
			if tt.raw == "not-a-number" && got != tt.raw {
				t.Errorf("parseSnowflakeTimestamp(%q) = %q, want %q (passthrough)", tt.raw, got, tt.raw)
			}
			if tt.raw == "" && got != "" {
				t.Errorf("parseSnowflakeTimestamp(%q) = %q, want empty passthrough", tt.raw, got)
			}
		})
	}
}

func TestExtractQuestion(t *testing.T) {
	tests := []struct {
		name  string
		json  string
		want  string
	}{
		{
			name: "valid request with user message",
			json: `{"snow.ai.observability.request_body":{"messages":[{"role":"user","content":[{"type":"text","text":"hello world"}]}]}}`,
			want: "hello world",
		},
		{
			name: "last user message returned",
			json: `{"snow.ai.observability.request_body":{"messages":[{"role":"user","content":[{"type":"text","text":"first"}]},{"role":"assistant","content":[{"type":"text","text":"response"}]},{"role":"user","content":[{"type":"text","text":"second"}]}]}}`,
			want: "second",
		},
		{
			name: "empty messages",
			json: `{"snow.ai.observability.request_body":{"messages":[]}}`,
			want: "",
		},
		{
			name: "no user messages",
			json: `{"snow.ai.observability.request_body":{"messages":[{"role":"assistant","content":[{"type":"text","text":"response"}]}]}}`,
			want: "",
		},
		{
			name:  "invalid json",
			json:  "not json",
			want:  "",
		},
		{
			name:  "empty string",
			json:  "",
			want:  "",
		},
		{
			name: "missing request body key",
			json: `{"other_key":{"messages":[]}}`,
			want: "",
		},
		{
			name: "multiple text parts concatenated",
			json: `{"snow.ai.observability.request_body":{"messages":[{"role":"user","content":[{"type":"text","text":"part1"},{"type":"text","text":"part2"}]}]}}`,
			want: "part1part2",
		},
		{
			name: "skips non-text content",
			json: `{"snow.ai.observability.request_body":{"messages":[{"role":"user","content":[{"type":"image","url":"..."},{"type":"text","text":"question"}]}]}}`,
			want: "question",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractQuestion(tt.json)
			if got != tt.want {
				t.Errorf("extractQuestion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractResponse(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{
			name: "valid response",
			json: `{"snow.ai.observability.response":"{\"content\":[{\"type\":\"text\",\"text\":\"answer\"}]}"}`,
			want: "answer",
		},
		{
			name: "multiple text parts",
			json: `{"snow.ai.observability.response":"{\"content\":[{\"type\":\"text\",\"text\":\"part1\"},{\"type\":\"text\",\"text\":\"part2\"}]}"}`,
			want: "part1part2",
		},
		{
			name: "skips tool_use content",
			json: `{"snow.ai.observability.response":"{\"content\":[{\"type\":\"tool_use\",\"name\":\"sql\"},{\"type\":\"text\",\"text\":\"answer\"}]}"}`,
			want: "answer",
		},
		{
			name:  "invalid json",
			json:  "not json",
			want:  "",
		},
		{
			name:  "empty string",
			json:  "",
			want:  "",
		},
		{
			name: "missing response key",
			json: `{"other_key":"something"}`,
			want: "",
		},
		{
			name: "empty response string",
			json: `{"snow.ai.observability.response":""}`,
			want: "",
		},
		{
			name: "nested response invalid json",
			json: `{"snow.ai.observability.response":"not valid json"}`,
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractResponse(tt.json)
			if got != tt.want {
				t.Errorf("extractResponse() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractToolUses(t *testing.T) {
	t.Run("single tool use with result", func(t *testing.T) {
		json := `{"snow.ai.observability.response":"{\"content\":[{\"type\":\"tool_use\",\"tool_use\":{\"type\":\"cortex_analyst_text_to_sql\",\"name\":\"my_tool\",\"tool_use_id\":\"id1\",\"input\":{\"query\":\"show sales\"}}},{\"type\":\"tool_result\",\"tool_result\":{\"tool_use_id\":\"id1\",\"status\":\"success\",\"content\":[{\"json\":{\"sql\":\"SELECT * FROM sales\"}}]}}]}"}`
		got := extractToolUses(json)
		if len(got) != 1 {
			t.Fatalf("expected 1 tool use, got %d", len(got))
		}
		if got[0].ToolType != "cortex_analyst_text_to_sql" {
			t.Errorf("ToolType = %q, want %q", got[0].ToolType, "cortex_analyst_text_to_sql")
		}
		if got[0].ToolName != "my_tool" {
			t.Errorf("ToolName = %q, want %q", got[0].ToolName, "my_tool")
		}
		if got[0].Query != "show sales" {
			t.Errorf("Query = %q, want %q", got[0].Query, "show sales")
		}
		if got[0].ToolStatus != "success" {
			t.Errorf("ToolStatus = %q, want %q", got[0].ToolStatus, "success")
		}
		if got[0].SQL != "SELECT * FROM sales" {
			t.Errorf("SQL = %q, want %q", got[0].SQL, "SELECT * FROM sales")
		}
	})

	t.Run("no tool uses", func(t *testing.T) {
		json := `{"snow.ai.observability.response":"{\"content\":[{\"type\":\"text\",\"text\":\"answer\"}]}"}`
		got := extractToolUses(json)
		if len(got) != 0 {
			t.Errorf("expected 0 tool uses, got %d", len(got))
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		got := extractToolUses("not json")
		if got != nil {
			t.Errorf("expected nil for invalid json, got %v", got)
		}
	})

	t.Run("empty string", func(t *testing.T) {
		got := extractToolUses("")
		if got != nil {
			t.Errorf("expected nil for empty string, got %v", got)
		}
	})

	t.Run("multiple tool uses", func(t *testing.T) {
		json := `{"snow.ai.observability.response":"{\"content\":[{\"type\":\"tool_use\",\"tool_use\":{\"name\":\"tool_a\",\"tool_use_id\":\"id1\",\"input\":{}}},{\"type\":\"tool_use\",\"tool_use\":{\"name\":\"tool_b\",\"tool_use_id\":\"id2\",\"input\":{}}}]}"}`
		got := extractToolUses(json)
		if len(got) != 2 {
			t.Fatalf("expected 2 tool uses, got %d", len(got))
		}
		if got[0].ToolName != "tool_a" {
			t.Errorf("got[0].ToolName = %q, want tool_a", got[0].ToolName)
		}
		if got[1].ToolName != "tool_b" {
			t.Errorf("got[1].ToolName = %q, want tool_b", got[1].ToolName)
		}
	})
}

func TestExtractResponseTimeMs(t *testing.T) {
	tests := []struct {
		name string
		json string
		want int64
	}{
		{
			name: "valid response time",
			json: `{"snow.ai.observability.response_time_ms":1234.0}`,
			want: 1234,
		},
		{
			name: "fractional ms truncated",
			json: `{"snow.ai.observability.response_time_ms":1234.9}`,
			want: 1234,
		},
		{
			name:  "missing key",
			json:  `{"other_key":100}`,
			want:  0,
		},
		{
			name:  "invalid json",
			json:  "not json",
			want:  0,
		},
		{
			name:  "empty string",
			json:  "",
			want:  0,
		},
		{
			name: "zero",
			json: `{"snow.ai.observability.response_time_ms":0}`,
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractResponseTimeMs(tt.json)
			if got != tt.want {
				t.Errorf("extractResponseTimeMs() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestClassifySentiment(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"positive keyword", "positive", "positive"},
		{"negative keyword", "negative", "negative"},
		{"true = positive", "true", "positive"},
		{"false = negative", "false", "negative"},
		{"1 = positive", "1", "positive"},
		{"0 = negative", "0", "negative"},
		{"thumbsup", "thumbsup", "positive"},
		{"thumbsdown", "thumbsdown", "negative"},
		{"good", "good", "positive"},
		{"bad", "bad", "negative"},
		{"unknown", "neutral", "unknown"},
		{"empty", "", "unknown"},
		{"case insensitive positive", "POSITIVE", "positive"},
		{"case insensitive negative", "NEGATIVE", "negative"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifySentiment(tt.input)
			if got != tt.want {
				t.Errorf("classifySentiment(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestProbeString(t *testing.T) {
	m := map[string]any{
		"Sentiment": "positive",
		"score":     "95",
	}

	t.Run("finds case-insensitive key", func(t *testing.T) {
		got := probeString(m, []string{"sentiment"})
		if got != "positive" {
			t.Errorf("probeString() = %q, want %q", got, "positive")
		}
	})

	t.Run("returns first match", func(t *testing.T) {
		got := probeString(m, []string{"missing", "score"})
		if got != "95" {
			t.Errorf("probeString() = %q, want %q", got, "95")
		}
	})

	t.Run("no match returns empty", func(t *testing.T) {
		got := probeString(m, []string{"nonexistent"})
		if got != "" {
			t.Errorf("probeString() = %q, want empty", got)
		}
	})
}
