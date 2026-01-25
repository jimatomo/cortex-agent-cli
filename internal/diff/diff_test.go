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
