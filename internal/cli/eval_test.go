package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"coragent/internal/agent"
	"coragent/internal/config"
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
			got := computeOverallPass(tt.result, tt.tc, 0)
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

func intPtr(n int) *int { return &n }

func TestComputeOverallPassWithResponseScoreThreshold(t *testing.T) {
	tests := []struct {
		name      string
		result    EvalResult
		tc        agent.EvalTestCase
		threshold int
		want      bool
	}{
		{
			name: "score above threshold",
			result: EvalResult{
				ResponseScore: intPtr(85),
			},
			tc:        agent.EvalTestCase{ExpectedResponse: "expected"},
			threshold: 70,
			want:      true,
		},
		{
			name: "score below threshold",
			result: EvalResult{
				ResponseScore: intPtr(50),
			},
			tc:        agent.EvalTestCase{ExpectedResponse: "expected"},
			threshold: 70,
			want:      false,
		},
		{
			name: "score equals threshold",
			result: EvalResult{
				ResponseScore: intPtr(70),
			},
			tc:        agent.EvalTestCase{ExpectedResponse: "expected"},
			threshold: 70,
			want:      true,
		},
		{
			name: "no threshold (0) - score ignored",
			result: EvalResult{
				ResponseScore: intPtr(10),
			},
			tc:        agent.EvalTestCase{ExpectedResponse: "expected"},
			threshold: 0,
			want:      true,
		},
		{
			name: "nil score with threshold - passes",
			result: EvalResult{
				ResponseScore: nil,
			},
			tc:        agent.EvalTestCase{ExpectedResponse: "expected"},
			threshold: 70,
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeOverallPass(tt.result, tt.tc, tt.threshold)
			if got != tt.want {
				t.Errorf("computeOverallPass() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseJudgeResponse(t *testing.T) {
	t.Run("valid response", func(t *testing.T) {
		raw := `{"structured_output":[{"raw_message":{"score":85,"reasoning":"Good match"},"type":"json"}]}`
		result, err := parseJudgeResponse(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Score != 85 {
			t.Errorf("score = %d, want 85", result.Score)
		}
		if result.Reasoning != "Good match" {
			t.Errorf("reasoning = %q, want %q", result.Reasoning, "Good match")
		}
	})

	t.Run("full COMPLETE response", func(t *testing.T) {
		raw := `{
			"created": 1771036914,
			"model": "llama4-scout",
			"structured_output": [
				{
					"raw_message": {"score": 92, "reasoning": "Both responses correctly identify the expense"},
					"type": "json"
				}
			],
			"usage": {"completion_tokens": 110, "prompt_tokens": 176, "total_tokens": 286}
		}`
		result, err := parseJudgeResponse(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Score != 92 {
			t.Errorf("score = %d, want 92", result.Score)
		}
	})

	t.Run("score clamped to 100", func(t *testing.T) {
		raw := `{"structured_output":[{"raw_message":{"score":150,"reasoning":"overflow"},"type":"json"}]}`
		result, err := parseJudgeResponse(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Score != 100 {
			t.Errorf("score = %d, want 100", result.Score)
		}
	})

	t.Run("score clamped to 0", func(t *testing.T) {
		raw := `{"structured_output":[{"raw_message":{"score":-5,"reasoning":"negative"},"type":"json"}]}`
		result, err := parseJudgeResponse(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Score != 0 {
			t.Errorf("score = %d, want 0", result.Score)
		}
	})

	t.Run("empty structured_output", func(t *testing.T) {
		raw := `{"structured_output":[]}`
		_, err := parseJudgeResponse(raw)
		if err == nil {
			t.Fatal("expected error for empty structured_output")
		}
	})

	t.Run("missing structured_output", func(t *testing.T) {
		raw := `{"model":"llama4-scout"}`
		_, err := parseJudgeResponse(raw)
		if err == nil {
			t.Fatal("expected error for missing structured_output")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := parseJudgeResponse("not json")
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})
}

func TestGenerateEvalMarkdownWithScore(t *testing.T) {
	report := EvalReport{
		AgentName:   "TEST-AGENT",
		Database:    "TEST_DB",
		Schema:      "PUBLIC",
		EvaluatedAt: "2025-01-15T10:30:00Z",
		Results: []EvalResult{
			{
				Question:            "Q4の売上を教えて",
				ExpectedTools:       []string{"revenue_view"},
				ActualTools:         []string{"revenue_view"},
				ToolMatch:           true,
				Response:            "Q4の総売上は1.2億円でした",
				ExpectedResponse:    "Q4の売上は約1.2億円でした",
				ResponseScore:       intPtr(92),
				ResponseScoreReason: "Both correctly identify Q4 revenue",
				JudgeModel:          "llama4-scout",
				Passed:              true,
				ThreadID:            "123",
			},
		},
	}

	md := generateEvalMarkdown(report)

	// Check Score column header
	if !strings.Contains(md, "| Score") {
		t.Error("missing Score column header")
	}

	// Check score value in table
	if !strings.Contains(md, "92") {
		t.Error("missing score value in table")
	}

	// Check detail section
	if !strings.Contains(md, "**Expected Response:** Q4の売上は約1.2億円でした") {
		t.Error("missing expected response in details")
	}
	if !strings.Contains(md, "**Response Score:** 92/100") {
		t.Error("missing response score in details")
	}
	if !strings.Contains(md, "**Score Reasoning:** Both correctly identify Q4 revenue") {
		t.Error("missing score reasoning in details")
	}
	if !strings.Contains(md, "**Judge Model:** llama4-scout") {
		t.Error("missing judge model in details")
	}
}

func TestResolveJudgeModel(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		spec := agent.AgentSpec{}
		cfg := config.CoragentConfig{}
		got := resolveJudgeModel(spec, cfg)
		if got != defaultJudgeModel {
			t.Errorf("got %q, want %q", got, defaultJudgeModel)
		}
	})

	t.Run("config.toml override", func(t *testing.T) {
		spec := agent.AgentSpec{}
		cfg := config.CoragentConfig{}
		cfg.Eval.JudgeModel = "custom-model"
		got := resolveJudgeModel(spec, cfg)
		if got != "custom-model" {
			t.Errorf("got %q, want %q", got, "custom-model")
		}
	})

	t.Run("agent spec overrides config.toml", func(t *testing.T) {
		spec := agent.AgentSpec{
			Eval: &agent.EvalConfig{
				JudgeModel: "spec-model",
			},
		}
		cfg := config.CoragentConfig{}
		cfg.Eval.JudgeModel = "config-model"
		got := resolveJudgeModel(spec, cfg)
		if got != "spec-model" {
			t.Errorf("got %q, want %q", got, "spec-model")
		}
	})
}
