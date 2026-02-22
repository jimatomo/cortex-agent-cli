package agent

// RoleGrant specifies a role and the privileges to grant it on an agent.
// Corresponds to a single entry under deploy.grant.account_roles or
// deploy.grant.database_roles in the YAML spec.
type RoleGrant struct {
	// Role is the account role name (e.g. "ANALYST") or the fully-qualified
	// database role name (e.g. "MY_DB.DATA_READER").
	Role string `yaml:"role" json:"role"`
	// Privileges is the list of Snowflake privileges to grant.
	// Use "ALL" to expand to USAGE, MODIFY, and MONITOR.
	Privileges []string `yaml:"privileges" json:"privileges"`
}

// GrantConfig specifies role grants to apply whenever the agent is deployed.
// Corresponds to the deploy.grant block in the YAML spec.
type GrantConfig struct {
	// AccountRoles lists account-level role grants (GRANT … TO ROLE …).
	AccountRoles []RoleGrant `yaml:"account_roles,omitempty" json:"account_roles,omitempty"`
	// DatabaseRoles lists database-level role grants (GRANT … TO DATABASE ROLE …).
	DatabaseRoles []RoleGrant `yaml:"database_roles,omitempty" json:"database_roles,omitempty"`
}

// DeployConfig contains deployment-only settings that are not sent to the
// Snowflake Cortex Agent API. They are stripped from the JSON payload at
// marshal time via json:"-" on AgentSpec.Deploy.
type DeployConfig struct {
	// Database overrides the target Snowflake database for this agent.
	// Falls back to the connection-level default when omitted.
	Database string `yaml:"database,omitempty" json:"database,omitempty"`
	// Schema overrides the target Snowflake schema for this agent.
	// Falls back to the connection-level default when omitted.
	Schema string `yaml:"schema,omitempty" json:"schema,omitempty"`
	// QuoteIdentifiers wraps database/schema/agent name in double-quotes so
	// that mixed-case identifiers are preserved exactly as written.
	QuoteIdentifiers bool `yaml:"quote_identifiers,omitempty" json:"quote_identifiers,omitempty"`
	// Grant configures GRANT/REVOKE statements applied after each apply.
	Grant *GrantConfig `yaml:"grant,omitempty" json:"grant,omitempty"`
}

// EvalConfig contains evaluation test cases and judge configuration.
// It is stripped from the JSON payload (json:"-") and only used locally
// by the eval command.
type EvalConfig struct {
	// Tests is the list of test cases to run during eval.
	Tests []EvalTestCase `yaml:"tests" json:"tests"`
	// JudgeModel is the Snowflake Cortex model used to score responses.
	// Defaults to the value in .coragent.toml or the built-in default.
	JudgeModel string `yaml:"judge_model,omitempty" json:"judge_model,omitempty"`
	// ResponseScoreThreshold is the minimum score (0–100) a response must
	// achieve for the test case to be considered passed.
	// A nil value means response scoring is disabled for the agent.
	ResponseScoreThreshold *int `yaml:"response_score_threshold,omitempty" json:"response_score_threshold,omitempty"`
}

// EvalTestCase defines a single evaluation test case.
type EvalTestCase struct {
	// Question is the user message sent to the agent. Required.
	Question string `yaml:"question" json:"question"`
	// ExpectedTools lists tool names that must appear in the agent's response.
	// The test passes only if every listed tool was invoked.
	ExpectedTools []string `yaml:"expected_tools,omitempty" json:"expected_tools,omitempty"`
	// ExpectedResponse is the ideal answer text used for LLM-based scoring.
	// Requires a judge model to be configured.
	ExpectedResponse string `yaml:"expected_response,omitempty" json:"expected_response,omitempty"`
	// Command is a shell command that receives eval context via stdin (JSON)
	// and signals pass/fail via exit code (0 = pass).
	Command string `yaml:"command,omitempty" json:"command,omitempty"`
	// ResponseScoreThreshold overrides the agent-level threshold for this
	// specific test case. A pointer so that 0 can be used to disable scoring.
	ResponseScoreThreshold *int `yaml:"response_score_threshold,omitempty" json:"response_score_threshold,omitempty"`
}

