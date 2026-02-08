package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"coragent/internal/agent"
)

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"404 status", APIError{StatusCode: 404, Body: ""}, true},
		{"does not exist in body", APIError{StatusCode: 400, Body: "object does not exist"}, true},
		{"002003 error code", APIError{StatusCode: 400, Body: "SQL error 002003"}, true},
		{"agent not found", APIError{StatusCode: 400, Body: "Agent not found"}, true},
		{"500 error", APIError{StatusCode: 500, Body: "internal server error"}, false},
		{"generic does not exist", fmt.Errorf("resource does not exist"), true},
		{"generic not found", fmt.Errorf("thing not found"), true},
		{"unrelated error", fmt.Errorf("connection timeout"), false},
		{"case insensitive", APIError{StatusCode: 400, Body: "DOES NOT EXIST"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNotFoundError(tt.err)
			if got != tt.want {
				t.Errorf("isNotFoundError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIdentifierSegment(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"simple", "MY_DB", "MY_DB"},
		{"with space", "my db", `"my db"`},
		{"with dash", "my-db", `"my-db"`},
		{"already quoted", `"MY_DB"`, "MY_DB"},
		{"special chars escaped", "a\"b", `"a""b"`},
		{"empty", "", `""`},
		{"underscore start", "_test", "_test"},
		{"dollar sign", "test$1", "test$1"},
		{"starts with number", "123abc", `"123abc"`},
		{"whitespace trimmed", "  MY_DB  ", "MY_DB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := identifierSegment(tt.value)
			if got != tt.want {
				t.Errorf("identifierSegment(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestIsSimpleIdentifier(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"alpha only", "MYDB", true},
		{"with underscore", "MY_DB", true},
		{"with dollar", "test$1", true},
		{"starts with number", "1abc", false},
		{"has dash", "my-db", false},
		{"has space", "my db", false},
		{"empty", "", false},
		{"lowercase", "mydb", true},
		{"mixed case", "MyDb", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSimpleIdentifier(tt.value)
			if got != tt.want {
				t.Errorf("isSimpleIdentifier(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestNormalizePayload(t *testing.T) {
	tests := []struct {
		name    string
		payload any
		check   func(any) error
	}{
		{
			name:    "map with name normalizes",
			payload: map[string]any{"name": "my-agent", "comment": "test"},
			check: func(result any) error {
				m := result.(map[string]any)
				if m["name"] != `"my-agent"` {
					return fmt.Errorf("name = %q, want %q", m["name"], `"my-agent"`)
				}
				return nil
			},
		},
		{
			name:    "map with simple name",
			payload: map[string]any{"name": "MY_AGENT"},
			check: func(result any) error {
				m := result.(map[string]any)
				if m["name"] != "MY_AGENT" {
					return fmt.Errorf("name = %q, want %q", m["name"], "MY_AGENT")
				}
				return nil
			},
		},
		{
			name:    "map without name",
			payload: map[string]any{"comment": "test"},
			check: func(result any) error {
				m := result.(map[string]any)
				if _, ok := m["name"]; ok {
					return fmt.Errorf("unexpected 'name' key")
				}
				return nil
			},
		},
		{
			name:    "non-map returns as is",
			payload: "just a string",
			check: func(result any) error {
				if result != "just a string" {
					return fmt.Errorf("got %v, want 'just a string'", result)
				}
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizePayload(tt.payload)
			if err := tt.check(result); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestNormalizeAgentSpecMap(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
		check func(map[string]any) error
	}{
		{
			name:  "toolResources to tool_resources",
			input: map[string]any{"toolResources": map[string]any{"tool1": map[string]any{}}},
			check: func(m map[string]any) error {
				if _, ok := m["tool_resources"]; !ok {
					return fmt.Errorf("missing 'tool_resources' key")
				}
				if _, ok := m["toolResources"]; ok {
					return fmt.Errorf("unexpected 'toolResources' key")
				}
				return nil
			},
		},
		{
			name:  "TOOLRESOURCES to tool_resources",
			input: map[string]any{"TOOLRESOURCES": map[string]any{}},
			check: func(m map[string]any) error {
				if _, ok := m["tool_resources"]; !ok {
					return fmt.Errorf("missing 'tool_resources' key")
				}
				return nil
			},
		},
		{
			name:  "tool_resources stays",
			input: map[string]any{"tool_resources": map[string]any{}},
			check: func(m map[string]any) error {
				if _, ok := m["tool_resources"]; !ok {
					return fmt.Errorf("missing 'tool_resources' key")
				}
				return nil
			},
		},
		{
			name:  "other keys preserved",
			input: map[string]any{"name": "test", "comment": "hello"},
			check: func(m map[string]any) error {
				if m["name"] != "test" {
					return fmt.Errorf("name = %v", m["name"])
				}
				if m["comment"] != "hello" {
					return fmt.Errorf("comment = %v", m["comment"])
				}
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeAgentSpecMap(tt.input)
			if err := tt.check(got); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestNormalizeToolResources(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
		check func(map[string]any) error
	}{
		{
			name: "array to object",
			input: map[string]any{
				"tool1": []any{map[string]any{"semantic_view": "sv1"}},
			},
			check: func(m map[string]any) error {
				obj, ok := m["tool1"].(map[string]any)
				if !ok {
					return fmt.Errorf("tool1 type = %T, want map[string]any", m["tool1"])
				}
				if obj["semantic_view"] != "sv1" {
					return fmt.Errorf("semantic_view = %v", obj["semantic_view"])
				}
				return nil
			},
		},
		{
			name: "already object",
			input: map[string]any{
				"tool1": map[string]any{"semantic_view": "sv1"},
			},
			check: func(m map[string]any) error {
				if _, ok := m["tool1"].(map[string]any); !ok {
					return fmt.Errorf("tool1 type = %T, want map[string]any", m["tool1"])
				}
				return nil
			},
		},
		{
			name: "empty array",
			input: map[string]any{
				"tool1": []any{},
			},
			check: func(m map[string]any) error {
				if _, ok := m["tool1"]; ok {
					return fmt.Errorf("tool1 should not be in output for empty array")
				}
				return nil
			},
		},
		{
			name: "non-map non-array",
			input: map[string]any{
				"tool1": "string_value",
			},
			check: func(m map[string]any) error {
				if m["tool1"] != "string_value" {
					return fmt.Errorf("tool1 = %v", m["tool1"])
				}
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeToolResources(tt.input)
			if err := tt.check(got); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestDecodeProfile(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		wantNil bool
		wantDN  string
		wantErr bool
	}{
		{"nil", nil, true, "", false},
		{"map", map[string]any{"display_name": "Agent"}, false, "Agent", false},
		{"json string", `{"display_name": "Agent"}`, false, "Agent", false},
		{"empty string", "", true, "", false},
		{"whitespace string", "   ", true, "", false},
		{"invalid json", "not json", false, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeProfile(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil profile")
			}
			if got.DisplayName != tt.wantDN {
				t.Errorf("DisplayName = %q, want %q", got.DisplayName, tt.wantDN)
			}
		})
	}
}

func TestMergeAgentSpecs(t *testing.T) {
	tests := []struct {
		name  string
		base  agent.AgentSpec
		extra agent.AgentSpec
		check func(agent.AgentSpec) error
	}{
		{
			name:  "name overwritten",
			base:  agent.AgentSpec{Name: "old"},
			extra: agent.AgentSpec{Name: "new"},
			check: func(s agent.AgentSpec) error {
				if s.Name != "new" {
					return fmt.Errorf("Name = %q", s.Name)
				}
				return nil
			},
		},
		{
			name:  "empty extra preserves base",
			base:  agent.AgentSpec{Name: "base", Comment: "kept"},
			extra: agent.AgentSpec{},
			check: func(s agent.AgentSpec) error {
				if s.Name != "base" || s.Comment != "kept" {
					return fmt.Errorf("got Name=%q Comment=%q", s.Name, s.Comment)
				}
				return nil
			},
		},
		{
			name:  "comment overwritten",
			base:  agent.AgentSpec{Comment: "old"},
			extra: agent.AgentSpec{Comment: "new"},
			check: func(s agent.AgentSpec) error {
				if s.Comment != "new" {
					return fmt.Errorf("Comment = %q", s.Comment)
				}
				return nil
			},
		},
		{
			name:  "profile overwritten",
			base:  agent.AgentSpec{Profile: &agent.Profile{DisplayName: "old"}},
			extra: agent.AgentSpec{Profile: &agent.Profile{DisplayName: "new"}},
			check: func(s agent.AgentSpec) error {
				if s.Profile.DisplayName != "new" {
					return fmt.Errorf("Profile.DisplayName = %q", s.Profile.DisplayName)
				}
				return nil
			},
		},
		{
			name: "tools overwritten",
			base: agent.AgentSpec{Tools: []agent.Tool{{ToolSpec: map[string]any{"name": "old"}}}},
			extra: agent.AgentSpec{Tools: []agent.Tool{{ToolSpec: map[string]any{"name": "new"}}}},
			check: func(s agent.AgentSpec) error {
				if len(s.Tools) != 1 || s.Tools[0].ToolSpec["name"] != "new" {
					return fmt.Errorf("Tools = %+v", s.Tools)
				}
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeAgentSpecs(tt.base, tt.extra)
			if err := tt.check(got); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestDecodeAgentSpec(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		wantName string
		wantErr  bool
	}{
		{
			name:     "direct JSON",
			data:     []byte(`{"name":"agent1","comment":"test"}`),
			wantName: "agent1",
		},
		{
			name:     "nested agent_spec",
			data:     []byte(`{"name":"agent1","agent_spec":"{\"name\":\"inner\",\"comment\":\"test\"}"}`),
			wantName: "inner",
		},
		{
			name:     "empty",
			data:     []byte{},
			wantName: "",
		},
		{
			name:    "invalid json",
			data:    []byte(`not json`),
			wantErr: true,
		},
		{
			name:    "array wrapping",
			data:    []byte(`[{"name":"agent1","comment":"test"}]`),
			wantErr: true, // arrays cannot directly unmarshal to AgentSpec
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeAgentSpec(tt.data)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantName)
			}
		})
	}
}

func TestLooksLikeAgentSpec(t *testing.T) {
	knownKeys := []string{"name", "comment", "profile", "models", "instructions", "orchestration", "tools", "tool_resources"}
	for _, key := range knownKeys {
		t.Run("has_"+key, func(t *testing.T) {
			m := map[string]any{key: "value"}
			if !looksLikeAgentSpec(m) {
				t.Errorf("looksLikeAgentSpec with key %q should return true", key)
			}
		})
	}

	t.Run("unrelated keys", func(t *testing.T) {
		m := map[string]any{"foo": "bar", "baz": 42}
		if looksLikeAgentSpec(m) {
			t.Errorf("looksLikeAgentSpec with unrelated keys should return false")
		}
	})

	t.Run("empty map", func(t *testing.T) {
		m := map[string]any{}
		if looksLikeAgentSpec(m) {
			t.Errorf("looksLikeAgentSpec with empty map should return false")
		}
	})
}

func TestNormalizeAgentKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{"toolResources", "toolResources", "tool_resources"},
		{"tool_resources", "tool_resources", "tool_resources"},
		{"TOOLRESOURCES", "TOOLRESOURCES", "tool_resources"},
		{"ToolResources", "ToolResources", "tool_resources"},
		{"name", "name", "name"},
		{"comment", "comment", "comment"},
		{"tools", "tools", "tools"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeAgentKey(tt.key)
			if got != tt.want {
				t.Errorf("normalizeAgentKey(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestTruncateDebug(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"short", []byte("hello"), "hello"},
		{"exactly 4000", []byte(strings.Repeat("x", 4000)), strings.Repeat("x", 4000)},
		{"4001 truncated", []byte(strings.Repeat("x", 4001)), strings.Repeat("x", 4000) + "...(truncated)"},
		{"empty", []byte{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateDebug(tt.data)
			if got != tt.want {
				t.Errorf("truncateDebug len=%d, got len=%d", len(tt.want), len(got))
			}
		})
	}
}

func TestAPIError_Error(t *testing.T) {
	err := APIError{StatusCode: 404, Body: "not found"}
	got := err.Error()
	if !strings.Contains(got, "404") || !strings.Contains(got, "not found") {
		t.Errorf("Error() = %q, want to contain status and body", got)
	}
}

func TestDecodeProfile_DefaultCase(t *testing.T) {
	// Test with a non-standard type (falls through to default case)
	// json.Number marshals to a JSON number, which can't decode to Profile
	_, err := decodeProfile(json.Number("42"))
	if err == nil {
		t.Error("expected error when decoding number as Profile")
	}
}
