package cli

import (
	"testing"

	"coragent/internal/agent"
	"coragent/internal/api"
	"coragent/internal/diff"
)

func TestTopLevel(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"empty", "", ""},
		{"simple", "tools", "tools"},
		{"dotted", "tools.name", "tools"},
		{"indexed", "tools[0]", "tools"},
		{"deep dotted", "tools[0].name.sub", "tools"},
		{"profile", "profile", "profile"},
		{"profile.display_name", "profile.display_name", "profile"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := topLevel(tt.path)
			if got != tt.want {
				t.Errorf("topLevel(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestEmptyValueForKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want any
	}{
		{"tools", "tools", []any{}},
		{"tool_resources", "tool_resources", map[string]any{}},
		{"other", "comment", nil},
		{"name", "name", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := emptyValueForKey(tt.key)
			switch expected := tt.want.(type) {
			case nil:
				if got != nil {
					t.Errorf("emptyValueForKey(%q) = %v, want nil", tt.key, got)
				}
			case []any:
				arr, ok := got.([]any)
				if !ok {
					t.Errorf("emptyValueForKey(%q) type = %T, want []any", tt.key, got)
				} else if len(arr) != len(expected) {
					t.Errorf("emptyValueForKey(%q) len = %d, want %d", tt.key, len(arr), len(expected))
				}
			case map[string]any:
				m, ok := got.(map[string]any)
				if !ok {
					t.Errorf("emptyValueForKey(%q) type = %T, want map[string]any", tt.key, got)
				} else if len(m) != len(expected) {
					t.Errorf("emptyValueForKey(%q) len = %d, want %d", tt.key, len(m), len(expected))
				}
			}
		})
	}
}

func TestUpdatePayload(t *testing.T) {
	tests := []struct {
		name    string
		spec    agent.AgentSpec
		changes []diff.Change
		wantKey string
		wantErr bool
	}{
		{
			name: "single change",
			spec: agent.AgentSpec{Name: "agent1", Comment: "updated"},
			changes: []diff.Change{
				{Path: "comment", Type: diff.Modified},
			},
			wantKey: "comment",
		},
		{
			name: "nested path uses top level",
			spec: agent.AgentSpec{Name: "agent1", Profile: &agent.Profile{DisplayName: "test"}},
			changes: []diff.Change{
				{Path: "profile.display_name", Type: diff.Modified},
			},
			wantKey: "profile",
		},
		{
			name: "empty change path skipped",
			spec: agent.AgentSpec{Name: "agent1"},
			changes: []diff.Change{
				{Path: "", Type: diff.Modified},
			},
		},
		{
			name: "deleted field gets empty value",
			spec: agent.AgentSpec{Name: "agent1"},
			changes: []diff.Change{
				{Path: "tools[0]", Type: diff.Removed},
			},
			wantKey: "tools",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := updatePayload(tt.spec, tt.changes)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantKey != "" {
				if _, ok := got[tt.wantKey]; !ok {
					t.Errorf("expected key %q in payload, got keys: %v", tt.wantKey, keysOf(got))
				}
			}
		})
	}
}

func TestUpdatePayload_MultipleChanges(t *testing.T) {
	spec := agent.AgentSpec{
		Name:    "agent1",
		Comment: "new comment",
		Profile: &agent.Profile{DisplayName: "Agent"},
	}
	changes := []diff.Change{
		{Path: "comment", Type: diff.Modified},
		{Path: "profile.display_name", Type: diff.Modified},
	}
	got, err := updatePayload(spec, changes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := got["comment"]; !ok {
		t.Error("expected 'comment' key")
	}
	if _, ok := got["profile"]; !ok {
		t.Error("expected 'profile' key")
	}
}

func TestToGrantRows(t *testing.T) {
	input := []api.ShowGrantsRow{
		{Privilege: "USAGE", GrantedTo: "ROLE", GranteeName: "ANALYST"},
		{Privilege: "OPERATE", GrantedTo: "DATABASE_ROLE", GranteeName: "DB.RUNNER"},
	}
	got := convertGrantRows(input)
	if len(got) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(got))
	}
	if got[0].Privilege != "USAGE" {
		t.Errorf("row 0 Privilege = %q, want %q", got[0].Privilege, "USAGE")
	}
	if got[1].GranteeName != "DB.RUNNER" {
		t.Errorf("row 1 GranteeName = %q, want %q", got[1].GranteeName, "DB.RUNNER")
	}
}

func TestToGrantRows_Empty(t *testing.T) {
	got := convertGrantRows(nil)
	if len(got) != 0 {
		t.Errorf("expected 0 rows, got %d", len(got))
	}
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
