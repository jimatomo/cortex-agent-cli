package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"coragent/internal/agent"
)

func TestEvalOutputPaths(t *testing.T) {
	t.Run("without timestamp", func(t *testing.T) {
		jsonPath, mdPath := evalOutputPaths("./out", "my-agent", false)
		if jsonPath != "out/my-agent_eval.json" {
			t.Errorf("jsonPath = %q, want %q", jsonPath, "out/my-agent_eval.json")
		}
		if mdPath != "out/my-agent_eval.md" {
			t.Errorf("mdPath = %q, want %q", mdPath, "out/my-agent_eval.md")
		}
	})

	t.Run("with timestamp", func(t *testing.T) {
		jsonPath, mdPath := evalOutputPaths("./out", "my-agent", true)
		// Should match pattern: my-agent_eval_YYYYMMDD_HHMMSS.json
		if !strings.Contains(jsonPath, "my-agent_eval_") {
			t.Errorf("jsonPath missing timestamp suffix: %q", jsonPath)
		}
		if !strings.HasSuffix(jsonPath, ".json") {
			t.Errorf("jsonPath should end with .json: %q", jsonPath)
		}
		if !strings.Contains(mdPath, "my-agent_eval_") {
			t.Errorf("mdPath missing timestamp suffix: %q", mdPath)
		}
		if !strings.HasSuffix(mdPath, ".md") {
			t.Errorf("mdPath should end with .md: %q", mdPath)
		}
		// Verify timestamp format (8 digits _ 6 digits)
		// Extract suffix between "my-agent_eval_" and ".json"
		base := filepath.Base(jsonPath)
		ts := strings.TrimPrefix(base, "my-agent_eval_")
		ts = strings.TrimSuffix(ts, ".json")
		if len(ts) != 15 { // YYYYMMDD_HHMMSS = 15 chars
			t.Errorf("timestamp suffix %q has unexpected length %d", ts, len(ts))
		}
	})

	t.Run("dot output dir", func(t *testing.T) {
		jsonPath, _ := evalOutputPaths(".", "agent", false)
		if jsonPath != "agent_eval.json" {
			t.Errorf("jsonPath = %q, want %q", jsonPath, "agent_eval.json")
		}
	})
}

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
				Passed:        true,
				Response:      "売上データによると...",
				ThreadID:      "12345",
			},
			{
				Question:      "ドキュメントを検索して",
				ExpectedTools: []string{"snowflake_docs_service"},
				ActualTools:   []string{"other_tool"},
				ToolMatch:     false,
				Passed:        false,
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
				Passed:         true,
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

func boolPtr(b bool) *bool { return &b }

func TestComputeOverallPass(t *testing.T) {
	tests := []struct {
		name   string
		result EvalResult
		tc     agent.EvalTestCase
		want   bool
	}{
		{
			name: "tools only - match",
			result: EvalResult{
				ToolMatch: true,
			},
			tc:   agent.EvalTestCase{ExpectedTools: []string{"tool_a"}},
			want: true,
		},
		{
			name: "tools only - no match",
			result: EvalResult{
				ToolMatch: false,
			},
			tc:   agent.EvalTestCase{ExpectedTools: []string{"tool_a"}},
			want: false,
		},
		{
			name: "command only - pass",
			result: EvalResult{
				CommandPassed: boolPtr(true),
			},
			tc:   agent.EvalTestCase{Command: "echo ok"},
			want: true,
		},
		{
			name: "command only - fail",
			result: EvalResult{
				CommandPassed: boolPtr(false),
			},
			tc:   agent.EvalTestCase{Command: "echo ok"},
			want: false,
		},
		{
			name: "both - both pass",
			result: EvalResult{
				ToolMatch:     true,
				CommandPassed: boolPtr(true),
			},
			tc:   agent.EvalTestCase{ExpectedTools: []string{"tool_a"}, Command: "echo ok"},
			want: true,
		},
		{
			name: "both - tools fail",
			result: EvalResult{
				ToolMatch:     false,
				CommandPassed: boolPtr(true),
			},
			tc:   agent.EvalTestCase{ExpectedTools: []string{"tool_a"}, Command: "echo ok"},
			want: false,
		},
		{
			name: "both - command fail",
			result: EvalResult{
				ToolMatch:     true,
				CommandPassed: boolPtr(false),
			},
			tc:   agent.EvalTestCase{ExpectedTools: []string{"tool_a"}, Command: "echo ok"},
			want: false,
		},
		{
			name: "error overrides everything",
			result: EvalResult{
				ToolMatch:     true,
				CommandPassed: boolPtr(true),
				Error:         "run agent: timeout",
			},
			tc:   agent.EvalTestCase{ExpectedTools: []string{"tool_a"}, Command: "echo ok"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeOverallPass(tt.result, tt.tc)
			if got != tt.want {
				t.Errorf("computeOverallPass() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRunEvalCommand(t *testing.T) {
	dir := t.TempDir()

	t.Run("exit 0", func(t *testing.T) {
		input := CommandInput{Question: "test", Response: "resp", ActualTools: []string{"tool_a"}}
		out, err := runEvalCommand(context.Background(), "echo ok", input, dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "ok") {
			t.Errorf("expected output to contain 'ok', got %q", out)
		}
	})

	t.Run("exit non-zero", func(t *testing.T) {
		input := CommandInput{Question: "test"}
		_, err := runEvalCommand(context.Background(), "exit 1", input, dir)
		if err == nil {
			t.Fatal("expected error for exit 1, got nil")
		}
	})

	t.Run("reads stdin JSON", func(t *testing.T) {
		// Script that reads stdin and echoes question field
		script := filepath.Join(dir, "read_stdin.sh")
		os.WriteFile(script, []byte("#!/bin/sh\ncat\n"), 0o755)

		input := CommandInput{
			Question:    "hello",
			Response:    "world",
			ActualTools: []string{"tool_a"},
			ThreadID:    "t123",
		}
		out, err := runEvalCommand(context.Background(), "sh "+script, input, dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, `"question":"hello"`) {
			t.Errorf("expected JSON with question field, got %q", out)
		}
	})

	t.Run("captures stderr", func(t *testing.T) {
		input := CommandInput{Question: "test"}
		out, err := runEvalCommand(context.Background(), "echo err >&2", input, dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "err") {
			t.Errorf("expected stderr output, got %q", out)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately
		input := CommandInput{Question: "test"}
		_, err := runEvalCommand(ctx, "sleep 10", input, dir)
		if err == nil {
			t.Fatal("expected error for cancelled context, got nil")
		}
	})
}

func TestGenerateEvalMarkdownWithCommand(t *testing.T) {
	report := EvalReport{
		AgentName:   "TEST-AGENT",
		Database:    "TEST_DB",
		Schema:      "PUBLIC",
		EvaluatedAt: "2025-01-15T10:30:00Z",
		Results: []EvalResult{
			{
				Question:      "test with both",
				ExpectedTools: []string{"tool_a"},
				ActualTools:   []string{"tool_a"},
				ToolMatch:     true,
				Command:       "python check.py",
				CommandPassed: boolPtr(true),
				CommandOutput: "score: 0.95",
				Passed:        true,
				Response:      "response...",
				ThreadID:      "123",
			},
			{
				Question:      "command only fail",
				ActualTools:   []string{},
				Command:       "python eval.py",
				CommandPassed: boolPtr(false),
				CommandError:  "score below threshold",
				CommandOutput: "score: 0.2",
				Passed:        false,
				Response:      "bad response...",
				ThreadID:      "124",
			},
		},
	}

	md := generateEvalMarkdown(report)

	// Check Command column header exists
	if !strings.Contains(md, "| Command |") {
		t.Error("missing Command column header")
	}

	// Check command details in detail section
	if !strings.Contains(md, "**Command:** `python check.py`") {
		t.Error("missing command name in details")
	}
	if !strings.Contains(md, "**Command Result:** ✅ passed") {
		t.Error("missing command pass result")
	}
	if !strings.Contains(md, "**Command Result:** ❌ failed") {
		t.Error("missing command fail result")
	}
	if !strings.Contains(md, "score: 0.95") {
		t.Error("missing command output")
	}
	if !strings.Contains(md, "score below threshold") {
		t.Error("missing command error")
	}

	// Summary should show 1/2 passed
	if !strings.Contains(md, "**Result: 1/2 passed**") {
		t.Errorf("missing or incorrect result summary, got:\n%s", md)
	}
}
