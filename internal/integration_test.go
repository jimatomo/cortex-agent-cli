//go:build integration

// Package internal provides end-to-end integration tests that exercise
// the full workflow from authentication through API operations.
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"coragent/internal/agent"
	"coragent/internal/api"
	"coragent/internal/auth"
	"coragent/internal/diff"
)

// skipIfNotConfigured skips the test if required environment variables are not set.
func skipIfNotConfigured(t *testing.T) (auth.Config, string, string) {
	t.Helper()

	cfg := auth.FromEnv()
	if cfg.Account == "" {
		t.Skip("SNOWFLAKE_ACCOUNT not set; skipping integration test")
	}

	db := os.Getenv("SNOWFLAKE_DATABASE")
	if db == "" {
		t.Skip("SNOWFLAKE_DATABASE not set; skipping integration test")
	}

	schema := os.Getenv("SNOWFLAKE_SCHEMA")
	if schema == "" {
		t.Skip("SNOWFLAKE_SCHEMA not set; skipping integration test")
	}

	// Validate auth configuration
	authType := strings.ToUpper(strings.TrimSpace(cfg.Authenticator))
	if authType == "" || authType == auth.AuthenticatorKeyPair {
		if cfg.User == "" || cfg.PrivateKey == "" {
			t.Skip("Key pair authentication not fully configured; skipping test")
		}
	} else if authType == auth.AuthenticatorWorkloadIdentity {
		if cfg.OAuthToken == "" {
			t.Skip("WIF authentication configured but no OAuth token; skipping test")
		}
	}

	return cfg, db, schema
}

// uniqueName generates a unique name for testing.
func uniqueName(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

// TestE2E_FullAgentLifecycle tests the complete agent lifecycle:
// Create -> Read -> Update -> Read -> Delete -> Verify Deleted
func TestE2E_FullAgentLifecycle(t *testing.T) {
	cfg, db, schema := skipIfNotConfigured(t)

	client, err := api.NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	agentName := uniqueName("E2E_LIFECYCLE")

	t.Cleanup(func() {
		// Best-effort cleanup
		_ = client.DeleteAgent(ctx, db, schema, agentName)
	})

	// Step 1: Create agent
	t.Log("Step 1: Creating agent...")
	createSpec := agent.AgentSpec{
		Name:    agentName,
		Comment: "E2E test agent - initial",
		Profile: &agent.Profile{
			DisplayName: "E2E Test Agent",
		},
		Models: &agent.Models{
			Orchestration: "claude-3-5-sonnet",
		},
		Instructions: &agent.Instructions{
			System: "You are a helpful test assistant.",
		},
	}

	if err := client.CreateAgent(ctx, db, schema, createSpec); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	t.Log("  Agent created successfully")

	// Step 2: Read and verify initial state
	t.Log("Step 2: Reading agent to verify creation...")
	readSpec, exists, err := client.GetAgent(ctx, db, schema, agentName)
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if !exists {
		t.Fatal("Agent should exist after creation")
	}
	if readSpec.Name != agentName {
		t.Errorf("Name mismatch: got %q, want %q", readSpec.Name, agentName)
	}
	if readSpec.Comment != createSpec.Comment {
		t.Errorf("Comment mismatch: got %q, want %q", readSpec.Comment, createSpec.Comment)
	}
	t.Log("  Agent verified successfully")

	// Step 3: Update agent
	t.Log("Step 3: Updating agent...")
	updatePayload := map[string]any{
		"comment": "E2E test agent - updated",
		"profile": map[string]any{
			"display_name": "E2E Test Agent (Updated)",
		},
		"instructions": map[string]any{
			"system": "You are an updated test assistant.",
		},
	}
	if err := client.UpdateAgent(ctx, db, schema, agentName, updatePayload); err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	t.Log("  Agent updated successfully")

	// Step 4: Read and verify update
	t.Log("Step 4: Reading agent to verify update...")
	updatedSpec, exists, err := client.GetAgent(ctx, db, schema, agentName)
	if err != nil {
		t.Fatalf("GetAgent after update: %v", err)
	}
	if !exists {
		t.Fatal("Agent should exist after update")
	}
	if updatedSpec.Comment != "E2E test agent - updated" {
		t.Errorf("Updated comment mismatch: got %q", updatedSpec.Comment)
	}
	if updatedSpec.Profile == nil || updatedSpec.Profile.DisplayName != "E2E Test Agent (Updated)" {
		t.Error("Updated profile.display_name not applied")
	}
	t.Log("  Update verified successfully")

	// Step 5: Delete agent
	t.Log("Step 5: Deleting agent...")
	if err := client.DeleteAgent(ctx, db, schema, agentName); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}
	t.Log("  Agent deleted successfully")

	// Step 6: Verify deletion
	t.Log("Step 6: Verifying agent is deleted...")
	_, exists, err = client.GetAgent(ctx, db, schema, agentName)
	if err != nil {
		t.Fatalf("GetAgent after delete: %v", err)
	}
	if exists {
		t.Error("Agent should not exist after deletion")
	}
	t.Log("  Deletion verified successfully")

	t.Log("E2E lifecycle test completed successfully!")
}

