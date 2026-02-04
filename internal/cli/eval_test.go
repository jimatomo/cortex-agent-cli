package cli

import (
	"strings"
	"testing"
)

func TestCheckToolMatch(t *testing.T) {
	tests := []struct {
		name     string
		expected []string
		actual   []string
		want     bool
	}{
		{
			name:     "exact match",
			expected: []string{"tool_a"},
			actual:   []string{"tool_a"},
			want:     true,
		},
		{
			name:     "subset match (extra tools ok)",
			expected: []string{"tool_a"},
			actual:   []string{"tool_a", "tool_b"},
			want:     true,
		},
		{
			name:     "missing expected tool",
			expected: []string{"tool_a", "tool_b"},
			actual:   []string{"tool_a"},
			want:     false,
		},
		{
			name:     "no expected tools",
			expected: []string{},
			actual:   []string{"tool_a"},
			want:     true,
		},
		{
			name:     "no actual tools",
			expected: []string{"tool_a"},
			actual:   []string{},
			want:     false,
		},
		{
			name:     "both empty",
			expected: []string{},
			actual:   []string{},
			want:     true,
		},
		{
			name:     "multiple expected all present",
			expected: []string{"tool_a", "tool_b"},
			actual:   []string{"tool_c", "tool_a", "tool_b"},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkToolMatch(tt.expected, tt.actual)
			if got != tt.want {
				t.Errorf("checkToolMatch(%v, %v) = %v, want %v", tt.expected, tt.actual, got, tt.want)
			}
		})
	}
}

func TestGenerateEvalMarkdown(t *testing.T) {
	report := EvalReport{
		AgentName:   "TEST-AGENT",
		Database:    "TEST_DB",
		Schema:      "PUBLIC",
		EvaluatedAt: "2025-01-15T10:30:00Z",
		Results: []EvalResult{
			{
				Question:      "売上データを教えて",
				ExpectedTools: []string{"sample_semantic_view"},
				ActualTools:   []string{"sample_semantic_view"},
				ToolMatch:     true,
				Response:      "売上データによると...",
				ThreadID:      "12345",
			},
			{
				Question:      "ドキュメントを検索して",
				ExpectedTools: []string{"snowflake_docs_service"},
				ActualTools:   []string{"other_tool"},
				ToolMatch:     false,
				Response:      "検索結果...",
				ThreadID:      "12346",
			},
		},
	}

	md := generateEvalMarkdown(report)

	// Check header
	if !strings.Contains(md, "## Agent Evaluation: TEST-AGENT") {
		t.Error("missing agent name header")
	}

	// Check table header
	if !strings.Contains(md, "| # | Question | Expected Tools | Actual Tools | Result |") {
		t.Error("missing table header")
	}

	// Check pass/fail icons
	if !strings.Contains(md, "✅") {
		t.Error("missing check mark icon")
	}
	if !strings.Contains(md, "❌") {
		t.Error("missing x icon")
	}

	// Check result summary
	if !strings.Contains(md, "**Result: 1/2 passed**") {
		t.Error("missing or incorrect result summary")
	}

	// Check details sections
	if !strings.Contains(md, "<details>") {
		t.Error("missing details section")
	}
	if !strings.Contains(md, "Q1: 売上データを教えて") {
		t.Error("missing Q1 detail")
	}
	if !strings.Contains(md, "Q2: ドキュメントを検索して") {
		t.Error("missing Q2 detail")
	}

	// Check tool formatting
	if !strings.Contains(md, "`sample_semantic_view`") {
		t.Error("missing formatted tool name")
	}
}

func TestGenerateEvalMarkdownWithError(t *testing.T) {
	report := EvalReport{
		AgentName:   "TEST-AGENT",
		Database:    "TEST_DB",
		Schema:      "PUBLIC",
		EvaluatedAt: "2025-01-15T10:30:00Z",
		Results: []EvalResult{
			{
				Question:      "test question",
				ExpectedTools: []string{"tool_a"},
				ActualTools:   []string{},
				ToolMatch:     false,
				Response:      "",
				ThreadID:      "123",
				Error:         "connection timeout",
			},
		},
	}

	md := generateEvalMarkdown(report)

	if !strings.Contains(md, "**Error:** connection timeout") {
		t.Error("missing error message in markdown")
	}
	if !strings.Contains(md, "**Result: 0/1 passed**") {
		t.Error("missing or incorrect result summary")
	}
}

func TestHasExtraToolCalls(t *testing.T) {
	tests := []struct {
		name     string
		expected []string
		actual   []string
		want     bool
	}{
		{"exact match", []string{"tool_a"}, []string{"tool_a"}, false},
		{"multiple exact match", []string{"tool_a", "tool_b"}, []string{"tool_a", "tool_b"}, false},
		{"empty both", []string{}, []string{}, false},
		{"duplicate call", []string{"tool_a"}, []string{"tool_a", "tool_a"}, true},
		{"unexpected tool", []string{"tool_a"}, []string{"tool_a", "tool_b"}, true},
		{"only unexpected", []string{"tool_a"}, []string{"tool_b"}, true},
		{"no actual", []string{"tool_a"}, []string{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasExtraToolCalls(tt.expected, tt.actual)
			if got != tt.want {
				t.Errorf("hasExtraToolCalls(%v, %v) = %v, want %v", tt.expected, tt.actual, got, tt.want)
			}
		})
	}
}

func TestGenerateEvalMarkdownWithExtraToolCalls(t *testing.T) {
	report := EvalReport{
		AgentName:   "TEST-AGENT",
		Database:    "TEST_DB",
		Schema:      "PUBLIC",
		EvaluatedAt: "2025-01-15T10:30:00Z",
		Results: []EvalResult{
			{
				Question:       "test question",
				ExpectedTools:  []string{"tool_a"},
				ActualTools:    []string{"tool_a", "tool_b"},
				ToolMatch:      true,
				ExtraToolCalls: true,
				Response:       "response...",
				ThreadID:       "123",
			},
		},
	}

	md := generateEvalMarkdown(report)

	if !strings.Contains(md, "⚠️") {
		t.Error("missing warning icon for extra tool calls")
	}
	if !strings.Contains(md, "Extra tool calls detected") {
		t.Error("missing extra tool calls warning message")
	}
	if !strings.Contains(md, "1/1 passed (1 warned)") {
		t.Errorf("missing or incorrect warned summary, got:\n%s", md)
	}
}

func TestFormatToolList(t *testing.T) {
	tests := []struct {
		name  string
		tools []string
		want  string
	}{
		{"empty", []string{}, "(none)"},
		{"single", []string{"tool_a"}, "`tool_a`"},
		{"multiple", []string{"tool_a", "tool_b"}, "`tool_a`, `tool_b`"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatToolList(tt.tools)
			if got != tt.want {
				t.Errorf("formatToolList(%v) = %q, want %q", tt.tools, got, tt.want)
			}
		})
	}
}
