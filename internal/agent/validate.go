package agent

import "fmt"

// Validate checks the AgentSpec for required fields and obvious misconfigurations.
// It returns a descriptive error if the spec is invalid, or nil if it is valid.
//
// Rules enforced:
//   - Name must not be empty.
//   - Each Tool must have a non-empty tool_spec with a non-empty "name" field.
//   - ToolResources keys must match at least one tool name in Tools when both are present.
//   - EvalConfig.Tests must each have a non-empty Question.
//   - DeployConfig.Grant privileges must be non-empty for each RoleGrant.
func (s AgentSpec) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("agent name is required")
	}

	// Validate tools
	toolNames := make(map[string]bool)
	for i, tool := range s.Tools {
		if len(tool.ToolSpec) == 0 {
			return fmt.Errorf("tools[%d]: tool_spec must not be empty", i)
		}
		name, _ := tool.ToolSpec["name"].(string)
		if name == "" {
			return fmt.Errorf("tools[%d]: tool_spec.name is required", i)
		}
		toolNames[name] = true
	}

	// Validate tool_resources keys reference known tools
	if len(s.ToolResources) > 0 && len(s.Tools) > 0 {
		for key := range s.ToolResources {
			if !toolNames[key] {
				return fmt.Errorf("tool_resources key %q does not match any tool name", key)
			}
		}
	}

	// Validate eval test cases
	if s.Eval != nil {
		for i, tc := range s.Eval.Tests {
			if tc.Question == "" {
				return fmt.Errorf("eval.tests[%d]: question is required", i)
			}
		}
		if s.Eval.ResponseScoreThreshold != nil {
			v := *s.Eval.ResponseScoreThreshold
			if v < 0 || v > 100 {
				return fmt.Errorf("eval.response_score_threshold must be between 0 and 100, got %d", v)
			}
		}
	}

	// Validate grant config
	if s.Deploy != nil && s.Deploy.Grant != nil {
		g := s.Deploy.Grant
		for i, rg := range g.AccountRoles {
			if rg.Role == "" {
				return fmt.Errorf("deploy.grant.account_roles[%d]: role is required", i)
			}
			if len(rg.Privileges) == 0 {
				return fmt.Errorf("deploy.grant.account_roles[%d]: privileges must not be empty", i)
			}
		}
		for i, rg := range g.DatabaseRoles {
			if rg.Role == "" {
				return fmt.Errorf("deploy.grant.database_roles[%d]: role is required", i)
			}
			if len(rg.Privileges) == 0 {
				return fmt.Errorf("deploy.grant.database_roles[%d]: privileges must not be empty", i)
			}
		}
	}

	return nil
}
