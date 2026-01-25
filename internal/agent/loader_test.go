package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAgentsFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	err := os.WriteFile(path, []byte(`
name: test-agent
comment: hello
models:
  orchestration: claude-4-sonnet
`), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	agents, err := LoadAgents(path)
	if err != nil {
		t.Fatalf("LoadAgents error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].Spec.Name != "test-agent" {
		t.Fatalf("unexpected name: %s", agents[0].Spec.Name)
	}
}

func TestLoadAgentsFromDir(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("name: a"), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}
	err = os.WriteFile(filepath.Join(dir, "b.yml"), []byte("name: b"), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	agents, err := LoadAgents(dir)
	if err != nil {
		t.Fatalf("LoadAgents error: %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}
}

func TestLoadAgentsRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	err := os.WriteFile(path, []byte("name: test\nunknown: value\n"), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err = LoadAgents(path)
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}
