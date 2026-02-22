package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"coragent/internal/agent"
	"coragent/internal/api"
	"coragent/internal/config"

	"github.com/spf13/cobra"
)

// EvalResult holds the result of a single evaluation test case.
type EvalResult struct {
	Question            string   `json:"question"`
	ExpectedTools       []string `json:"expected_tools,omitempty"`
	ActualTools         []string `json:"actual_tools"`
	ToolMatch           bool     `json:"tool_match"`
	ExtraToolCalls      bool     `json:"extra_tool_calls"`
	Response            string   `json:"response"`
	ThreadID            string   `json:"thread_id"`
	Command             string   `json:"command,omitempty"`
	CommandPassed       *bool    `json:"command_passed,omitempty"`
	CommandOutput       string   `json:"command_output,omitempty"`
	CommandError        string   `json:"command_error,omitempty"`
	ExpectedResponse    string   `json:"expected_response,omitempty"`
	ResponseScore       *int     `json:"response_score,omitempty"`
	ResponseScoreReason string   `json:"response_score_reason,omitempty"`
	JudgeModel          string   `json:"judge_model,omitempty"`
	ResponseScoreErr    string   `json:"response_score_error,omitempty"`
	Passed              bool     `json:"passed"`
	Error               string   `json:"error,omitempty"`
}