// TestE2E_DiffAndUpdate tests the diff detection and selective update workflow.
func TestE2E_DiffAndUpdate(t *testing.T) {
	cfg, db, schema := skipIfNotConfigured(t)

	client, err := api.NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	agentName := uniqueName("E2E_DIFF")

	t.Cleanup(func() {
		_ = client.DeleteAgent(ctx, db, schema, agentName)
	})

	// Create initial agent
	initialSpec := agent.AgentSpec{
		Name:    agentName,
		Comment: "Initial comment",
		Profile: &agent.Profile{
			DisplayName: "Initial Name",
		},
	}

	if err := client.CreateAgent(ctx, db, schema, initialSpec); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// Get current state from server
	remoteSpec, exists, err := client.GetAgent(ctx, db, schema, agentName)
	if err != nil || !exists {
		t.Fatalf("GetAgent: err=%v, exists=%v", err, exists)
	}

	// Define desired state with changes
	desiredSpec := agent.AgentSpec{
		Name:    agentName,
		Comment: "Updated comment via diff",
		Profile: &agent.Profile{
			DisplayName: "Updated Name via diff",
		},
	}

	// Compute diff (local=desired, remote=current)
	changes, err := diff.Diff(desiredSpec, remoteSpec)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("Expected changes between specs but got none")
	}

	t.Logf("Detected %d changes:", len(changes))
	for _, c := range changes {
		t.Logf("  %s: %s -> %s (%s)", c.Path, formatValue(c.After), formatValue(c.Before), c.Type)
	}

	// Build update payload from changes
	updatePayload, err := buildUpdatePayload(desiredSpec, changes)
	if err != nil {
		t.Fatalf("buildUpdatePayload: %v", err)
	}
	if len(updatePayload) == 0 {
		t.Fatal("buildUpdatePayload returned empty payload")
	}

	// Apply update
	if err := client.UpdateAgent(ctx, db, schema, agentName, updatePayload); err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}

	// Verify update was applied
	finalSpec, _, err := client.GetAgent(ctx, db, schema, agentName)
	if err != nil {
		t.Fatalf("GetAgent after update: %v", err)
	}

	if finalSpec.Comment != desiredSpec.Comment {
		t.Errorf("Comment not updated: got %q, want %q", finalSpec.Comment, desiredSpec.Comment)
	}
	if finalSpec.Profile == nil || finalSpec.Profile.DisplayName != desiredSpec.Profile.DisplayName {
		t.Error("Profile.DisplayName not updated")
	}

	// Verify no more diff
	finalChanges, err := diff.Diff(finalSpec, desiredSpec)
	if err != nil {
		t.Fatalf("Diff after update: %v", err)
	}
	if len(finalChanges) > 0 {
		t.Errorf("Expected no changes after update, got %d:", len(finalChanges))
		for _, c := range finalChanges {
			t.Errorf("  %s: %v -> %v", c.Path, c.After, c.Before)
		}
	}
}

