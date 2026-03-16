package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestEscapeSQLJSONString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain", `{"a":"b"}`, `{"a":"b"}`},
		{"single quote", `{"text":"it's ok"}`, `{"text":"it''s ok"}`},
		{"escaped newline", `{"sql":"SELECT 1\nFROM dual"}`, `{"sql":"SELECT 1\\nFROM dual"}`},
		{"escaped quote", `{"text":"\"hello\""}`, `{"text":"\\"hello\\""}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeSQLJSONString(tt.input)
			if got != tt.want {
				t.Errorf("escapeSQLJSONString(%q) = %q, want %q", tt.input, got, tt.want)
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
		{"timestamp_tz epoch with tz token", "1771563530.421000000 1980", "2026"},
		{"not numeric", "not-a-number", "not-a-number"},
		{"empty", "", ""},
		{"zero", "0.000000000", "1970"},
		{"fractional seconds", "1700000000.500000000", "2023"},
		{"rfc3339", "2026-02-20T13:58:50.421Z", "2026-02-20 13:58:50.421 UTC"},
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

func TestSinceForSQL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"utc suffix", "2026-03-08 00:00:00.000 UTC", "2026-03-08 00:00:00.000 +0000"},
		{"already offset", "2026-03-08 00:00:00.000 +0900", "2026-03-08 00:00:00.000 +0900"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sinceForSQL(tt.input); got != tt.want {
				t.Fatalf("sinceForSQL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeFeedbackEventTimestamp(t *testing.T) {
	got := normalizeFeedbackEventTimestamp("2026-03-08 00:00:00.000 UTC")
	want := "2026-03-08 00:00:00.000 +0000"
	if got != want {
		t.Fatalf("normalizeFeedbackEventTimestamp() = %q, want %q", got, want)
	}
}

func TestExtractQuestion(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
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
			name: "invalid json",
			json: "not json",
			want: "",
		},
		{
			name: "empty string",
			json: "",
			want: "",
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
			name: "invalid json",
			json: "not json",
			want: "",
		},
		{
			name: "empty string",
			json: "",
			want: "",
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
			name: "missing key",
			json: `{"other_key":100}`,
			want: 0,
		},
		{
			name: "invalid json",
			json: "not json",
			want: 0,
		},
		{
			name: "empty string",
			json: "",
			want: 0,
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

func TestParseNegativeInferenceResponse(t *testing.T) {
	t.Run("structured output wrapper", func(t *testing.T) {
		raw := `{"structured_output":[{"raw_message":{"negative":true,"reasoning":"The answer did not address the user's request"},"type":"json"}]}`
		got, err := parseNegativeInferenceResponse(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got.Negative {
			t.Fatal("Negative = false, want true")
		}
		if got.Reasoning == "" {
			t.Fatal("Reasoning should not be empty")
		}
	})

	t.Run("direct schema object", func(t *testing.T) {
		raw := `{"negative":true,"reasoning":"The answer did not address the user's request"}`
		got, err := parseNegativeInferenceResponse(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got.Negative {
			t.Fatal("Negative = false, want true")
		}
		if got.Reasoning == "" {
			t.Fatal("Reasoning should not be empty")
		}
	})
}

func TestSQLCellString(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    string
		wantErr string
	}{
		{
			name:  "string",
			input: `{"structured_output":[{"raw_message":{"negative":true}}]}`,
			want:  `{"structured_output":[{"raw_message":{"negative":true}}]}`,
		},
		{
			name:  "variant object marshaled",
			input: map[string]any{"structured_output": []any{map[string]any{"raw_message": map[string]any{"negative": true, "reasoning": "x"}}}},
			want:  `{"structured_output":[{"raw_message":{"negative":true,"reasoning":"x"}}]}`,
		},
		{
			name:    "null",
			input:   nil,
			wantErr: "null response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sqlCellString(tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("sqlCellString() error = nil, want %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("sqlCellString() error = %q, want substring %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("sqlCellString() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("sqlCellString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInferNegativeFeedbackHeuristically(t *testing.T) {
	tests := []struct {
		name   string
		record FeedbackRecord
		wantOK bool
		wantNg bool
	}{
		{
			name: "empty response is negative",
			record: FeedbackRecord{
				Question: "show me the sales trend",
				Response: "",
			},
			wantOK: true,
			wantNg: true,
		},
		{
			name: "missing requested year data is negative",
			record: FeedbackRecord{
				Question: "2026年の支出の傾向を教えてください",
				Response: "申し訳ございませんが、データベースには2026年の支出データが存在しません。利用可能なデータは2024年のみです。代わりに2024年の分析は可能です。",
			},
			wantOK: true,
			wantNg: true,
		},
		{
			name: "substantive answer is not short-circuited",
			record: FeedbackRecord{
				Question: "show me the sales trend",
				Response: "Sales increased 12% quarter over quarter, led by enterprise accounts.",
			},
			wantOK: false,
			wantNg: false,
		},
		{
			name: "generic status phrase is not short-circuited",
			record: FeedbackRecord{
				Question: "show all orders with status",
				Response: "I found 3 rows where the status is Not Available and 7 rows marked Complete.",
			},
			wantOK: false,
			wantNg: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := inferNegativeFeedbackHeuristically(tt.record)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got.Negative != tt.wantNg {
				t.Fatalf("Negative = %v, want %v", got.Negative, tt.wantNg)
			}
			if ok && strings.TrimSpace(got.Reasoning) == "" {
				t.Fatal("Reasoning should not be empty when heuristic applies")
			}
		})
	}
}

func TestGetFeedbackInferNegativePreservesExplicitSince(t *testing.T) {
	t.Parallel()

	var statements []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/statements" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req sqlStatementRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		statements = append(statements, req.Statement)
		w.Header().Set("Content-Type", "application/json")
		if len(statements) == 1 {
			_ = json.NewEncoder(w).Encode(sqlStatementResponse{
				ResultSetMetaData: struct {
					RowType []sqlRowType `json:"rowType"`
				}{RowType: []sqlRowType{{Name: "timestamp"}, {Name: "record_id"}}},
				Data: [][]any{},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(sqlStatementResponse{
			ResultSetMetaData: struct {
				RowType []sqlRowType `json:"rowType"`
			}{RowType: []sqlRowType{{Name: "timestamp"}, {Name: "record_id"}}},
			Data: [][]any{},
		})
	}))
	defer srv.Close()

	client := newDescribeTestClient(t, srv)

	_, err := client.GetFeedback(context.Background(), "DB", "SC", "agent", FeedbackQueryOptions{
		Since:         "2026-03-08 12:34:56.000 UTC",
		ExplicitSince: "",
		RequestSince:  "2026-03-08 12:34:56.000 UTC",
		InferNegative: true,
	})
	if err != nil {
		t.Fatalf("GetFeedback() error = %v", err)
	}
	if len(statements) != 2 {
		t.Fatalf("statement count = %d, want 2", len(statements))
	}
	if strings.Contains(statements[0], "f.TIMESTAMP >=") {
		t.Fatalf("explicit query unexpectedly filtered by timestamp:\n%s", statements[0])
	}
	if !strings.Contains(statements[1], "r.TIMESTAMP >= TO_TIMESTAMP_TZ('2026-03-08 12:34:56.000 +0000'") {
		t.Fatalf("request-only query missing request cursor:\n%s", statements[1])
	}
}

func TestSyncFeedbackFromEventsToTableInferNegativePreservesExplicitSince(t *testing.T) {
	t.Parallel()

	var statements []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/statements" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req sqlStatementRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		statements = append(statements, req.Statement)
		w.Header().Set("Content-Type", "application/json")

		switch len(statements) {
		case 1:
			_ = json.NewEncoder(w).Encode(sqlStatementResponse{
				ResultSetMetaData: struct {
					RowType []sqlRowType `json:"rowType"`
				}{RowType: []sqlRowType{{Name: "column_name"}}},
				Data: [][]any{
					{"record_id"},
					{"sentiment_source"},
					{"sentiment_reason"},
				},
			})
		case 2:
			_ = json.NewEncoder(w).Encode(sqlStatementResponse{
				ResultSetMetaData: struct {
					RowType []sqlRowType `json:"rowType"`
				}{RowType: []sqlRowType{
					{Name: "timestamp"},
					{Name: "resource_attributes"},
					{Name: "feedback_attrs"},
					{Name: "feedback_value"},
					{Name: "request_value"},
					{Name: "record_id"},
				}},
				Data: [][]any{{
					"2026-03-08T12:34:56.000Z",
					`{"snow.user.name":"user1"}`,
					`{"snow.ai.observability.object.name":"agent","snow.ai.observability.user.name":"user1"}`,
					`{"positive":false,"feedback_message":"not helpful","categories":["quality"]}`,
					`{"snow.ai.observability.request_body":{"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]},"snow.ai.observability.response":"{\"content\":[{\"type\":\"text\",\"text\":\"answer\"}]}","snow.ai.observability.response_time_ms":"123"}`,
					"rid-1",
				}},
			})
		default:
			_ = json.NewEncoder(w).Encode(sqlStatementResponse{
				ResultSetMetaData: struct {
					RowType []sqlRowType `json:"rowType"`
				}{RowType: []sqlRowType{{Name: "timestamp"}, {Name: "record_id"}}},
				Data: [][]any{},
			})
		}
	}))
	defer srv.Close()

	client := newDescribeTestClient(t, srv)

	err := client.SyncFeedbackFromEventsToTable(context.Background(), "SRC_DB", "SRC_SC", "agent", "DST_DB", "DST_SC", "DST_TABLE", FeedbackQueryOptions{
		Since:         "2026-03-08 12:34:56.000 UTC",
		ExplicitSince: "",
		RequestSince:  "2026-03-08 12:34:56.000 UTC",
		InferNegative: true,
	})
	if err != nil {
		t.Fatalf("SyncFeedbackFromEventsToTable() error = %v", err)
	}
	if len(statements) < 6 {
		t.Fatalf("statement count = %d, want at least 6", len(statements))
	}
	if !strings.HasPrefix(statements[0], "SHOW COLUMNS IN TABLE") {
		t.Fatalf("first statement = %q, want SHOW COLUMNS", statements[0])
	}
	if strings.Contains(statements[1], "f.TIMESTAMP >=") {
		t.Fatalf("explicit query unexpectedly filtered by timestamp:\n%s", statements[1])
	}
	if !strings.Contains(statements[2], "r.TIMESTAMP >= TO_TIMESTAMP_TZ('2026-03-08 12:34:56.000 +0000'") {
		t.Fatalf("request-only query missing request cursor:\n%s", statements[2])
	}
	if !strings.Contains(statements[4], "(record_id, sentiment, sentiment_source, sentiment_reason)") {
		t.Fatalf("selection-stage insert should only stage metadata:\n%s", statements[4])
	}
	if strings.Contains(statements[4], "question") || strings.Contains(statements[4], "request_value") {
		t.Fatalf("selection-stage insert should not inline large request payloads:\n%s", statements[4])
	}
	if !strings.Contains(statements[5], "WITH selected_records AS (") {
		t.Fatalf("merge statement should load rows from selected source records:\n%s", statements[5])
	}
	if !strings.Contains(statements[5], "GET_AI_OBSERVABILITY_EVENTS('SRC_DB', 'SRC_SC', 'agent', 'CORTEX AGENT')") {
		t.Fatalf("merge statement should reload rows from observability events:\n%s", statements[5])
	}
}

func TestMergeFeedbackRecords(t *testing.T) {
	explicit := []FeedbackRecord{
		{RecordID: "r1", Timestamp: "2026-03-08 10:00:00.000 UTC", FeedbackMessage: "explicit"},
	}
	inferred := []FeedbackRecord{
		{RecordID: "r1", Timestamp: "2026-03-08 11:00:00.000 UTC", SentimentSource: "inferred"},
		{RecordID: "r2", Timestamp: "2026-03-08 12:00:00.000 UTC", SentimentSource: "inferred"},
	}

	t.Run("infer disabled returns explicit only", func(t *testing.T) {
		got := mergeFeedbackRecords(explicit, inferred, false)
		if len(got) != 1 || got[0].RecordID != "r1" {
			t.Fatalf("got = %+v, want only explicit r1", got)
		}
	})

	t.Run("infer enabled appends non-duplicate inferred and sorts", func(t *testing.T) {
		got := mergeFeedbackRecords(explicit, inferred, true)
		if len(got) != 2 {
			t.Fatalf("len(got) = %d, want 2", len(got))
		}
		if got[0].RecordID != "r2" || got[1].RecordID != "r1" {
			t.Fatalf("unexpected order/contents: %+v", got)
		}
	})
}
