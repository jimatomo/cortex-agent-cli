package regression_test

import (
	"context"
	"net/url"
	"testing"

	"coragent/internal/agent"
	"coragent/internal/api"
	"coragent/internal/auth"
	"coragent/internal/regression"
)

const (
	testDB     = "TESTDB"
	testSchema = "TESTSCHEMA"
)

// newTestClient creates an api.Client pointed at the given mock server URL,
// using a generated RSA key so that BearerToken succeeds without real credentials.
func newTestClient(t *testing.T, ms *regression.MockServer) *api.Client {
	t.Helper()
	base, err := url.Parse(ms.URL())
	if err != nil {
		t.Fatalf("parse mock URL: %v", err)
	}
	return api.NewClientForTest(base, auth.Config{
		Account:    "TEST",
		User:       "TESTUSER",
		PrivateKey: regression.TestRSAPEM(t),
	})
}

// TestLifecycle_FullScenario exercises validate → plan → apply → describe → delete.
//
// This scenario verifies the complete agent lifecycle:
//  1. Agent does not exist initially.
//  2. After apply, the agent is created and describable.
//  3. After delete, the agent no longer exists.
func TestLifecycle_FullScenario(t *testing.T) {
	ms := regression.NewMockServer(t)
	client := newTestClient(t, ms)
	ctx := context.Background()

	spec := agent.AgentSpec{
		Name:    "my-agent",
		Comment: "regression test agent",
	}

	// 1. Agent does not exist yet.
	_, exists, err := client.GetAgent(ctx, testDB, testSchema, spec.Name)
	if err != nil {
		t.Fatalf("GetAgent (before create): %v", err)
	}
	if exists {
		t.Fatal("expected agent to not exist before creation")
	}

	// 2. Create (apply) the agent.
	if err := client.CreateAgent(ctx, testDB, testSchema, spec); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// 3. Agent now exists; describe it.
	got, exists, err := client.GetAgent(ctx, testDB, testSchema, spec.Name)
	if err != nil {
		t.Fatalf("GetAgent (after create): %v", err)
	}
	if !exists {
		t.Fatal("expected agent to exist after creation")
	}
	if got.Name != spec.Name {
		t.Errorf("GetAgent.Name = %q, want %q", got.Name, spec.Name)
	}

	// 4. Update the agent (change comment).
	spec.Comment = "updated comment"
	updatePayload := map[string]any{
		"name":    spec.Name,
		"comment": spec.Comment,
	}
	if err := client.UpdateAgent(ctx, testDB, testSchema, spec.Name, updatePayload); err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}

	// 5. Delete the agent.
	if err := client.DeleteAgent(ctx, testDB, testSchema, spec.Name); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}

	// 6. Agent no longer exists.
	_, exists, err = client.GetAgent(ctx, testDB, testSchema, spec.Name)
	if err != nil {
		t.Fatalf("GetAgent (after delete): %v", err)
	}
	if exists {
		t.Fatal("expected agent to not exist after deletion")
	}
}

// TestLifecycle_MultipleAgents verifies that multiple agents can be managed
// independently in the same database/schema.
func TestLifecycle_MultipleAgents(t *testing.T) {
	ms := regression.NewMockServer(t)
	client := newTestClient(t, ms)
	ctx := context.Background()

	names := []string{"agent-a", "agent-b", "agent-c"}
	for _, name := range names {
		if err := client.CreateAgent(ctx, testDB, testSchema, agent.AgentSpec{Name: name}); err != nil {
			t.Fatalf("CreateAgent(%q): %v", name, err)
		}
	}

	listed, err := client.ListAgents(ctx, testDB, testSchema)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(listed) != len(names) {
		t.Errorf("ListAgents = %d agents, want %d", len(listed), len(names))
	}

	// Delete one and verify the others are unaffected.
	if err := client.DeleteAgent(ctx, testDB, testSchema, "agent-b"); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}

	_, exists, err := client.GetAgent(ctx, testDB, testSchema, "agent-b")
	if err != nil {
		t.Fatalf("GetAgent after delete: %v", err)
	}
	if exists {
		t.Error("expected agent-b to not exist after deletion")
	}

	_, exists, err = client.GetAgent(ctx, testDB, testSchema, "agent-a")
	if err != nil {
		t.Fatalf("GetAgent agent-a: %v", err)
	}
	if !exists {
		t.Error("expected agent-a to still exist")
	}
}
