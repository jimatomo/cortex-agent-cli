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

func TestLoadAgentWithGrant(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	err := os.WriteFile(path, []byte(`
name: test-agent
deploy:
  database: TEST_DB
  schema: PUBLIC
  grant:
    account_roles:
      - role: ANALYST_ROLE
        privileges:
          - USAGE
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
	if agents[0].Spec.Deploy == nil {
		t.Fatal("expected Deploy to be non-nil")
	}
	if agents[0].Spec.Deploy.Grant == nil {
		t.Fatal("expected Grant to be non-nil")
	}
	if len(agents[0].Spec.Deploy.Grant.AccountRoles) != 1 {
		t.Fatalf("expected 1 account role, got %d", len(agents[0].Spec.Deploy.Grant.AccountRoles))
	}
	if agents[0].Spec.Deploy.Grant.AccountRoles[0].Role != "ANALYST_ROLE" {
		t.Errorf("expected role ANALYST_ROLE, got %s", agents[0].Spec.Deploy.Grant.AccountRoles[0].Role)
	}
}

func TestLoadAgentWithMultiplePrivileges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	err := os.WriteFile(path, []byte(`
name: test-agent
deploy:
  grant:
    account_roles:
      - role: ADMIN_ROLE
        privileges:
          - USAGE
          - MODIFY
          - MONITOR
`), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	agents, err := LoadAgents(path, false)
	if err != nil {
		t.Fatalf("LoadAgents error: %v", err)
	}
	if len(agents[0].Spec.Deploy.Grant.AccountRoles[0].Privileges) != 3 {
		t.Fatalf("expected 3 privileges, got %d", len(agents[0].Spec.Deploy.Grant.AccountRoles[0].Privileges))
	}
}

func TestLoadAgentRejectsInvalidPrivilege(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	err := os.WriteFile(path, []byte(`
name: test-agent
deploy:
  grant:
    account_roles:
      - role: ANALYST_ROLE
        privileges:
          - INVALID_PRIV
`), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err = LoadAgents(path, false)
	if err == nil {
		t.Fatal("expected error for invalid privilege, got nil")
	}
}

func TestLoadAgentRejectsUnqualifiedDatabaseRole(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	err := os.WriteFile(path, []byte(`
name: test-agent
deploy:
  grant:
    database_roles:
      - role: UNQUALIFIED_ROLE
        privileges:
          - USAGE
`), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err = LoadAgents(path, false)
	if err == nil {
		t.Fatal("expected error for unqualified database role, got nil")
	}
}

func TestLoadAgentRejectsEmptyRole(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	err := os.WriteFile(path, []byte(`
name: test-agent
deploy:
  grant:
    account_roles:
      - role: ""
        privileges:
          - USAGE
`), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err = LoadAgents(path, false)
	if err == nil {
		t.Fatal("expected error for empty role, got nil")
	}
}

func TestLoadAgentRejectsEmptyPrivileges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	err := os.WriteFile(path, []byte(`
name: test-agent
deploy:
  grant:
    account_roles:
      - role: ANALYST_ROLE
        privileges: []
`), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err = LoadAgents(path, false)
	if err == nil {
		t.Fatal("expected error for empty privileges, got nil")
	}
}

func TestLoadAgentWithDatabaseRole(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	err := os.WriteFile(path, []byte(`
name: test-agent
deploy:
  grant:
    database_roles:
      - role: TEST_DB.DATA_READER
        privileges:
          - USAGE
          - MONITOR
`), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	agents, err := LoadAgents(path, false)
	if err != nil {
		t.Fatalf("LoadAgents error: %v", err)
	}
	if len(agents[0].Spec.Deploy.Grant.DatabaseRoles) != 1 {
		t.Fatalf("expected 1 database role, got %d", len(agents[0].Spec.Deploy.Grant.DatabaseRoles))
	}
	if agents[0].Spec.Deploy.Grant.DatabaseRoles[0].Role != "TEST_DB.DATA_READER" {
		t.Errorf("expected role TEST_DB.DATA_READER, got %s", agents[0].Spec.Deploy.Grant.DatabaseRoles[0].Role)
	}
}

func TestLoadAgentWithAllPrivilege(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	err := os.WriteFile(path, []byte(`
name: test-agent
deploy:
  grant:
    account_roles:
      - role: ADMIN_ROLE
        privileges:
          - ALL
`), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	agents, err := LoadAgents(path, false)
	if err != nil {
		t.Fatalf("LoadAgents error: %v", err)
	}
	if agents[0].Spec.Deploy.Grant.AccountRoles[0].Privileges[0] != "ALL" {
		t.Errorf("expected privilege ALL, got %s", agents[0].Spec.Deploy.Grant.AccountRoles[0].Privileges[0])
	}
}