// CommandInput is the JSON payload written to stdin of eval commands.
type CommandInput struct {
	Question         string   `json:"question"`
	Response         string   `json:"response"`
	ActualTools      []string `json:"actual_tools"`
	ExpectedTools    []string `json:"expected_tools,omitempty"`
	ExpectedResponse string   `json:"expected_response,omitempty"`
	ThreadID         string   `json:"thread_id"`
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
			specs, err := agent.LoadAgents(path, recursive, opts.Env)
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
			client, cfg, err := buildClientAndCfg(opts)
			if err != nil {
				return err
			}

			// Apply config file settings if output-dir flag not explicitly set
			appCfg := config.LoadCoragentConfig()
			if !cmd.Flags().Changed("output-dir") {
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

				specDir := filepath.Dir(item.Path)
				eo := evalOptions{
					judgeModel:             resolveJudgeModel(item.Spec, appCfg),
					responseScoreThreshold: resolveResponseScoreThreshold(item.Spec, appCfg),
					ignoreTools:            mergeIgnoreTools(defaultIgnoreTools, appCfg.Eval.IgnoreTools),
				}
				if err := runEvalForAgent(client, target, item.Spec, outputDir, specDir, appCfg.Eval.TimestampSuffix, eo); err != nil {
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

// evalOutputPaths returns the JSON and Markdown output file paths for an eval report.
// When timestampSuffix is true, a UTC timestamp is appended to the base name.
func evalOutputPaths(outputDir, agentName string, timestampSuffix bool) (jsonPath, mdPath string) {
	suffix := ""
	if timestampSuffix {
		suffix = "_" + time.Now().UTC().Format("20060102_150405")
	}
	jsonPath = filepath.Join(outputDir, agentName+"_eval"+suffix+".json")
	mdPath = filepath.Join(outputDir, agentName+"_eval"+suffix+".md")
	return
}

func runEvalForAgent(client *api.Client, target Target, spec agent.AgentSpec, outputDir, specDir string, timestampSuffix bool, eo evalOptions) error {
	report := EvalReport{
		AgentName:   spec.Name,
		Database:    target.Database,
		Schema:      target.Schema,
		EvaluatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	jsonPath, mdPath := evalOutputPaths(outputDir, spec.Name, timestampSuffix)

	tests := spec.Eval.Tests
	fmt.Fprintf(os.Stderr, "Evaluating %s (%d tests)...\n", spec.Name, len(tests))

	// Run each test case
	for i, tc := range tests {
		result := runEvalTest(client, target, spec.Name, tc, i+1, len(tests), specDir, eo)
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
		if r.Passed {
			passed++
		}
	}
	fmt.Fprintf(os.Stderr, "\nResults: %d/%d passed\n", passed, len(report.Results))
	fmt.Fprintf(os.Stderr, "Output: %s\n", jsonPath)
	fmt.Fprintf(os.Stderr, "Report: %s\n", mdPath)

	return nil
}

func runEvalTest(client *api.Client, target Target, agentName string, tc agent.EvalTestCase, num, total int, specDir string, eo evalOptions) EvalResult {
	result := EvalResult{
		Question:         tc.Question,
		ExpectedTools:    tc.ExpectedTools,
		ActualTools:      []string{},
		Command:          tc.Command,
		ExpectedResponse: tc.ExpectedResponse,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// Run agent only when question is specified
	if strings.TrimSpace(tc.Question) != "" {
		threadID, err := client.CreateThread(ctx)
		if err != nil {
			result.Error = fmt.Sprintf("create thread: %v", err)
			result.Passed = false
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
		}

		toolsUsed = filterIgnoredTools(toolsUsed, eo.ignoreTools)
		result.ActualTools = toolsUsed
		result.Response = responseText.String()
		result.ToolMatch = checkToolMatch(tc.ExpectedTools, toolsUsed)
		result.ExtraToolCalls = hasExtraToolCalls(tc.ExpectedTools, toolsUsed)
	}

	// Run command if specified
	if tc.Command != "" {
		input := CommandInput{
			Question:         tc.Question,
			Response:         result.Response,
			ActualTools:      result.ActualTools,
			ExpectedTools:    tc.ExpectedTools,
			ExpectedResponse: tc.ExpectedResponse,
			ThreadID:         result.ThreadID,
		}
		cmdOut, cmdErr := runEvalCommand(ctx, tc.Command, input, specDir)
		result.CommandOutput = cmdOut
		if cmdErr != nil {
			passed := false
			result.CommandPassed = &passed
			result.CommandError = cmdErr.Error()
		} else {
			passed := true
			result.CommandPassed = &passed
		}
	}

	// Run LLM judge if expected_response is set
	if strings.TrimSpace(tc.ExpectedResponse) != "" && result.Response != "" {
		result.JudgeModel = eo.judgeModel
		jr, err := judgeResponse(ctx, client, eo.judgeModel, tc.Question, tc.ExpectedResponse, result.Response)
		if err != nil {
			result.ResponseScoreErr = err.Error()
		} else {
			result.ResponseScore = &jr.Score
			result.ResponseScoreReason = jr.Reasoning
		}
	}

	threshold := effectiveThreshold(tc, eo.responseScoreThreshold)
	result.Passed = computeOverallPass(result, tc, threshold)

	// Console output
	label := tc.Question
	if label == "" {
		label = tc.Command
	}
	if result.Error != "" {
		fmt.Fprintf(os.Stderr, "[%d/%d] %s ... ❌ (%s)\n", num, total, label, result.Error)
	} else if !result.Passed {
		var reasons []string
		if len(tc.ExpectedTools) > 0 && !result.ToolMatch {
			reasons = append(reasons, fmt.Sprintf("expected: %s, actual: %s",
				strings.Join(tc.ExpectedTools, ", "), strings.Join(result.ActualTools, ", ")))
		}
		if result.CommandPassed != nil && !*result.CommandPassed {
			reasons = append(reasons, fmt.Sprintf("command failed: %s", result.CommandError))
		}
		if result.ResponseScore != nil && threshold > 0 && *result.ResponseScore < threshold {
			reasons = append(reasons, fmt.Sprintf("score %d < threshold %d", *result.ResponseScore, threshold))
		}
		fmt.Fprintf(os.Stderr, "[%d/%d] %s ... ❌ (%s)\n", num, total, label, strings.Join(reasons, "; "))
	} else if result.ExtraToolCalls {
		fmt.Fprintf(os.Stderr, "[%d/%d] %s ... ⚠️ (tools: %s) extra tool calls detected\n", num, total, label, strings.Join(result.ActualTools, ", "))
	} else {
		fmt.Fprintf(os.Stderr, "[%d/%d] %s ... ✅\n", num, total, label)
	}
	if result.ResponseScore != nil {
		fmt.Fprintf(os.Stderr, "     Score: %d/100 (%s)\n", *result.ResponseScore, result.JudgeModel)
	}
	if result.ResponseScoreErr != "" {
		fmt.Fprintf(os.Stderr, "     Score error: %s\n", result.ResponseScoreErr)
	}
	if result.CommandOutput != "" {
		fmt.Fprint(os.Stderr, result.CommandOutput)
		if !strings.HasSuffix(result.CommandOutput, "\n") {
			fmt.Fprintln(os.Stderr)
		}
	}

	return result
}

// runEvalCommand executes a command via sh -c, passing input as JSON on stdin.
func runEvalCommand(ctx context.Context, command string, input CommandInput, workDir string) (string, error) {
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("marshal command input: %w", err)
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = workDir
	cmd.Stdin = bytes.NewReader(inputJSON)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	if err != nil {
		return output, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return output, nil
}

// computeOverallPass determines the overall pass/fail for a test case.
// Tool match (if expected_tools specified), command (if specified), and
// response score threshold (if > 0) must all pass.
func computeOverallPass(result EvalResult, tc agent.EvalTestCase, responseScoreThreshold int) bool {
	if result.Error != "" {
		return false
	}
	if len(tc.ExpectedTools) > 0 && !result.ToolMatch {
		return false
	}
	if result.CommandPassed != nil && !*result.CommandPassed {
		return false
	}
	if responseScoreThreshold > 0 && result.ResponseScore != nil && *result.ResponseScore < responseScoreThreshold {
		return false
	}
	return true
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

	// Check optional columns
	hasCommand := false
	hasScore := false
	for _, r := range report.Results {
		if r.Command != "" {
			hasCommand = true
		}
		if r.ResponseScore != nil {
			hasScore = true
		}
	}

	// Build summary table header dynamically
	header := "| # | Question | Expected Tools | Actual Tools"
	sep := "|---|----------|---------------|--------------"
	if hasCommand {
		header += " | Command"
		sep += "|--------"
	}
	if hasScore {
		header += " | Score"
		sep += "|------"
	}
	header += " | Result |\n"
	sep += "|--------|\n"
	b.WriteString(header)
	b.WriteString(sep)

	passed := 0
	warned := 0
	for i, r := range report.Results {
		icon := "✅"
		if !r.Passed {
			icon = "❌"
		} else if r.ExtraToolCalls {
			icon = "⚠️"
			passed++
			warned++
		} else {
			passed++
		}

		cmdStatus := ""
		if r.Command != "" {
			if r.CommandPassed != nil && *r.CommandPassed {
				cmdStatus = "✅"
			} else if r.CommandPassed != nil {
				cmdStatus = "❌"
			}
		}

		scoreStr := ""
		if r.ResponseScore != nil {
			scoreStr = fmt.Sprintf("%d", *r.ResponseScore)
		}

		row := fmt.Sprintf("| %d | %s | %s | %s",
			i+1,
			r.Question,
			formatToolList(r.ExpectedTools),
			formatToolList(r.ActualTools),
		)
		if hasCommand {
			row += fmt.Sprintf(" | %s", cmdStatus)
		}
		if hasScore {
			row += fmt.Sprintf(" | %s", scoreStr)
		}
		row += fmt.Sprintf(" | %s |\n", icon)
		b.WriteString(row)
	}

	fmt.Fprintf(&b, "\n**Result: %d/%d passed", passed, len(report.Results))
	if warned > 0 {
		fmt.Fprintf(&b, " (%d warned)", warned)
	}
	b.WriteString("**\n")

	// Detail sections
	for i, r := range report.Results {
		icon := "✅"
		if !r.Passed {
			icon = "❌"
		} else if r.ExtraToolCalls {
			icon = "⚠️"
		}
		fmt.Fprintf(&b, "\n<details>\n<summary>Q%d: %s %s</summary>\n\n", i+1, r.Question, icon)

		if len(r.ExpectedTools) > 0 {
			fmt.Fprintf(&b, "**Expected Tools:** %s\n", formatToolList(r.ExpectedTools))
		}
		fmt.Fprintf(&b, "**Actual Tools:** %s\n", formatToolList(r.ActualTools))

		if r.ExtraToolCalls {
			b.WriteString("\n**Warning:** Extra tool calls detected. The agent may have failed to retrieve the expected results.\n")
		}

		if r.Error != "" {
			fmt.Fprintf(&b, "\n**Error:** %s\n", r.Error)
		}

		if r.Command != "" {
			fmt.Fprintf(&b, "\n**Command:** `%s`\n", r.Command)
			if r.CommandPassed != nil {
				if *r.CommandPassed {
					b.WriteString("**Command Result:** ✅ passed\n")
				} else {
					b.WriteString("**Command Result:** ❌ failed\n")
				}
			}
			if r.CommandError != "" {
				fmt.Fprintf(&b, "**Command Error:** %s\n", r.CommandError)
			}
			if r.CommandOutput != "" {
				fmt.Fprintf(&b, "\n**Command Output:**\n```\n%s\n```\n", r.CommandOutput)
			}
		}

		if r.ExpectedResponse != "" {
			fmt.Fprintf(&b, "\n**Expected Response:** %s\n", r.ExpectedResponse)
		}
		if r.ResponseScore != nil {
			fmt.Fprintf(&b, "**Response Score:** %d/100\n", *r.ResponseScore)
			if r.ResponseScoreReason != "" {
				fmt.Fprintf(&b, "**Score Reasoning:** %s\n", r.ResponseScoreReason)
			}
			if r.JudgeModel != "" {
				fmt.Fprintf(&b, "**Judge Model:** %s\n", r.JudgeModel)
			}
		}
		if r.ResponseScoreErr != "" {
			fmt.Fprintf(&b, "**Score Error:** %s\n", r.ResponseScoreErr)
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
