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

	agents, err := LoadAgents(path, false)
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

	agents, err := LoadAgents(dir, false)
	if err != nil {
		t.Fatalf("LoadAgents error: %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}
}

func TestLoadAgentsFromDirNonRecursive(t *testing.T) {
	dir := t.TempDir()
	// Create file in top-level directory
	err := os.WriteFile(filepath.Join(dir, "top.yaml"), []byte("name: top"), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}
	// Create subdirectory with a file
	subdir := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("create subdir: %v", err)
	}
	err = os.WriteFile(filepath.Join(subdir, "nested.yaml"), []byte("name: nested"), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Non-recursive should only load top-level file
	agents, err := LoadAgents(dir, false)
	if err != nil {
		t.Fatalf("LoadAgents error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent (non-recursive), got %d", len(agents))
	}
	if agents[0].Spec.Name != "top" {
		t.Fatalf("expected agent name 'top', got %s", agents[0].Spec.Name)
	}
}

func TestLoadAgentsFromDirRecursive(t *testing.T) {
	dir := t.TempDir()
	// Create file in top-level directory
	err := os.WriteFile(filepath.Join(dir, "top.yaml"), []byte("name: top"), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}
	// Create subdirectory with a file
	subdir := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("create subdir: %v", err)
	}
	err = os.WriteFile(filepath.Join(subdir, "nested.yaml"), []byte("name: nested"), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Recursive should load both files
	agents, err := LoadAgents(dir, true)
	if err != nil {
		t.Fatalf("LoadAgents error: %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents (recursive), got %d", len(agents))
	}
	// Files should be sorted, so nested comes before top (subdir/nested.yaml < top.yaml)
	names := []string{agents[0].Spec.Name, agents[1].Spec.Name}
	if names[0] != "nested" || names[1] != "top" {
		t.Fatalf("expected agents [nested, top], got %v", names)
	}
}

func TestLoadAgentsRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	err := os.WriteFile(path, []byte("name: test\nunknown: value\n"), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err = LoadAgents(path, false)
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}
