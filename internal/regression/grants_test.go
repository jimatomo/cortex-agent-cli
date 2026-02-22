package regression_test

import (
	"context"
	"testing"

	"coragent/internal/agent"
	"coragent/internal/regression"
)

// TestGrants_AddRemove verifies that ExecuteGrant and ExecuteRevoke are reflected
// in subsequent ShowGrants calls — simulating grant changes across apply cycles.
func TestGrants_AddRemove(t *testing.T) {
	ms := regression.NewMockServer(t)
	client := newTestClient(t, ms)
	ctx := context.Background()

	// Create agent.
	spec := agent.AgentSpec{Name: "grant-agent"}
	if err := client.CreateAgent(ctx, testDB, testSchema, spec); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// Initially no grants.
	rows, err := client.ShowGrants(ctx, testDB, testSchema, spec.Name)
	if err != nil {
		t.Fatalf("ShowGrants (empty): %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 grants initially, got %d", len(rows))
	}

	// Apply cycle 1: grant USAGE to two roles.
	if err := client.ExecuteGrant(ctx, testDB, testSchema, spec.Name, "ROLE", "ROLE_A", "USAGE"); err != nil {
		t.Fatalf("ExecuteGrant ROLE_A: %v", err)
	}
	if err := client.ExecuteGrant(ctx, testDB, testSchema, spec.Name, "ROLE", "ROLE_B", "USAGE"); err != nil {
		t.Fatalf("ExecuteGrant ROLE_B: %v", err)
	}

	rows, err = client.ShowGrants(ctx, testDB, testSchema, spec.Name)
	if err != nil {
		t.Fatalf("ShowGrants (after cycle 1): %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 grants after cycle 1, got %d", len(rows))
	}

	// Apply cycle 2: remove ROLE_B, keep ROLE_A.
	if err := client.ExecuteRevoke(ctx, testDB, testSchema, spec.Name, "ROLE", "ROLE_B", "USAGE"); err != nil {
		t.Fatalf("ExecuteRevoke ROLE_B: %v", err)
	}

	rows, err = client.ShowGrants(ctx, testDB, testSchema, spec.Name)
	if err != nil {
		t.Fatalf("ShowGrants (after cycle 2): %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("expected 1 grant after cycle 2, got %d", len(rows))
	} else if rows[0].GranteeName != "ROLE_A" {
		t.Errorf("expected remaining grant for ROLE_A, got %q", rows[0].GranteeName)
	}

	// Idempotent grant: granting the same privilege again should not duplicate.
	if err := client.ExecuteGrant(ctx, testDB, testSchema, spec.Name, "ROLE", "ROLE_A", "USAGE"); err != nil {
		t.Fatalf("ExecuteGrant ROLE_A (repeat): %v", err)
	}
	rows, err = client.ShowGrants(ctx, testDB, testSchema, spec.Name)
	if err != nil {
		t.Fatalf("ShowGrants (idempotent): %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("expected 1 grant after idempotent re-grant, got %d", len(rows))
	}
}

// TestGrants_PreSeeded verifies that SetGrants primes the server correctly and
// ShowGrants returns the expected rows.
func TestGrants_PreSeeded(t *testing.T) {
	ms := regression.NewMockServer(t)
	client := newTestClient(t, ms)
	ctx := context.Background()

	ms.SetGrants("seeded", []string{
		"USAGE:ROLE:ROLE_X",
		"USAGE:ROLE:ROLE_Y",
	})

	spec := agent.AgentSpec{Name: "seeded"}
	if err := client.CreateAgent(ctx, testDB, testSchema, spec); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	rows, err := client.ShowGrants(ctx, testDB, testSchema, spec.Name)
	if err != nil {
		t.Fatalf("ShowGrants: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 pre-seeded grants, got %d", len(rows))
	}
}
