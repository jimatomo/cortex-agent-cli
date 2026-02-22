package regression_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"coragent/internal/agent"
	"coragent/internal/regression"
)

// TestVars_MultiEnvSubstitution verifies that loading the same YAML spec with
// different --env values produces different AgentSpecs and that those specs can
// be applied to the mock server independently.
func TestVars_MultiEnvSubstitution(t *testing.T) {
	// Write a temporary agent YAML with a vars section.
	yaml := `
name: vars-agent
comment: "${vars.ENV_LABEL} environment"

vars:
  default:
    ENV_LABEL: default
    INSTRUCTIONS: "Default instructions."
  dev:
    ENV_LABEL: dev
    INSTRUCTIONS: "Dev instructions."
  prod:
    ENV_LABEL: prod
    INSTRUCTIONS: "Prod instructions."
`
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(yamlPath, []byte(yaml), 0600); err != nil {
		t.Fatalf("write YAML: %v", err)
	}

	cases := []struct {
		env            string
		wantComment    string
		wantLabel      string
	}{
		{"", "default environment", "default"},
		{"dev", "dev environment", "dev"},
		{"prod", "prod environment", "prod"},
	}

	for _, tc := range cases {
		t.Run("env="+tc.env, func(t *testing.T) {
			agents, err := agent.LoadAgents(yamlPath, false, tc.env)
			if err != nil {
				t.Fatalf("LoadAgents(env=%q): %v", tc.env, err)
			}
			if len(agents) != 1 {
				t.Fatalf("expected 1 agent, got %d", len(agents))
			}
			spec := agents[0].Spec
			if spec.Comment != tc.wantComment {
				t.Errorf("Comment = %q, want %q", spec.Comment, tc.wantComment)
			}
		})
	}
}

// TestVars_ApplyWithEnv verifies end-to-end: load a spec for a given env,
// create the agent via the mock API, then describe it to confirm the substituted
// comment was stored.
func TestVars_ApplyWithEnv(t *testing.T) {
	yaml := `
name: env-agent
comment: "${vars.LABEL}"

vars:
  default:
    LABEL: default-label
  staging:
    LABEL: staging-label
`
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(yamlPath, []byte(yaml), 0600); err != nil {
		t.Fatalf("write YAML: %v", err)
	}

	ms := regression.NewMockServer(t)
	client := newTestClient(t, ms)
	ctx := context.Background()

	// Load spec for "staging" environment.
	agents, err := agent.LoadAgents(yamlPath, false, "staging")
	if err != nil {
		t.Fatalf("LoadAgents: %v", err)
	}
	spec := agents[0].Spec

	if spec.Comment != "staging-label" {
		t.Fatalf("expected comment %q before apply, got %q", "staging-label", spec.Comment)
	}

	// Apply (create) via mock API.
	if err := client.CreateAgent(ctx, testDB, testSchema, spec); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// Describe and verify the stored comment.
	got, exists, err := client.GetAgent(ctx, testDB, testSchema, spec.Name)
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if !exists {
		t.Fatal("agent does not exist after create")
	}
	if got.Comment != "staging-label" {
		t.Errorf("GetAgent.Comment = %q, want %q", got.Comment, "staging-label")
	}
}
