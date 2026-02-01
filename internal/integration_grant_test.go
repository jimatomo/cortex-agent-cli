//go:build integration

package internal

import (
	"context"
	"testing"

	"coragent/internal/agent"
	"coragent/internal/api"
	"coragent/internal/grant"
)

// TestE2E_GrantAccountRole tests granting privileges to an account role.
func TestE2E_GrantAccountRole(t *testing.T) {
	cfg, db, schema := skipIfNotConfigured(t)

	client, err := api.NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	agentName := uniqueName("E2E_GRANT_ACCT")

	t.Cleanup(func() {
		_ = client.DeleteAgent(ctx, db, schema, agentName)
	})

	// Create agent
	spec := agent.AgentSpec{
		Name:    agentName,
		Comment: "Grant account role test",
	}
	if err := client.CreateAgent(ctx, db, schema, spec); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// Grant USAGE to ACCOUNTADMIN (a role that always exists)
	if err := client.ExecuteGrant(ctx, db, schema, agentName, "ROLE", "ACCOUNTADMIN", "USAGE"); err != nil {
		t.Fatalf("ExecuteGrant: %v", err)
	}

	// Verify grant exists via ShowGrants
	rows, err := client.ShowGrants(ctx, db, schema, agentName)
	if err != nil {
		t.Fatalf("ShowGrants: %v", err)
	}

	found := false
	for _, row := range rows {
		if row.Privilege == "USAGE" && row.GrantedTo == "ROLE" && row.GranteeName == "ACCOUNTADMIN" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected to find USAGE grant to ACCOUNTADMIN")
	}
}

// TestE2E_RevokeAccountRole tests revoking privileges from an account role.
func TestE2E_RevokeAccountRole(t *testing.T) {
	cfg, db, schema := skipIfNotConfigured(t)

	client, err := api.NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	agentName := uniqueName("E2E_REVOKE_ACCT")

	t.Cleanup(func() {
		_ = client.DeleteAgent(ctx, db, schema, agentName)
	})

	// Create agent
	spec := agent.AgentSpec{
		Name:    agentName,
		Comment: "Revoke account role test",
	}
	if err := client.CreateAgent(ctx, db, schema, spec); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// Grant then revoke USAGE to ACCOUNTADMIN
	if err := client.ExecuteGrant(ctx, db, schema, agentName, "ROLE", "ACCOUNTADMIN", "USAGE"); err != nil {
		t.Fatalf("ExecuteGrant: %v", err)
	}

	if err := client.ExecuteRevoke(ctx, db, schema, agentName, "ROLE", "ACCOUNTADMIN", "USAGE"); err != nil {
		t.Fatalf("ExecuteRevoke: %v", err)
	}

	// Verify grant is removed
	rows, err := client.ShowGrants(ctx, db, schema, agentName)
	if err != nil {
		t.Fatalf("ShowGrants: %v", err)
	}

	for _, row := range rows {
		if row.Privilege == "USAGE" && row.GrantedTo == "ROLE" && row.GranteeName == "ACCOUNTADMIN" {
			t.Error("Expected USAGE grant to ACCOUNTADMIN to be revoked")
		}
	}
}

// TestE2E_GrantMultiplePrivileges tests granting multiple privileges to a role.
func TestE2E_GrantMultiplePrivileges(t *testing.T) {
	cfg, db, schema := skipIfNotConfigured(t)

	client, err := api.NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	agentName := uniqueName("E2E_GRANT_MULTI")

	t.Cleanup(func() {
		_ = client.DeleteAgent(ctx, db, schema, agentName)
	})

	// Create agent
	spec := agent.AgentSpec{
		Name:    agentName,
		Comment: "Multiple privileges test",
	}
	if err := client.CreateAgent(ctx, db, schema, spec); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// Grant multiple privileges
	privileges := []string{"USAGE", "MONITOR"}
	for _, priv := range privileges {
		if err := client.ExecuteGrant(ctx, db, schema, agentName, "ROLE", "ACCOUNTADMIN", priv); err != nil {
			t.Fatalf("ExecuteGrant %s: %v", priv, err)
		}
	}

	// Verify all grants exist
	rows, err := client.ShowGrants(ctx, db, schema, agentName)
	if err != nil {
		t.Fatalf("ShowGrants: %v", err)
	}

	foundPrivs := make(map[string]bool)
	for _, row := range rows {
		if row.GrantedTo == "ROLE" && row.GranteeName == "ACCOUNTADMIN" {
			foundPrivs[row.Privilege] = true
		}
	}

	for _, priv := range privileges {
		if !foundPrivs[priv] {
			t.Errorf("Expected to find %s grant to ACCOUNTADMIN", priv)
		}
	}
}

