package regression_test

import (
	"context"
	"testing"

	"coragent/internal/agent"
	"coragent/internal/api"
	"coragent/internal/regression"
)

// TestThreads_Continuity verifies that a thread ID can be created and then
// reused across multiple RunAgent calls, and that the thread persists in the
// threads list until explicitly deleted.
func TestThreads_Continuity(t *testing.T) {
	ms := regression.NewMockServer(t)
	client := newTestClient(t, ms)
	ctx := context.Background()

	const agentName = "thread-agent"

	// Create agent.
	if err := client.CreateAgent(ctx, testDB, testSchema, agent.AgentSpec{Name: agentName}); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// Register an SSE reply for this agent.
	ms.SetRunReply(agentName, regression.BuildSSEReply("Hello from thread."))

	// 1. Create a new thread.
	threadID, err := client.CreateThread(ctx)
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	if threadID == "" {
		t.Fatal("expected non-empty thread ID")
	}

	// 2. Thread appears in ListThreads.
	threads, err := client.ListThreads(ctx)
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if len(threads) != 1 {
		t.Errorf("ListThreads = %d, want 1", len(threads))
	}

	// 3. First run with the thread ID.
	var resp1 string
	req1 := api.RunAgentRequest{
		Messages: []api.Message{api.NewTextMessage("user", "First message")},
		ThreadID: threadID,
	}
	if _, err := client.RunAgent(ctx, testDB, testSchema, agentName, req1, api.RunAgentOptions{
		OnTextDelta: func(d string) { resp1 += d },
	}); err != nil {
		t.Fatalf("RunAgent (first): %v", err)
	}
	if resp1 == "" {
		t.Error("expected non-empty response from first run")
	}

	// 4. Second run reusing the same thread ID.
	ms.SetRunReply(agentName, regression.BuildSSEReply("Second reply."))
	var resp2 string
	req2 := api.RunAgentRequest{
		Messages: []api.Message{api.NewTextMessage("user", "Second message")},
		ThreadID: threadID,
	}
	if _, err := client.RunAgent(ctx, testDB, testSchema, agentName, req2, api.RunAgentOptions{
		OnTextDelta: func(d string) { resp2 += d },
	}); err != nil {
		t.Fatalf("RunAgent (second): %v", err)
	}
	if resp2 != "Second reply." {
		t.Errorf("second run response = %q, want %q", resp2, "Second reply.")
	}

	// 5. Thread is still retrievable after both runs.
	got, err := client.GetThread(ctx, threadID)
	if err != nil {
		t.Fatalf("GetThread: %v", err)
	}
	if got == nil || got.ThreadID != threadID {
		t.Errorf("GetThread returned unexpected result: %+v", got)
	}

	// 6. Delete the thread; it should no longer exist.
	if err := client.DeleteThread(ctx, threadID); err != nil {
		t.Fatalf("DeleteThread: %v", err)
	}

	threads, err = client.ListThreads(ctx)
	if err != nil {
		t.Fatalf("ListThreads (after delete): %v", err)
	}
	if len(threads) != 0 {
		t.Errorf("expected 0 threads after delete, got %d", len(threads))
	}
}