// TestE2E_MultipleAgents tests creating and managing multiple agents concurrently.
func TestE2E_MultipleAgents(t *testing.T) {
	cfg, db, schema := skipIfNotConfigured(t)

	client, err := api.NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	baseTime := time.Now().UnixNano()
	agentNames := []string{
		fmt.Sprintf("E2E_MULTI_A_%d", baseTime),
		fmt.Sprintf("E2E_MULTI_B_%d", baseTime),
		fmt.Sprintf("E2E_MULTI_C_%d", baseTime),
	}

	// Cleanup all agents at the end
	t.Cleanup(func() {
		for _, name := range agentNames {
			_ = client.DeleteAgent(ctx, db, schema, name)
		}
	})

	// Create multiple agents
	for i, name := range agentNames {
		spec := agent.AgentSpec{
			Name:    name,
			Comment: fmt.Sprintf("Multi-agent test %d", i+1),
			Profile: &agent.Profile{
				DisplayName: fmt.Sprintf("Agent %d", i+1),
			},
		}
		if err := client.CreateAgent(ctx, db, schema, spec); err != nil {
			t.Fatalf("CreateAgent %s: %v", name, err)
		}
		t.Logf("Created agent: %s", name)
	}

	// Verify all agents exist
	for _, name := range agentNames {
		_, exists, err := client.GetAgent(ctx, db, schema, name)
		if err != nil {
			t.Errorf("GetAgent %s: %v", name, err)
		}
		if !exists {
			t.Errorf("Agent %s should exist", name)
		}
	}

	// Delete all agents
	for _, name := range agentNames {
		if err := client.DeleteAgent(ctx, db, schema, name); err != nil {
			t.Errorf("DeleteAgent %s: %v", name, err)
		}
		t.Logf("Deleted agent: %s", name)
	}

	// Verify all agents are deleted
	for _, name := range agentNames {
		_, exists, err := client.GetAgent(ctx, db, schema, name)
		if err != nil {
			t.Errorf("GetAgent %s after delete: %v", name, err)
		}
		if exists {
			t.Errorf("Agent %s should not exist after delete", name)
		}
	}
}

// TestE2E_AuthenticationReuse tests that authentication works correctly across multiple API calls.
func TestE2E_AuthenticationReuse(t *testing.T) {
	cfg, db, schema := skipIfNotConfigured(t)

	client, err := api.NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	agentName := uniqueName("E2E_AUTH_REUSE")

	t.Cleanup(func() {
		_ = client.DeleteAgent(ctx, db, schema, agentName)
	})

	spec := agent.AgentSpec{
		Name:    agentName,
		Comment: "Auth reuse test",
	}

	// Perform multiple operations with the same client to test auth reuse
	operations := []struct {
		name string
		fn   func() error
	}{
		{"Create", func() error { return client.CreateAgent(ctx, db, schema, spec) }},
		{"Get1", func() error {
			_, _, err := client.GetAgent(ctx, db, schema, agentName)
			return err
		}},
		{"Update", func() error {
			return client.UpdateAgent(ctx, db, schema, agentName, map[string]any{"comment": "Updated"})
		}},
		{"Get2", func() error {
			_, _, err := client.GetAgent(ctx, db, schema, agentName)
			return err
		}},
		{"Delete", func() error { return client.DeleteAgent(ctx, db, schema, agentName) }},
		{"GetAfterDelete", func() error {
			_, exists, err := client.GetAgent(ctx, db, schema, agentName)
			if err == nil && exists {
				return fmt.Errorf("agent should not exist")
			}
			return nil
		}},
	}

	for _, op := range operations {
		t.Run(op.name, func(t *testing.T) {
			if err := op.fn(); err != nil {
				t.Errorf("Operation %s failed: %v", op.name, err)
			}
		})
	}
}

func formatValue(v any) string {
	if v == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%v", v)
}

// buildUpdatePayload creates a map containing only the changed top-level fields.
// This mirrors the logic in internal/cli/apply.go's updatePayload function.
func buildUpdatePayload(spec agent.AgentSpec, changes []diff.Change) (map[string]any, error) {
	data, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("marshal spec: %w", err)
	}
	var local map[string]any
	if err := json.Unmarshal(data, &local); err != nil {
		return nil, fmt.Errorf("unmarshal spec: %w", err)
	}

	payload := make(map[string]any)
	for _, change := range changes {
		key := topLevelKey(change.Path)
		if key == "" {
			continue
		}
		if val, ok := local[key]; ok {
			payload[key] = val
		} else {
			payload[key] = nil
		}
	}
	return payload, nil
}

// topLevelKey extracts the top-level key from a change path.
func topLevelKey(path string) string {
	if path == "" {
		return ""
	}
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return ""
	}
	// Handle array notation like "tools[0]"
	return strings.Split(parts[0], "[")[0]
}
