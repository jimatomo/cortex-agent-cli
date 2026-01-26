//go:build integration

package api

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"coragent/internal/agent"
	"coragent/internal/auth"
)

// testConfig returns auth.Config populated from environment variables.
// Skips the test if required environment variables are not set or authentication is not properly configured.
func testConfig(t *testing.T) auth.Config {
	t.Helper()
	cfg := auth.FromEnv()
	if cfg.Account == "" {
		t.Skip("SNOWFLAKE_ACCOUNT not set; skipping integration test")
	}

	ctx := context.Background()

	// Validate authentication is properly configured
	authType := strings.ToUpper(strings.TrimSpace(cfg.Authenticator))
	if authType == "" || authType == auth.AuthenticatorKeyPair {
		if cfg.User == "" || cfg.PrivateKey == "" {
			t.Skip("Key pair authentication not fully configured; skipping integration test")
		}
	} else if authType == auth.AuthenticatorWorkloadIdentity {
		provider := strings.ToUpper(cfg.WorkloadIdentityProvider)
		if provider == "AWS" {
			// For AWS WIF, check if AWS credentials are available
			if !auth.IsAWSEnvironment(ctx) {
				t.Skip("AWS WIF configured but no AWS credentials available; skipping integration test")
			}
		} else if cfg.OAuthToken == "" {
			t.Skip("WIF authentication configured but no OAuth token available; skipping integration test")
		}
	}

	return cfg
}

// testDatabase returns the database name from SNOWFLAKE_DATABASE or skips the test.
func testDatabase(t *testing.T) string {
	t.Helper()
	db := os.Getenv("SNOWFLAKE_DATABASE")
	if db == "" {
		t.Skip("SNOWFLAKE_DATABASE not set; skipping integration test")
	}
	return db
}

// testSchema returns the schema name from SNOWFLAKE_SCHEMA or skips the test.
func testSchema(t *testing.T) string {
	t.Helper()
	schema := os.Getenv("SNOWFLAKE_SCHEMA")
	if schema == "" {
		t.Skip("SNOWFLAKE_SCHEMA not set; skipping integration test")
	}
	return schema
}

// uniqueAgentName generates a unique agent name for testing to avoid conflicts.
func uniqueAgentName(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

func TestIntegration_CreateAndDeleteAgent(t *testing.T) {
	cfg := testConfig(t)
	db := testDatabase(t)
	schema := testSchema(t)

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	agentName := uniqueAgentName("TEST_AGENT_CREATE")

	spec := agent.AgentSpec{
		Name:    agentName,
		Comment: "Integration test agent",
		Profile: &agent.Profile{
			DisplayName: "Test Agent",
		},
		Models: &agent.Models{
			Orchestration: "claude-3-5-sonnet",
		},
	}

	// Cleanup: ensure the agent is deleted at the end of the test
	t.Cleanup(func() {
		_ = client.DeleteAgent(ctx, db, schema, agentName)
	})

	// Create agent
	if err := client.CreateAgent(ctx, db, schema, spec); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// Verify agent was created by fetching it
	retrieved, exists, err := client.GetAgent(ctx, db, schema, agentName)
	if err != nil {
		t.Fatalf("GetAgent after create: %v", err)
	}
	if !exists {
		t.Fatal("Agent was created but GetAgent returned exists=false")
	}
	if retrieved.Name != agentName {
		t.Errorf("Agent name mismatch: got %q, want %q", retrieved.Name, agentName)
	}
	if retrieved.Comment != spec.Comment {
		t.Errorf("Agent comment mismatch: got %q, want %q", retrieved.Comment, spec.Comment)
	}

	// Delete agent
	if err := client.DeleteAgent(ctx, db, schema, agentName); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}

	// Verify agent no longer exists
	_, exists, err = client.GetAgent(ctx, db, schema, agentName)
	if err != nil {
		t.Fatalf("GetAgent after delete: %v", err)
	}
	if exists {
		t.Error("Agent still exists after delete")
	}
}

func TestIntegration_UpdateAgent(t *testing.T) {
	cfg := testConfig(t)
	db := testDatabase(t)
	schema := testSchema(t)

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	agentName := uniqueAgentName("TEST_AGENT_UPDATE")

	initialSpec := agent.AgentSpec{
		Name:    agentName,
		Comment: "Initial comment",
		Profile: &agent.Profile{
			DisplayName: "Initial Display Name",
		},
	}

	t.Cleanup(func() {
		_ = client.DeleteAgent(ctx, db, schema, agentName)
	})

	// Create agent
	if err := client.CreateAgent(ctx, db, schema, initialSpec); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// Update agent with new values
	updatePayload := map[string]any{
		"comment": "Updated comment",
		"profile": map[string]any{
			"display_name": "Updated Display Name",
		},
	}
	if err := client.UpdateAgent(ctx, db, schema, agentName, updatePayload); err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}

	// Verify the update was applied
	updated, exists, err := client.GetAgent(ctx, db, schema, agentName)
	if err != nil {
		t.Fatalf("GetAgent after update: %v", err)
	}
	if !exists {
		t.Fatal("Agent does not exist after update")
	}
	if updated.Comment != "Updated comment" {
		t.Errorf("Agent comment not updated: got %q, want %q", updated.Comment, "Updated comment")
	}
	if updated.Profile == nil || updated.Profile.DisplayName != "Updated Display Name" {
		displayName := ""
		if updated.Profile != nil {
			displayName = updated.Profile.DisplayName
		}
		t.Errorf("Agent display name not updated: got %q, want %q", displayName, "Updated Display Name")
	}
}

