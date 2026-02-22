package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"coragent/internal/agent"
	"coragent/internal/api"
	"coragent/internal/config"
)

const defaultJudgeModel = "llama4-scout"

// defaultIgnoreTools lists tool names that are excluded from eval tool matching
// by default. These are utility tools (e.g. visualization) that agents may call
// autonomously and should not affect tool-match evaluation.
var defaultIgnoreTools = []string{
	"data_to_chart",
}

// evalOptions holds resolved eval configuration.
type evalOptions struct {
	judgeModel             string
	responseScoreThreshold int
	ignoreTools            []string
}

// judgeResult is the structured output from the LLM judge.
type judgeResult struct {
	Score     int    `json:"score"`
	Reasoning string `json:"reasoning"`
}

// resolveJudgeModel returns the judge model using priority:
// agent spec > config.toml > default.
func resolveJudgeModel(spec agent.AgentSpec, appCfg config.CoragentConfig) string {
	if spec.Eval != nil && strings.TrimSpace(spec.Eval.JudgeModel) != "" {
		return spec.Eval.JudgeModel
	}
	if strings.TrimSpace(appCfg.Eval.JudgeModel) != "" {
		return appCfg.Eval.JudgeModel
	}
	return defaultJudgeModel
}

// resolveResponseScoreThreshold returns the agent-level threshold using priority:
// agent spec > config.toml > 0 (disabled).
func resolveResponseScoreThreshold(spec agent.AgentSpec, appCfg config.CoragentConfig) int {
	if spec.Eval != nil && spec.Eval.ResponseScoreThreshold != nil {
		return *spec.Eval.ResponseScoreThreshold
	}
	return appCfg.Eval.ResponseScoreThreshold
}

// effectiveThreshold returns the threshold for a specific test case using priority:
// test case > agent-level default.
func effectiveThreshold(tc agent.EvalTestCase, agentDefault int) int {
	if tc.ResponseScoreThreshold != nil {
		return *tc.ResponseScoreThreshold
	}
	return agentDefault
}

// judgeResponse calls SNOWFLAKE.CORTEX.COMPLETE with structured output to score
// the actual response against the expected response. Returns score (0-100) and reasoning.
func judgeResponse(ctx context.Context, client *api.Client, model, question, expectedResponse, actualResponse string) (judgeResult, error) {
	prompt := fmt.Sprintf(
		"You are an evaluation judge. Compare the actual response to the expected response for the given question.\n\n"+
			"Question: %s\n\nExpected Response: %s\n\nActual Response: %s\n\n"+
			"Score the actual response from 0 to 100 based on how well it matches the expected response in meaning and correctness. "+
			"Provide a brief reasoning.",
		question, expectedResponse, actualResponse,
	)

	// Escape single quotes for SQL string literal
	escapedPrompt := strings.ReplaceAll(prompt, "'", "''")

	stmt := fmt.Sprintf(`SELECT SNOWFLAKE.CORTEX.COMPLETE(
    '%s',
    [{'role': 'user', 'content': '%s'}],
    {
        'temperature': 0,
        'response_format': {
            'type': 'json',
            'schema': {
                'type': 'object',
                'properties': {
                    'score': {'type': 'integer', 'description': '0-100 similarity score'},
                    'reasoning': {'type': 'string', 'description': 'Brief explanation of the score'}
                },
                'required': ['score', 'reasoning']
            }
        }
    }
) AS response;`, model, escapedPrompt)

	raw, err := client.CortexComplete(ctx, stmt)
	if err != nil {
		return judgeResult{}, err
	}

	return parseJudgeResponse(raw)
}

// parseJudgeResponse extracts the judgeResult from the COMPLETE function response.
// With response_format, COMPLETE returns structured_output[0].raw_message containing the result directly.
func parseJudgeResponse(raw string) (judgeResult, error) {
	var completeResp struct {
		StructuredOutput []struct {
			RawMessage judgeResult `json:"raw_message"`
		} `json:"structured_output"`
	}
	if err := json.Unmarshal([]byte(raw), &completeResp); err != nil {
		return judgeResult{}, fmt.Errorf("parse complete response: %w", err)
	}
	if len(completeResp.StructuredOutput) == 0 {
		return judgeResult{}, fmt.Errorf("no structured_output in complete response")
	}

	result := completeResp.StructuredOutput[0].RawMessage

	// Clamp score to 0-100
	if result.Score < 0 {
		result.Score = 0
	}
	if result.Score > 100 {
		result.Score = 100
	}

	return result, nil
}

// mergeIgnoreTools combines default and user-defined ignore lists, removing duplicates.
func mergeIgnoreTools(defaults, userDefined []string) []string {
	seen := make(map[string]bool, len(defaults)+len(userDefined))
	var merged []string
	for _, t := range defaults {
		if !seen[t] {
			seen[t] = true
			merged = append(merged, t)
		}
	}
	for _, t := range userDefined {
		if !seen[t] {
			seen[t] = true
			merged = append(merged, t)
		}
	}
	return merged
}

// filterIgnoredTools returns a copy of tools with ignored tools removed.
func filterIgnoredTools(tools []string, ignore []string) []string {
	if len(ignore) == 0 {
		return tools
	}
	ignoreSet := make(map[string]bool, len(ignore))
	for _, t := range ignore {
		ignoreSet[t] = true
	}
	var filtered []string
	for _, t := range tools {
		if !ignoreSet[t] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}