// TestE2E_ShowGrants tests the ShowGrants API response parsing.
func TestE2E_ShowGrants(t *testing.T) {
	cfg, db, schema := skipIfNotConfigured(t)

	client, err := api.NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	agentName := uniqueName("E2E_SHOWGRANTS")

	t.Cleanup(func() {
		_ = client.DeleteAgent(ctx, db, schema, agentName)
	})

	// Create agent
	spec := agent.AgentSpec{
		Name:    agentName,
		Comment: "ShowGrants test",
	}
	if err := client.CreateAgent(ctx, db, schema, spec); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// Add a grant
	if err := client.ExecuteGrant(ctx, db, schema, agentName, "ROLE", "ACCOUNTADMIN", "USAGE"); err != nil {
		t.Fatalf("ExecuteGrant: %v", err)
	}

	// Show grants and verify response structure
	rows, err := client.ShowGrants(ctx, db, schema, agentName)
	if err != nil {
		t.Fatalf("ShowGrants: %v", err)
	}

	if len(rows) == 0 {
		t.Fatal("Expected at least one grant row")
	}

	// Verify row structure
	for _, row := range rows {
		if row.Privilege == "" {
			t.Error("Expected non-empty Privilege")
		}
		if row.GrantedTo == "" {
			t.Error("Expected non-empty GrantedTo")
		}
		if row.GranteeName == "" {
			t.Error("Expected non-empty GranteeName")
		}
		t.Logf("Grant: %s ON AGENT TO %s %s", row.Privilege, row.GrantedTo, row.GranteeName)
	}
}

// TestE2E_GrantDiff tests the grant diff computation and application.
func TestE2E_GrantDiff(t *testing.T) {
	cfg, db, schema := skipIfNotConfigured(t)

	client, err := api.NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	agentName := uniqueName("E2E_GRANTDIFF")

	t.Cleanup(func() {
		_ = client.DeleteAgent(ctx, db, schema, agentName)
	})

	// Create agent
	spec := agent.AgentSpec{
		Name:    agentName,
		Comment: "Grant diff test",
	}
	if err := client.CreateAgent(ctx, db, schema, spec); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// Initial state: grant USAGE
	if err := client.ExecuteGrant(ctx, db, schema, agentName, "ROLE", "ACCOUNTADMIN", "USAGE"); err != nil {
		t.Fatalf("ExecuteGrant USAGE: %v", err)
	}

	// Get current state
	rows, err := client.ShowGrants(ctx, db, schema, agentName)
	if err != nil {
		t.Fatalf("ShowGrants: %v", err)
	}
	current := grant.FromShowGrantsRows(toGrantShowRows(rows))

	// Define desired state: remove USAGE, add MONITOR
	desired := grant.GrantState{
		Entries: []grant.GrantEntry{
			{Privilege: "MONITOR", RoleType: "ROLE", RoleName: "ACCOUNTADMIN"},
		},
	}

	// Compute diff
	diff := grant.ComputeDiff(desired, current)

	t.Logf("Diff: %d grants, %d revokes", len(diff.ToGrant), len(diff.ToRevoke))

	// Apply diff
	for _, e := range diff.ToRevoke {
		if err := client.ExecuteRevoke(ctx, db, schema, agentName, e.RoleType, e.RoleName, e.Privilege); err != nil {
			t.Errorf("ExecuteRevoke %s: %v", e.Privilege, err)
		}
	}
	for _, e := range diff.ToGrant {
		if err := client.ExecuteGrant(ctx, db, schema, agentName, e.RoleType, e.RoleName, e.Privilege); err != nil {
			t.Errorf("ExecuteGrant %s: %v", e.Privilege, err)
		}
	}

	// Verify final state
	finalRows, err := client.ShowGrants(ctx, db, schema, agentName)
	if err != nil {
		t.Fatalf("ShowGrants after diff: %v", err)
	}

	hasUsage := false
	hasMonitor := false
	for _, row := range finalRows {
		if row.GrantedTo == "ROLE" && row.GranteeName == "ACCOUNTADMIN" {
			if row.Privilege == "USAGE" {
				hasUsage = true
			}
			if row.Privilege == "MONITOR" {
				hasMonitor = true
			}
		}
	}

	if hasUsage {
		t.Error("Expected USAGE to be revoked")
	}
	if !hasMonitor {
		t.Error("Expected MONITOR to be granted")
	}
}

