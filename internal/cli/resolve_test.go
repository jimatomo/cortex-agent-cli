package cli

import (
	"testing"

	"coragent/internal/agent"
	"coragent/internal/auth"
)

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{"no values", nil, ""},
		{"all empty", []string{"", "", ""}, ""},
		{"first match", []string{"a", "b"}, "a"},
		{"middle match", []string{"", "b", "c"}, "b"},
		{"whitespace only skipped", []string{"  ", "\t", "c"}, "c"},
		{"trims result", []string{"  hello  "}, "hello"},
		{"single empty", []string{""}, ""},
		{"single value", []string{"x"}, "x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstNonEmpty(tt.values...)
			if got != tt.want {
				t.Errorf("firstNonEmpty(%v) = %q, want %q", tt.values, got, tt.want)
			}
		})
	}
}

func TestDeployValue(t *testing.T) {
	tests := []struct {
		name string
		spec agent.AgentSpec
		key  string
		want string
	}{
		{"nil deploy", agent.AgentSpec{}, "database", ""},
		{"database", agent.AgentSpec{Deploy: &agent.DeployConfig{Database: "MY_DB"}}, "database", "MY_DB"},
		{"schema", agent.AgentSpec{Deploy: &agent.DeployConfig{Schema: "MY_SCHEMA"}}, "schema", "MY_SCHEMA"},
		{"trims whitespace", agent.AgentSpec{Deploy: &agent.DeployConfig{Database: "  DB  "}}, "database", "DB"},
		{"unknown key", agent.AgentSpec{Deploy: &agent.DeployConfig{Database: "DB"}}, "unknown", ""},
		{"empty database", agent.AgentSpec{Deploy: &agent.DeployConfig{Database: ""}}, "database", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deployValue(tt.spec, tt.key)
			if got != tt.want {
				t.Errorf("deployValue(spec, %q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestResolveTarget(t *testing.T) {
	tests := []struct {
		name    string
		spec    agent.AgentSpec
		opts    *RootOptions
		cfg     auth.Config
		wantDB  string
		wantSch string
		wantErr bool
	}{
		{
			name:    "opts highest priority",
			spec:    agent.AgentSpec{Deploy: &agent.DeployConfig{Database: "DEPLOY_DB", Schema: "DEPLOY_SCH"}},
			opts:    &RootOptions{Database: "OPTS_DB", Schema: "OPTS_SCH"},
			cfg:     auth.Config{Database: "CFG_DB", Schema: "CFG_SCH"},
			wantDB:  "OPTS_DB",
			wantSch: "OPTS_SCH",
		},
		{
			name:    "deploy fallback",
			spec:    agent.AgentSpec{Deploy: &agent.DeployConfig{Database: "DEPLOY_DB", Schema: "DEPLOY_SCH"}},
			opts:    &RootOptions{},
			cfg:     auth.Config{Database: "CFG_DB", Schema: "CFG_SCH"},
			wantDB:  "DEPLOY_DB",
			wantSch: "DEPLOY_SCH",
		},
		{
			name:    "config fallback",
			spec:    agent.AgentSpec{},
			opts:    &RootOptions{},
			cfg:     auth.Config{Database: "CFG_DB", Schema: "CFG_SCH"},
			wantDB:  "CFG_DB",
			wantSch: "CFG_SCH",
		},
		{
			name:    "missing database",
			spec:    agent.AgentSpec{},
			opts:    &RootOptions{Schema: "S"},
			cfg:     auth.Config{},
			wantErr: true,
		},
		{
			name:    "missing schema",
			spec:    agent.AgentSpec{},
			opts:    &RootOptions{Database: "D"},
			cfg:     auth.Config{},
			wantErr: true,
		},
		{
			name:    "both missing",
			spec:    agent.AgentSpec{},
			opts:    &RootOptions{},
			cfg:     auth.Config{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveTarget(tt.spec, tt.opts, tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Database != tt.wantDB {
				t.Errorf("Database = %q, want %q", got.Database, tt.wantDB)
			}
			if got.Schema != tt.wantSch {
				t.Errorf("Schema = %q, want %q", got.Schema, tt.wantSch)
			}
		})
	}
}

func TestResolveTargetForExport(t *testing.T) {
	tests := []struct {
		name    string
		opts    *RootOptions
		cfg     auth.Config
		wantDB  string
		wantSch string
		wantErr bool
	}{
		{
			name:    "opts priority",
			opts:    &RootOptions{Database: "OPTS_DB", Schema: "OPTS_SCH"},
			cfg:     auth.Config{Database: "CFG_DB", Schema: "CFG_SCH"},
			wantDB:  "OPTS_DB",
			wantSch: "OPTS_SCH",
		},
		{
			name:    "config fallback",
			opts:    &RootOptions{},
			cfg:     auth.Config{Database: "CFG_DB", Schema: "CFG_SCH"},
			wantDB:  "CFG_DB",
			wantSch: "CFG_SCH",
		},
		{
			name:    "missing both",
			opts:    &RootOptions{},
			cfg:     auth.Config{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveTargetForExport(tt.opts, tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Database != tt.wantDB {
				t.Errorf("Database = %q, want %q", got.Database, tt.wantDB)
			}
			if got.Schema != tt.wantSch {
				t.Errorf("Schema = %q, want %q", got.Schema, tt.wantSch)
			}
		})
	}
}
