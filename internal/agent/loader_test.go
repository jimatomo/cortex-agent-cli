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

	agents, err := LoadAgents(path, false, "")
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

	agents, err := LoadAgents(dir, false, "")
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
	agents, err := LoadAgents(dir, false, "")
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
	agents, err := LoadAgents(dir, true, "")
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

func TestLoadAgentsSkipsDotFiles(t *testing.T) {
	dir := t.TempDir()
	// Valid agent file
	err := os.WriteFile(filepath.Join(dir, "agent.yaml"), []byte("name: my-agent"), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}
	// Dotfile that is not an agent spec (e.g. .goreleaser.yaml)
	err = os.WriteFile(filepath.Join(dir, ".goreleaser.yaml"), []byte("version: 2\nproject_name: foo\n"), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	agents, err := LoadAgents(dir, false, "")
	if err != nil {
		t.Fatalf("LoadAgents error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent (dotfile should be skipped), got %d", len(agents))
	}
	if agents[0].Spec.Name != "my-agent" {
		t.Fatalf("unexpected agent name: %s", agents[0].Spec.Name)
	}
}

func TestLoadAgentsRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	err := os.WriteFile(path, []byte("name: test\nunknown: value\n"), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err = LoadAgents(path, false, "")
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

	agents, err := LoadAgents(path, false, "")
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

	agents, err := LoadAgents(path, false, "")
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

	_, err = LoadAgents(path, false, "")
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

	_, err = LoadAgents(path, false, "")
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

	_, err = LoadAgents(path, false, "")
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

	_, err = LoadAgents(path, false, "")
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

	agents, err := LoadAgents(path, false, "")
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

func TestLoadAgentWithEval(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	err := os.WriteFile(path, []byte(`
name: test-agent
eval:
  tests:
    - question: "売上データを教えて"
      expected_tools:
        - sample_semantic_view
    - question: "ドキュメントを検索して"
      expected_tools:
        - snowflake_docs_service
`), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	agents, err := LoadAgents(path, false, "")
	if err != nil {
		t.Fatalf("LoadAgents error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].Spec.Eval == nil {
		t.Fatal("expected Eval to be non-nil")
	}
	if len(agents[0].Spec.Eval.Tests) != 2 {
		t.Fatalf("expected 2 eval tests, got %d", len(agents[0].Spec.Eval.Tests))
	}
	if agents[0].Spec.Eval.Tests[0].Question != "売上データを教えて" {
		t.Errorf("unexpected question: %s", agents[0].Spec.Eval.Tests[0].Question)
	}
	if len(agents[0].Spec.Eval.Tests[0].ExpectedTools) != 1 {
		t.Fatalf("expected 1 expected tool, got %d", len(agents[0].Spec.Eval.Tests[0].ExpectedTools))
	}
	if agents[0].Spec.Eval.Tests[0].ExpectedTools[0] != "sample_semantic_view" {
		t.Errorf("unexpected expected tool: %s", agents[0].Spec.Eval.Tests[0].ExpectedTools[0])
	}
}

func TestLoadAgentRejectsEvalTestWithoutToolsOrCommand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	err := os.WriteFile(path, []byte(`
name: test-agent
eval:
  tests:
    - question: "test question"
`), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err = LoadAgents(path, false, "")
	if err == nil {
		t.Fatal("expected error for eval test without expected_tools or command, got nil")
	}
}

func TestLoadAgentWithEvalCommand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	err := os.WriteFile(path, []byte(`
name: test-agent
eval:
  tests:
    - question: "test question"
      command: "python eval.py"
    - question: "another question"
      expected_tools:
        - tool_a
      command: "python check.py"
`), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	agents, err := LoadAgents(path, false, "")
	if err != nil {
		t.Fatalf("LoadAgents error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].Spec.Eval.Tests[0].Command != "python eval.py" {
		t.Errorf("unexpected command: %s", agents[0].Spec.Eval.Tests[0].Command)
	}
	if agents[0].Spec.Eval.Tests[1].Command != "python check.py" {
		t.Errorf("unexpected command: %s", agents[0].Spec.Eval.Tests[1].Command)
	}
	if len(agents[0].Spec.Eval.Tests[1].ExpectedTools) != 1 {
		t.Errorf("expected 1 expected tool, got %d", len(agents[0].Spec.Eval.Tests[1].ExpectedTools))
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

	agents, err := LoadAgents(path, false, "")
	if err != nil {
		t.Fatalf("LoadAgents error: %v", err)
	}
	if agents[0].Spec.Deploy.Grant.AccountRoles[0].Privileges[0] != "ALL" {
		t.Errorf("expected privilege ALL, got %s", agents[0].Spec.Deploy.Grant.AccountRoles[0].Privileges[0])
	}
}

func TestLoadAgentWithExpectedResponseOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	err := os.WriteFile(path, []byte(`
name: test-agent
eval:
  tests:
    - question: "会社概要をまとめて"
      expected_response: "当社はテクノロジー企業です"
`), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	agents, err := LoadAgents(path, false, "")
	if err != nil {
		t.Fatalf("LoadAgents error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].Spec.Eval.Tests[0].ExpectedResponse != "当社はテクノロジー企業です" {
		t.Errorf("unexpected expected_response: %s", agents[0].Spec.Eval.Tests[0].ExpectedResponse)
	}
}

func TestLoadAgentWithJudgeModel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	err := os.WriteFile(path, []byte(`
name: test-agent
eval:
  judge_model: claude-3-5-sonnet
  tests:
    - question: "test"
      expected_response: "response"
`), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	agents, err := LoadAgents(path, false, "")
	if err != nil {
		t.Fatalf("LoadAgents error: %v", err)
	}
	if agents[0].Spec.Eval.JudgeModel != "claude-3-5-sonnet" {
		t.Errorf("unexpected judge_model: %s", agents[0].Spec.Eval.JudgeModel)
	}
}

func TestLoadAgentWithResponseScoreThreshold(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	err := os.WriteFile(path, []byte(`
name: test-agent
eval:
  response_score_threshold: 80
  tests:
    - question: "test"
      expected_response: "response"
`), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	agents, err := LoadAgents(path, false, "")
	if err != nil {
		t.Fatalf("LoadAgents error: %v", err)
	}
	if agents[0].Spec.Eval.ResponseScoreThreshold == nil {
		t.Fatal("expected response_score_threshold to be set")
	}
	if *agents[0].Spec.Eval.ResponseScoreThreshold != 80 {
		t.Errorf("unexpected response_score_threshold: %d", *agents[0].Spec.Eval.ResponseScoreThreshold)
	}
}

func TestLoadAgentWithTestCaseResponseScoreThreshold(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	err := os.WriteFile(path, []byte(`
name: test-agent
eval:
  response_score_threshold: 70
  tests:
    - question: "strict test"
      expected_response: "response"
      response_score_threshold: 90
    - question: "default test"
      expected_response: "response"
`), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	agents, err := LoadAgents(path, false, "")
	if err != nil {
		t.Fatalf("LoadAgents error: %v", err)
	}
	tc0 := agents[0].Spec.Eval.Tests[0]
	if tc0.ResponseScoreThreshold == nil || *tc0.ResponseScoreThreshold != 90 {
		t.Errorf("test 0: expected response_score_threshold 90, got %v", tc0.ResponseScoreThreshold)
	}
	tc1 := agents[0].Spec.Eval.Tests[1]
	if tc1.ResponseScoreThreshold != nil {
		t.Errorf("test 1: expected nil response_score_threshold, got %d", *tc1.ResponseScoreThreshold)
	}
}

func TestLoadAgentRejectsEvalTestWithoutAnyExpectation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	err := os.WriteFile(path, []byte(`
name: test-agent
eval:
  tests:
    - question: "test question"
`), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err = LoadAgents(path, false, "")
	if err == nil {
		t.Fatal("expected error for eval test without any expectation, got nil")
	}
}

func TestLoadAgentWithVars(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	err := os.WriteFile(path, []byte(`
vars:
  dev:
    SNOWFLAKE_DATABASE: DEV_DB
    SNOWFLAKE_WAREHOUSE: DEV_WH
  default:
    SNOWFLAKE_DATABASE: MY_DB
    SNOWFLAKE_WAREHOUSE: COMPUTE_WH
name: test-agent
deploy:
  database: ${ vars.SNOWFLAKE_DATABASE }
`), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	agents, err := LoadAgents(path, false, "dev")
	if err != nil {
		t.Fatalf("LoadAgents error: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].Spec.Deploy == nil {
		t.Fatal("expected Deploy to be non-nil")
	}
	if agents[0].Spec.Deploy.Database != "DEV_DB" {
		t.Errorf("expected database DEV_DB, got %s", agents[0].Spec.Deploy.Database)
	}
}

func TestLoadAgentWithVarsDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	err := os.WriteFile(path, []byte(`
vars:
  dev:
    SNOWFLAKE_DATABASE: DEV_DB
  default:
    SNOWFLAKE_DATABASE: MY_DB
name: test-agent
deploy:
  database: ${ vars.SNOWFLAKE_DATABASE }
`), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	// No --env, should use default
	agents, err := LoadAgents(path, false, "")
	if err != nil {
		t.Fatalf("LoadAgents error: %v", err)
	}
	if agents[0].Spec.Deploy.Database != "MY_DB" {
		t.Errorf("expected database MY_DB, got %s", agents[0].Spec.Deploy.Database)
	}
}

func TestLoadAgentWithoutVars_BackwardCompatible(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	err := os.WriteFile(path, []byte(`
name: test-agent
deploy:
  database: HARDCODED_DB
`), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	agents, err := LoadAgents(path, false, "")
	if err != nil {
		t.Fatalf("LoadAgents error: %v", err)
	}
	if agents[0].Spec.Deploy.Database != "HARDCODED_DB" {
		t.Errorf("expected database HARDCODED_DB, got %s", agents[0].Spec.Deploy.Database)
	}
}

func TestLoadAgentWithVarsRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	err := os.WriteFile(path, []byte(`
vars:
  default:
    DB: MY_DB
name: test-agent
unknown_field: oops
`), 0o644)
	if err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err = LoadAgents(path, false, "")
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}
