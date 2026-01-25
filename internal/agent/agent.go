package agent

// DeployConfig contains optional deployment-only settings.
type DeployConfig struct {
	Database string `yaml:"database,omitempty" json:"database,omitempty"`
	Schema   string `yaml:"schema,omitempty" json:"schema,omitempty"`
}

// AgentSpec represents the Cortex Agent YAML/JSON schema payload.
type AgentSpec struct {
	Deploy        *DeployConfig  `yaml:"deploy,omitempty" json:"-"`
	Name          string         `yaml:"name" json:"name" validate:"required"`
	Comment       string         `yaml:"comment,omitempty" json:"comment,omitempty"`
	Profile       *Profile       `yaml:"profile,omitempty" json:"profile,omitempty"`
	Models        *Models        `yaml:"models,omitempty" json:"models,omitempty"`
	Instructions  *Instructions  `yaml:"instructions,omitempty" json:"instructions,omitempty"`
	Orchestration *Orchestration `yaml:"orchestration,omitempty" json:"orchestration,omitempty"`
	Tools         []Tool         `yaml:"tools,omitempty" json:"tools,omitempty"`
	ToolResources ToolResources  `yaml:"tool_resources,omitempty" json:"tool_resources,omitempty"`
}

type Profile struct {
	DisplayName string `yaml:"display_name,omitempty" json:"display_name,omitempty"`
}

type Models struct {
	Orchestration string `yaml:"orchestration,omitempty" json:"orchestration,omitempty"`
}

type Instructions struct {
	Response        string           `yaml:"response,omitempty" json:"response,omitempty"`
	Orchestration   string           `yaml:"orchestration,omitempty" json:"orchestration,omitempty"`
	System          string           `yaml:"system,omitempty" json:"system,omitempty"`
	SampleQuestions []SampleQuestion `yaml:"sample_questions,omitempty" json:"sample_questions,omitempty"`
}

type SampleQuestion struct {
	Question string `yaml:"question" json:"question"`
}

type Orchestration struct {
	Budget *BudgetConfig `yaml:"budget,omitempty" json:"budget,omitempty"`
}

type BudgetConfig struct {
	Seconds int `yaml:"seconds,omitempty" json:"seconds,omitempty"`
	Tokens  int `yaml:"tokens,omitempty" json:"tokens,omitempty"`
}

type Tool struct {
	ToolSpec map[string]any `yaml:"tool_spec" json:"tool_spec" validate:"required"`
}

// ToolResources allows tool-specific configuration blocks.
// The keys are tool types (e.g., "cortex_search"), and each entry is a list of resources.
type ToolResources map[string][]map[string]any
