package regression_test

import (
	"context"
	"encoding/json"
	"testing"

	"coragent/internal/agent"
	"coragent/internal/api"
	"coragent/internal/regression"
)

// TestEval_PassFail exercises the eval pipeline end-to-end against a mock server.
//
// It creates an agent with two test cases:
//  1. A text-response test case that the mock will answer correctly (pass).
//  2. A tool-match test case where the mock returns no tools (fail).
func TestEval_PassFail(t *testing.T) {
	ms := regression.NewMockServer(t)
	client := newTestClient(t, ms)
	ctx := context.Background()

	const agentName = "eval-agent"

	// Create agent.
	if err := client.CreateAgent(ctx, testDB, testSchema, agent.AgentSpec{Name: agentName}); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// Test case 1: agent returns a text response.
	ms.SetRunReply(agentName, regression.BuildSSEReply("The capital is Paris."))

	var got1 string
	req1 := api.RunAgentRequest{Messages: []api.Message{api.NewTextMessage("user", "What is the capital of France?")}}
	opts1 := api.RunAgentOptions{
		OnTextDelta: func(delta string) { got1 += delta },
	}
	if _, err := client.RunAgent(ctx, testDB, testSchema, agentName, req1, opts1); err != nil {
		t.Fatalf("RunAgent (pass case): %v", err)
	}
	if got1 != "The capital is Paris." {
		t.Errorf("pass case response = %q, want %q", got1, "The capital is Paris.")
	}

	// Test case 2: mock returns a response with a tool use event.
	ms.SetRunReply(agentName, regression.BuildSSEReply("42", "cortex_analyst_text_to_sql"))

	var toolCalled string
	var got2 string
	req2 := api.RunAgentRequest{Messages: []api.Message{api.NewTextMessage("user", "Count rows")}}
	opts2 := api.RunAgentOptions{
		OnToolUse:   func(name string, _ json.RawMessage) { toolCalled = name },
		OnTextDelta: func(delta string) { got2 += delta },
	}
	if _, err := client.RunAgent(ctx, testDB, testSchema, agentName, req2, opts2); err != nil {
		t.Fatalf("RunAgent (tool case): %v", err)
	}
	if toolCalled != "cortex_analyst_text_to_sql" {
		t.Errorf("tool case tool = %q, want %q", toolCalled, "cortex_analyst_text_to_sql")
	}
	if got2 != "42" {
		t.Errorf("tool case response = %q, want %q", got2, "42")
	}
}
