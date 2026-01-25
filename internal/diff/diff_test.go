package diff

import (
	"testing"

	"coragent/internal/agent"
)

func TestDiffDetectsChanges(t *testing.T) {
	local := agent.AgentSpec{
		Name: "agent",
		Instructions: &agent.Instructions{
			Response: "new",
		},
		Orchestration: &agent.Orchestration{
			Budget: &agent.BudgetConfig{
				Tokens: 4096,
			},
		},
	}
	remote := agent.AgentSpec{
		Name: "agent",
		Instructions: &agent.Instructions{
			Response: "old",
		},
		Orchestration: &agent.Orchestration{
			Budget: &agent.BudgetConfig{
				Tokens: 1024,
			},
		},
	}

	changes, err := Diff(local, remote)
	if err != nil {
		t.Fatalf("Diff error: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected changes, got none")
	}
}

func TestDiffNoChanges(t *testing.T) {
	spec := agent.AgentSpec{
		Name: "agent",
	}
	changes, err := Diff(spec, spec)
	if err != nil {
		t.Fatalf("Diff error: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("expected no changes, got %d", len(changes))
	}
}

func TestDiffForCreate(t *testing.T) {
	spec := agent.AgentSpec{
		Name:    "test-agent",
		Comment: "Test comment",
		Profile: &agent.Profile{DisplayName: "Test Bot"},
		Models:  &agent.Models{Orchestration: "claude-4-sonnet"},
		Orchestration: &agent.Orchestration{
			Budget: &agent.BudgetConfig{Seconds: 60, Tokens: 16000},
		},
	}
	changes, err := DiffForCreate(spec)
	if err != nil {
		t.Fatalf("DiffForCreate error: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected changes, got none")
	}
	// Log all changes for debugging
	for _, c := range changes {
		t.Logf("+ %s: %v", c.Path, c.Before)
	}
	// Should have at least: name, comment, profile.display_name, models.orchestration, orchestration.budget.seconds, orchestration.budget.tokens
	if len(changes) < 6 {
		t.Fatalf("expected at least 6 changes, got %d", len(changes))
	}
}