func TestIntegration_GetNonExistentAgent(t *testing.T) {
	cfg := testConfig(t)
	db := testDatabase(t)
	schema := testSchema(t)

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	nonExistentName := uniqueAgentName("NON_EXISTENT_AGENT")

	_, exists, err := client.GetAgent(ctx, db, schema, nonExistentName)
	if err != nil {
		t.Fatalf("GetAgent for non-existent agent should not error: %v", err)
	}
	if exists {
		t.Error("GetAgent returned exists=true for non-existent agent")
	}
}

func TestIntegration_DeleteNonExistentAgent(t *testing.T) {
	cfg := testConfig(t)
	db := testDatabase(t)
	schema := testSchema(t)

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	nonExistentName := uniqueAgentName("NON_EXISTENT_AGENT")

	err = client.DeleteAgent(ctx, db, schema, nonExistentName)
	// Deleting a non-existent agent should return an error (404 or similar)
	if err == nil {
		t.Log("DeleteAgent for non-existent agent returned nil error (may be expected behavior)")
	} else {
		// Verify it's a not-found type error
		if !isNotFoundError(err) {
			t.Logf("DeleteAgent returned error (not a not-found error): %v", err)
		}
	}
}

func TestIntegration_CreateDuplicateAgent(t *testing.T) {
	cfg := testConfig(t)
	db := testDatabase(t)
	schema := testSchema(t)

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	agentName := uniqueAgentName("TEST_AGENT_DUPLICATE")

	spec := agent.AgentSpec{
		Name:    agentName,
		Comment: "First agent",
	}

	t.Cleanup(func() {
		_ = client.DeleteAgent(ctx, db, schema, agentName)
	})

	// Create first agent
	if err := client.CreateAgent(ctx, db, schema, spec); err != nil {
		t.Fatalf("First CreateAgent: %v", err)
	}

	// Attempt to create duplicate agent
	err = client.CreateAgent(ctx, db, schema, spec)
	if err == nil {
		t.Error("CreateAgent for duplicate agent should return an error")
	} else {
		t.Logf("CreateAgent for duplicate returned expected error: %v", err)
	}
}

func TestIntegration_AgentWithTools(t *testing.T) {
	cfg := testConfig(t)
	db := testDatabase(t)
	schema := testSchema(t)

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	agentName := uniqueAgentName("TEST_AGENT_TOOLS")

	spec := agent.AgentSpec{
		Name:    agentName,
		Comment: "Agent with tools",
		Profile: &agent.Profile{
			DisplayName: "Tool Test Agent",
		},
		Models: &agent.Models{
			Orchestration: "claude-3-5-sonnet",
		},
		Instructions: &agent.Instructions{
			System: "You are a helpful assistant.",
		},
		Orchestration: &agent.Orchestration{
			Budget: &agent.BudgetConfig{
				Seconds: 60,
				Tokens:  4096,
			},
		},
		Tools: []agent.Tool{
			{
				ToolSpec: map[string]any{
					"type": "code_execution",
				},
			},
		},
	}

	t.Cleanup(func() {
		_ = client.DeleteAgent(ctx, db, schema, agentName)
	})

	// Create agent with tools
	if err := client.CreateAgent(ctx, db, schema, spec); err != nil {
		t.Fatalf("CreateAgent with tools: %v", err)
	}

	// Verify agent was created with correct configuration
	retrieved, exists, err := client.GetAgent(ctx, db, schema, agentName)
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if !exists {
		t.Fatal("Agent does not exist after create")
	}

	// Check tools were saved
	if len(retrieved.Tools) == 0 {
		t.Error("Agent tools not saved; expected at least one tool")
	}

	// Check orchestration budget
	if retrieved.Orchestration == nil || retrieved.Orchestration.Budget == nil {
		t.Error("Agent orchestration budget not saved")
	} else {
		if retrieved.Orchestration.Budget.Seconds != 60 {
			t.Errorf("Budget seconds mismatch: got %d, want %d", retrieved.Orchestration.Budget.Seconds, 60)
		}
	}
}

func TestIntegration_AgentWithInstructions(t *testing.T) {
	cfg := testConfig(t)
	db := testDatabase(t)
	schema := testSchema(t)

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	agentName := uniqueAgentName("TEST_AGENT_INSTRUCTIONS")

	spec := agent.AgentSpec{
		Name:    agentName,
		Comment: "Agent with instructions",
		Instructions: &agent.Instructions{
			System:        "You are a helpful assistant specializing in data analysis.",
			Orchestration: "Think step by step before answering.",
			Response:      "Be concise and accurate.",
			SampleQuestions: []agent.SampleQuestion{
				{Question: "How do I analyze sales data?"},
				{Question: "What is a pivot table?"},
			},
		},
	}

	t.Cleanup(func() {
		_ = client.DeleteAgent(ctx, db, schema, agentName)
	})

	if err := client.CreateAgent(ctx, db, schema, spec); err != nil {
		t.Fatalf("CreateAgent with instructions: %v", err)
	}

	retrieved, exists, err := client.GetAgent(ctx, db, schema, agentName)
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if !exists {
		t.Fatal("Agent does not exist")
	}

	if retrieved.Instructions == nil {
		t.Fatal("Agent instructions not saved")
	}
	if retrieved.Instructions.System != spec.Instructions.System {
		t.Errorf("System instruction mismatch: got %q, want %q", retrieved.Instructions.System, spec.Instructions.System)
	}
}
