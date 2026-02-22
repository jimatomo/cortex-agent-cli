package agent

import (
	"strings"
	"testing"
)

func TestValidate_Valid(t *testing.T) {
	spec := AgentSpec{Name: "my-agent"}
	if err := spec.Validate(); err != nil {
		t.Errorf("expected no error for minimal valid spec, got: %v", err)
	}
}

func TestValidate_EmptyName(t *testing.T) {
	spec := AgentSpec{Name: ""}
	err := spec.Validate()
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error should mention name, got: %v", err)
	}
}

func TestValidate_ToolEmptySpec(t *testing.T) {
	spec := AgentSpec{
		Name:  "agent",
		Tools: []Tool{{ToolSpec: map[string]any{}}},
	}
	err := spec.Validate()
	if err == nil {
		t.Fatal("expected error for empty tool_spec")
	}
	if !strings.Contains(err.Error(), "tool_spec") {
		t.Errorf("error should mention tool_spec, got: %v", err)
	}
}

func TestValidate_ToolMissingName(t *testing.T) {
	spec := AgentSpec{
		Name:  "agent",
		Tools: []Tool{{ToolSpec: map[string]any{"type": "cortex_analyst_text_to_sql"}}},
	}
	err := spec.Validate()
	if err == nil {
		t.Fatal("expected error for missing tool name")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error should mention name, got: %v", err)
	}
}

func TestValidate_ToolResourcesUnknownKey(t *testing.T) {
	spec := AgentSpec{
		Name:  "agent",
		Tools: []Tool{{ToolSpec: map[string]any{"name": "tool_a"}}},
		ToolResources: ToolResources{
			"unknown_tool": {"semantic_view": "DB.S.V"},
		},
	}
	err := spec.Validate()
	if err == nil {
		t.Fatal("expected error for unmatched tool_resources key")
	}
	if !strings.Contains(err.Error(), "unknown_tool") {
		t.Errorf("error should mention key name, got: %v", err)
	}
}

func TestValidate_ToolResourcesMatchingKey(t *testing.T) {
	spec := AgentSpec{
		Name:  "agent",
		Tools: []Tool{{ToolSpec: map[string]any{"name": "tool_a"}}},
		ToolResources: ToolResources{
			"tool_a": {"semantic_view": "DB.S.V"},
		},
	}
	if err := spec.Validate(); err != nil {
		t.Errorf("expected no error for matching tool_resources key, got: %v", err)
	}
}

func TestValidate_EvalEmptyQuestion(t *testing.T) {
	spec := AgentSpec{
		Name: "agent",
		Eval: &EvalConfig{
			Tests: []EvalTestCase{{Question: ""}},
		},
	}
	err := spec.Validate()
	if err == nil {
		t.Fatal("expected error for empty eval question")
	}
	if !strings.Contains(err.Error(), "question") {
		t.Errorf("error should mention question, got: %v", err)
	}
}

func TestValidate_EvalThresholdOutOfRange(t *testing.T) {
	threshold := 150
	spec := AgentSpec{
		Name: "agent",
		Eval: &EvalConfig{
			ResponseScoreThreshold: &threshold,
		},
	}
	err := spec.Validate()
	if err == nil {
		t.Fatal("expected error for threshold > 100")
	}
	if !strings.Contains(err.Error(), "threshold") {
		t.Errorf("error should mention threshold, got: %v", err)
	}
}

func TestValidate_EvalThresholdZero(t *testing.T) {
	threshold := 0
	spec := AgentSpec{
		Name: "agent",
		Eval: &EvalConfig{
			Tests:                  []EvalTestCase{{Question: "hello"}},
			ResponseScoreThreshold: &threshold,
		},
	}
	if err := spec.Validate(); err != nil {
		t.Errorf("threshold=0 should be valid (disables scoring), got: %v", err)
	}
}

func TestValidate_GrantEmptyRole(t *testing.T) {
	spec := AgentSpec{
		Name: "agent",
		Deploy: &DeployConfig{
			Grant: &GrantConfig{
				AccountRoles: []RoleGrant{{Role: "", Privileges: []string{"USAGE"}}},
			},
		},
	}
	err := spec.Validate()
	if err == nil {
		t.Fatal("expected error for empty grant role")
	}
	if !strings.Contains(err.Error(), "role") {
		t.Errorf("error should mention role, got: %v", err)
	}
}

func TestValidate_GrantEmptyPrivileges(t *testing.T) {
	spec := AgentSpec{
		Name: "agent",
		Deploy: &DeployConfig{
			Grant: &GrantConfig{
				AccountRoles: []RoleGrant{{Role: "ANALYST", Privileges: []string{}}},
			},
		},
	}
	err := spec.Validate()
	if err == nil {
		t.Fatal("expected error for empty grant privileges")
	}
	if !strings.Contains(err.Error(), "privileges") {
		t.Errorf("error should mention privileges, got: %v", err)
	}
}

func TestValidate_FullValidSpec(t *testing.T) {
	threshold := 70
	spec := AgentSpec{
		Name:    "my-agent",
		Comment: "A test agent",
		Profile: &Profile{DisplayName: "My Agent"},
		Models:  &Models{Orchestration: "claude-3-5-sonnet"},
		Instructions: &Instructions{
			System: "You are helpful.",
		},
		Tools: []Tool{
			{ToolSpec: map[string]any{"name": "sales_tool", "type": "cortex_analyst_text_to_sql"}},
		},
		ToolResources: ToolResources{
			"sales_tool": {"semantic_view": "DB.S.VIEW"},
		},
		Eval: &EvalConfig{
			Tests:                  []EvalTestCase{{Question: "What are sales?", ExpectedTools: []string{"sales_tool"}}},
			ResponseScoreThreshold: &threshold,
		},
		Deploy: &DeployConfig{
			Grant: &GrantConfig{
				AccountRoles: []RoleGrant{{Role: "ANALYST", Privileges: []string{"USAGE"}}},
			},
		},
	}
	if err := spec.Validate(); err != nil {
		t.Errorf("expected no error for full valid spec, got: %v", err)
	}
}