// TestE2E_GrantIdempotent tests that applying the same grants twice has no effect.
func TestE2E_GrantIdempotent(t *testing.T) {
	cfg, db, schema := skipIfNotConfigured(t)

	client, err := api.NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	agentName := uniqueName("E2E_IDEMPOTENT")

	t.Cleanup(func() {
		_ = client.DeleteAgent(ctx, db, schema, agentName)
	})

	// Create agent
	spec := agent.AgentSpec{
		Name:    agentName,
		Comment: "Idempotent grant test",
	}
	if err := client.CreateAgent(ctx, db, schema, spec); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// Define desired state
	desired := grant.GrantState{
		Entries: []grant.GrantEntry{
			{Privilege: "USAGE", RoleType: "ROLE", RoleName: "ACCOUNTADMIN"},
		},
	}

	// Apply grants first time
	for _, e := range desired.Entries {
		if err := client.ExecuteGrant(ctx, db, schema, agentName, e.RoleType, e.RoleName, e.Privilege); err != nil {
			t.Fatalf("ExecuteGrant (first): %v", err)
		}
	}

	// Get current state
	rows, err := client.ShowGrants(ctx, db, schema, agentName)
	if err != nil {
		t.Fatalf("ShowGrants: %v", err)
	}
	current := grant.FromShowGrantsRows(toGrantShowRows(rows))

	// Compute diff - should have no changes
	diff := grant.ComputeDiff(desired, current)

	if diff.HasChanges() {
		t.Errorf("Expected no changes on second apply, got %d grants and %d revokes",
			len(diff.ToGrant), len(diff.ToRevoke))
	}
}

// TestE2E_GrantInvalidRole tests error handling for invalid roles.
func TestE2E_GrantInvalidRole(t *testing.T) {
	cfg, db, schema := skipIfNotConfigured(t)

	client, err := api.NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	agentName := uniqueName("E2E_INVALIDROLE")

	t.Cleanup(func() {
		_ = client.DeleteAgent(ctx, db, schema, agentName)
	})

	// Create agent
	spec := agent.AgentSpec{
		Name:    agentName,
		Comment: "Invalid role test",
	}
	if err := client.CreateAgent(ctx, db, schema, spec); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// Try to grant to a non-existent role - should fail
	err = client.ExecuteGrant(ctx, db, schema, agentName, "ROLE", "NONEXISTENT_ROLE_12345", "USAGE")
	if err == nil {
		t.Error("Expected error when granting to non-existent role")
	} else {
		t.Logf("Got expected error: %v", err)
	}
}

// toGrantShowRows converts api.ShowGrantsRow to grant.ShowGrantsRow
func toGrantShowRows(rows []api.ShowGrantsRow) []grant.ShowGrantsRow {
	result := make([]grant.ShowGrantsRow, len(rows))
	for i, r := range rows {
		result[i] = grant.ShowGrantsRow{
			Privilege:   r.Privilege,
			GrantedTo:   r.GrantedTo,
			GranteeName: r.GranteeName,
		}
	}
	return result
}
