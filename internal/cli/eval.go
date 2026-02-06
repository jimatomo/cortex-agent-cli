package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"coragent/internal/agent"
	"coragent/internal/api"
	"coragent/internal/auth"
	"coragent/internal/config"

	"github.com/spf13/cobra"
)

// EvalResult holds the result of a single evaluation test case.
type EvalResult struct {
	Question       string   `json:"question"`
	ExpectedTools  []string `json:"expected_tools"`
	ActualTools    []string `json:"actual_tools"`
	ToolMatch      bool     `json:"tool_match"`
	ExtraToolCalls bool     `json:"extra_tool_calls"`
	Response       string   `json:"response"`
	ThreadID       string   `json:"thread_id"`
	Error          string   `json:"error,omitempty"`
}

// EvalReport holds the full evaluation report.
type EvalReport struct {
	AgentName   string       `json:"agent_name"`
	Database    string       `json:"database"`
	Schema      string       `json:"schema"`
	EvaluatedAt string       `json:"evaluated_at"`
	Results     []EvalResult `json:"results"`
}

func newEvalCmd(opts *RootOptions) *cobra.Command {
	var outputDir string
	var recursive bool

	cmd := &cobra.Command{
		Use:   "eval [path]",
		Short: "Evaluate agent accuracy using test cases",
		Long: `Evaluate a Cortex Agent by running test cases defined in the spec file's eval section.
Each test case sends a question to the agent and checks if the expected tools were used.
Results are output as JSON and Markdown reports.

Agents without an eval section are skipped.`,
		Example: `  # Run evaluation (current directory)
  coragent eval

  # Specify agent spec file
  coragent eval agent.yaml

  # Evaluate all agents in a directory
  coragent eval ./agents/

  # Recursive directory scan
  coragent eval ./agents/ -R

  # Specify output directory
  coragent eval agent.yaml -o ./eval-results`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}

			// 1. Load agents from file or directory
			specs, err := agent.LoadAgents(path, recursive)
			if err != nil {
				return err
			}

			// Filter agents that have eval tests
			var evalSpecs []agent.ParsedAgent
			for _, item := range specs {
				if item.Spec.Eval != nil && len(item.Spec.Eval.Tests) > 0 {
					evalSpecs = append(evalSpecs, item)
				}
			}
			if len(evalSpecs) == 0 {
				return fmt.Errorf("no eval tests defined in any agent in %s", path)
			}

			// 2. Setup auth and client
			cfg := auth.LoadConfig(opts.Connection)
			applyAuthOverrides(&cfg, opts)

			client, err := api.NewClientWithDebug(cfg, opts.Debug)
			if err != nil {
				return err
			}

			// Apply config file output dir if flag not explicitly set
			if !cmd.Flags().Changed("output-dir") {
				appCfg := config.LoadCoragentConfig()
				if appCfg.Eval.OutputDir != "" {
					outputDir = appCfg.Eval.OutputDir
				}
			}

			// Ensure output directory exists
			if err := os.MkdirAll(outputDir, 0o755); err != nil {
				return fmt.Errorf("create output dir: %w", err)
			}

			// 3. Evaluate each agent
			for _, item := range evalSpecs {
				target, err := ResolveTarget(item.Spec, opts, cfg)
				if err != nil {
					return fmt.Errorf("%s: %w", item.Path, err)
				}

				if err := runEvalForAgent(client, target, item.Spec, outputDir); err != nil {
					return fmt.Errorf("%s: %w", item.Path, err)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&outputDir, "output-dir", "o", ".", "Output directory for reports")
	cmd.Flags().BoolVarP(&recursive, "recursive", "R", false, "Recursively load agents from subdirectories")

	return cmd
}

func runEvalForAgent(client *api.Client, target Target, spec agent.AgentSpec, outputDir string) error {
	report := EvalReport{
		AgentName:   spec.Name,
		Database:    target.Database,
		Schema:      target.Schema,
		EvaluatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	jsonPath := filepath.Join(outputDir, spec.Name+"_eval.json")
	mdPath := filepath.Join(outputDir, spec.Name+"_eval.md")

	tests := spec.Eval.Tests
	fmt.Fprintf(os.Stderr, "Evaluating %s (%d tests)...\n", spec.Name, len(tests))

	// Run each test case
	for i, tc := range tests {
		result := runEvalTest(client, target, spec.Name, tc, i+1, len(tests))
		report.Results = append(report.Results, result)

		// Write intermediate JSON after each test
		if err := writeEvalJSON(jsonPath, report); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write intermediate JSON: %v\n", err)
		}
	}

	// Write final JSON
	if err := writeEvalJSON(jsonPath, report); err != nil {
		return fmt.Errorf("write JSON report: %w", err)
	}

	// Write Markdown report
	if err := writeEvalMarkdown(mdPath, report); err != nil {
		return fmt.Errorf("write Markdown report: %w", err)
	}

	// Print summary
	passed := 0
	for _, r := range report.Results {
		if r.ToolMatch {
			passed++
		}
	}
	fmt.Fprintf(os.Stderr, "\nResults: %d/%d passed\n", passed, len(report.Results))
	fmt.Fprintf(os.Stderr, "Output: %s\n", jsonPath)
	fmt.Fprintf(os.Stderr, "Report: %s\n", mdPath)

	return nil
}

func runEvalTest(client *api.Client, target Target, agentName string, tc agent.EvalTestCase, num, total int) EvalResult {
	result := EvalResult{
		Question:      tc.Question,
		ExpectedTools: tc.ExpectedTools,
		ActualTools:   []string{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// Create a new thread for each test
	threadID, err := client.CreateThread(ctx)
	if err != nil {
		result.Error = fmt.Sprintf("create thread: %v", err)
		fmt.Fprintf(os.Stderr, "[%d/%d] %s ... ❌ (%s)\n", num, total, tc.Question, result.Error)
		return result
	}
	result.ThreadID = threadID

	zero := int64(0)
	req := api.RunAgentRequest{
		Messages: []api.Message{
			api.NewTextMessage("user", tc.Question),
		},
		ThreadID:        threadID,
		ParentMessageID: &zero,
	}

	// Capture tools and response
	var toolsUsed []string
	var responseText strings.Builder

	runOpts := api.RunAgentOptions{
		OnToolUse: func(name string, input json.RawMessage) {
			toolsUsed = append(toolsUsed, name)
		},
		OnTextDelta: func(delta string) {
			responseText.WriteString(delta)
		},
	}

	_, err = client.RunAgent(ctx, target.Database, target.Schema, agentName, req, runOpts)
	if err != nil {
		result.Error = fmt.Sprintf("run agent: %v", err)
		result.ActualTools = toolsUsed
		result.ToolMatch = checkToolMatch(tc.ExpectedTools, toolsUsed)
		result.Response = responseText.String()
		fmt.Fprintf(os.Stderr, "[%d/%d] %s ... ❌ (%s)\n", num, total, tc.Question, result.Error)
		return result
	}

	result.ActualTools = toolsUsed
	result.Response = responseText.String()
	result.ToolMatch = checkToolMatch(tc.ExpectedTools, toolsUsed)
	result.ExtraToolCalls = hasExtraToolCalls(tc.ExpectedTools, toolsUsed)

	if !result.ToolMatch {
		fmt.Fprintf(os.Stderr, "[%d/%d] %s ... ❌ (expected: %s, actual: %s)\n",
			num, total, tc.Question,
			strings.Join(tc.ExpectedTools, ", "),
			strings.Join(toolsUsed, ", "))
	} else if result.ExtraToolCalls {
		fmt.Fprintf(os.Stderr, "[%d/%d] %s ... ⚠️ (tools: %s) extra tool calls detected\n", num, total, tc.Question, strings.Join(toolsUsed, ", "))
	} else {
		fmt.Fprintf(os.Stderr, "[%d/%d] %s ... ✅ (tools: %s)\n", num, total, tc.Question, strings.Join(toolsUsed, ", "))
	}

	return result
}

// hasExtraToolCalls returns true if actual tools contain duplicate calls
// or tools not listed in expected, suggesting the agent struggled to find results.
func hasExtraToolCalls(expected, actual []string) bool {
	expectedSet := make(map[string]bool, len(expected))
	for _, t := range expected {
		expectedSet[t] = true
	}
	seen := make(map[string]bool, len(actual))
	for _, t := range actual {
		if seen[t] {
			return true // duplicate call
		}
		seen[t] = true
		if !expectedSet[t] {
			return true // unexpected tool
		}
	}
	return false
}

// checkToolMatch returns true if all expected tools are present in actual tools.
func checkToolMatch(expected, actual []string) bool {
	actualSet := make(map[string]bool, len(actual))
	for _, t := range actual {
		actualSet[t] = true
	}
	for _, t := range expected {
		if !actualSet[t] {
			return false
		}
	}
	return true
}

func writeEvalJSON(path string, report EvalReport) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func writeEvalMarkdown(path string, report EvalReport) error {
	md := generateEvalMarkdown(report)
	return os.WriteFile(path, []byte(md), 0o644)
}

func generateEvalMarkdown(report EvalReport) string {
	var b strings.Builder

	fmt.Fprintf(&b, "## Agent Evaluation: %s\n\n", report.AgentName)

	// Summary table
	b.WriteString("| # | Question | Expected Tools | Actual Tools | Result |\n")
	b.WriteString("|---|----------|---------------|--------------|--------|\n")

	passed := 0
	warned := 0
	for i, r := range report.Results {
		icon := "✅"
		if !r.ToolMatch {
			icon = "❌"
		} else if r.ExtraToolCalls {
			icon = "⚠️"
			passed++
			warned++
		} else {
			passed++
		}
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s |\n",
			i+1,
			r.Question,
			formatToolList(r.ExpectedTools),
			formatToolList(r.ActualTools),
			icon,
		)
	}

	fmt.Fprintf(&b, "\n**Result: %d/%d passed", passed, len(report.Results))
	if warned > 0 {
		fmt.Fprintf(&b, " (%d warned)", warned)
	}
	b.WriteString("**\n")

	// Detail sections
	for i, r := range report.Results {
		icon := "✅"
		if !r.ToolMatch {
			icon = "❌"
		} else if r.ExtraToolCalls {
			icon = "⚠️"
		}
		fmt.Fprintf(&b, "\n<details>\n<summary>Q%d: %s %s</summary>\n\n", i+1, r.Question, icon)
		fmt.Fprintf(&b, "**Expected Tools:** %s\n", formatToolList(r.ExpectedTools))
		fmt.Fprintf(&b, "**Actual Tools:** %s\n", formatToolList(r.ActualTools))

		if r.ExtraToolCalls {
			b.WriteString("\n**Warning:** Extra tool calls detected. The agent may have failed to retrieve the expected results.\n")
		}

		if r.Error != "" {
			fmt.Fprintf(&b, "\n**Error:** %s\n", r.Error)
		}

		fmt.Fprintf(&b, "\n**Response:**\n\n%s\n", r.Response)
		b.WriteString("\n</details>\n")
	}

	return b.String()
}

func formatToolList(tools []string) string {
	if len(tools) == 0 {
		return "(none)"
	}
	parts := make([]string, len(tools))
	for i, t := range tools {
		parts[i] = "`" + t + "`"
	}
	return strings.Join(parts, ", ")
}