// AgentSpec represents the Cortex Agent YAML/JSON schema payload.
// Fields tagged json:"-" are local-only and are never sent to the Snowflake API.
//
// Stable fields (guaranteed unchanged through v1.x):
//   - Name, Comment, Profile, Models, Instructions, Orchestration, Tools, ToolResources
//
// Local-only fields (not part of the API contract):
//   - Deploy, Eval
type AgentSpec struct {
	// Deploy contains deployment-only settings (database, schema, grants).
	// Not sent to the Snowflake API. Snowflake API counterpart: none.
	Deploy *DeployConfig `yaml:"deploy,omitempty" json:"-"`
	// Eval contains evaluation test cases run by the eval command.
	// Not sent to the Snowflake API. Snowflake API counterpart: none.
	Eval *EvalConfig `yaml:"eval,omitempty" json:"-"`
	// Name is the agent identifier within its schema. Must be unique.
	// Snowflake API counterpart: name.
	Name string `yaml:"name" json:"name" validate:"required"`
	// Comment is a free-text description shown in Snowflake UI.
	// Snowflake API counterpart: comment.
	Comment string `yaml:"comment,omitempty" json:"comment,omitempty"`
	// Profile controls how the agent appears in Snowflake's chat interface.
	// Snowflake API counterpart: profile.
	Profile *Profile `yaml:"profile,omitempty" json:"profile,omitempty"`
	// Models selects the underlying LLM(s) used by the agent.
	// Snowflake API counterpart: models.
	Models *Models `yaml:"models,omitempty" json:"models,omitempty"`
	// Instructions provide the system prompt and sample questions.
	// Snowflake API counterpart: instructions.
	Instructions *Instructions `yaml:"instructions,omitempty" json:"instructions,omitempty"`
	// Orchestration configures token and time budgets for agent reasoning.
	// Snowflake API counterpart: orchestration.
	Orchestration *Orchestration `yaml:"orchestration,omitempty" json:"orchestration,omitempty"`
	// Tools is the ordered list of tools available to the agent.
	// Snowflake API counterpart: tools.
	Tools []Tool `yaml:"tools,omitempty" json:"tools,omitempty"`
	// ToolResources provides per-tool configuration (semantic views, search services, etc.).
	// Keys are tool names matching tool_spec.name.
	// Snowflake API counterpart: tool_resources.
	ToolResources ToolResources `yaml:"tool_resources,omitempty" json:"tool_resources,omitempty"`
}

// Profile controls the agent's visual appearance in Snowflake's chat UI.
type Profile struct {
	// DisplayName is the human-readable name shown in the chat interface.
	DisplayName string `yaml:"display_name,omitempty" json:"display_name,omitempty"`
	// Avatar is the icon identifier (e.g. "GlobeAgentIcon").
	Avatar string `yaml:"avatar,omitempty" json:"avatar,omitempty"`
	// Color is the CSS color value used for the agent's accent color.
	Color string `yaml:"color,omitempty" json:"color,omitempty"`
}

// Models selects the LLM(s) used by the agent.
type Models struct {
	// Orchestration is the model used for the agent's main reasoning loop
	// (e.g. "claude-3-5-sonnet", "llama3.1-70b").
	Orchestration string `yaml:"orchestration,omitempty" json:"orchestration,omitempty"`
}

// Instructions configure the agent's prompts and example questions.
type Instructions struct {
	// Response is the response-formatting prompt shown after tool results.
	Response string `yaml:"response,omitempty" json:"response,omitempty"`
	// Orchestration is the system-level orchestration prompt (tool selection guidance).
	Orchestration string `yaml:"orchestration,omitempty" json:"orchestration,omitempty"`
	// System is the primary system prompt for the agent.
	System string `yaml:"system,omitempty" json:"system,omitempty"`
	// SampleQuestions is a list of suggested questions shown in the chat UI.
	SampleQuestions []SampleQuestion `yaml:"sample_questions,omitempty" json:"sample_questions,omitempty"`
}

// SampleQuestion is a single suggested question shown in the agent's chat UI.
type SampleQuestion struct {
	// Question is the text of the suggested question.
	Question string `yaml:"question" json:"question"`
}

// Orchestration configures resource budgets for the agent's reasoning loop.
type Orchestration struct {
	// Budget limits how much compute the agent may consume per request.
	Budget *BudgetConfig `yaml:"budget,omitempty" json:"budget,omitempty"`
}

// BudgetConfig limits compute usage per agent request.
type BudgetConfig struct {
	// Seconds is the maximum wall-clock time (in seconds) the agent may spend
	// on a single request.
	Seconds int `yaml:"seconds,omitempty" json:"seconds,omitempty"`
	// Tokens is the maximum number of LLM tokens the agent may consume
	// per request.
	Tokens int `yaml:"tokens,omitempty" json:"tokens,omitempty"`
}

// Tool wraps a single tool definition payload.
type Tool struct {
	// ToolSpec is the raw tool definition sent verbatim to the Snowflake API.
	// Required fields depend on the tool type (e.g. cortex_analyst_text_to_sql).
	ToolSpec map[string]any `yaml:"tool_spec" json:"tool_spec" validate:"required"`
}

// ToolResources allows per-tool configuration blocks keyed by tool name.
// Keys must match the name field inside the corresponding tool_spec.
// Values are tool-specific resource maps (e.g. semantic_view, search_service).
type ToolResources map[string]map[string]any
