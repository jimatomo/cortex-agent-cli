package cli

import (
	"strings"
	"testing"

	"coragent/internal/api"
	"coragent/internal/auth"
	"coragent/internal/diff"
	"coragent/internal/grant"
)

func TestFormatValue(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want string
	}{
		{"nil", nil, "null"},
		{"empty string", "", `""`},
		{"short string", "hello", `"hello"`},
		{"integer", 42, "42"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"exactly 80 chars", strings.Repeat("x", 80), `"` + strings.Repeat("x", 80) + `"`},
		{"81 chars truncated", strings.Repeat("x", 81), `"` + strings.Repeat("x", 77) + `"...`},
		{"long string", strings.Repeat("a", 200), `"` + strings.Repeat("a", 77) + `"...`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatValue(tt.v)
			if got != tt.want {
				t.Errorf("formatValue(%v) = %q, want %q", tt.v, got, tt.want)
			}
		})
	}
}

func TestChangeSymbol(t *testing.T) {
	tests := []struct {
		name     string
		ct       diff.ChangeType
		contains string
	}{
		{"added", diff.Added, "+"},
		{"removed", diff.Removed, "-"},
		{"modified", diff.Modified, "~"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := changeSymbol(tt.ct)
			if !strings.Contains(got, tt.contains) {
				t.Errorf("changeSymbol(%q) = %q, want to contain %q", tt.ct, got, tt.contains)
			}
		})
	}
}

func TestApplyAuthOverrides(t *testing.T) {
	tests := []struct {
		name    string
		cfg     auth.Config
		opts    *RootOptions
		wantCfg auth.Config
	}{
		{
			name:    "empty opts no change",
			cfg:     auth.Config{Account: "ORIG", Role: "ORIG_ROLE"},
			opts:    &RootOptions{},
			wantCfg: auth.Config{Account: "ORIG", Role: "ORIG_ROLE"},
		},
		{
			name:    "account uppercased",
			cfg:     auth.Config{},
			opts:    &RootOptions{Account: "myaccount"},
			wantCfg: auth.Config{Account: "MYACCOUNT"},
		},
		{
			name:    "role uppercased",
			cfg:     auth.Config{},
			opts:    &RootOptions{Role: "myrole"},
			wantCfg: auth.Config{Role: "MYROLE"},
		},
		{
			name:    "database not uppercased",
			cfg:     auth.Config{},
			opts:    &RootOptions{Database: "myDb"},
			wantCfg: auth.Config{Database: "myDb"},
		},
		{
			name:    "schema not uppercased",
			cfg:     auth.Config{},
			opts:    &RootOptions{Schema: "mySchema"},
			wantCfg: auth.Config{Schema: "mySchema"},
		},
		{
			name:    "whitespace skipped",
			cfg:     auth.Config{Account: "ORIG"},
			opts:    &RootOptions{Account: "  "},
			wantCfg: auth.Config{Account: "ORIG"},
		},
		{
			name:    "trims whitespace",
			cfg:     auth.Config{},
			opts:    &RootOptions{Account: "  acct  "},
			wantCfg: auth.Config{Account: "ACCT"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.cfg
			applyAuthOverrides(&cfg, tt.opts)
			if cfg.Account != tt.wantCfg.Account {
				t.Errorf("Account = %q, want %q", cfg.Account, tt.wantCfg.Account)
			}
			if cfg.Role != tt.wantCfg.Role {
				t.Errorf("Role = %q, want %q", cfg.Role, tt.wantCfg.Role)
			}
			if cfg.Database != tt.wantCfg.Database {
				t.Errorf("Database = %q, want %q", cfg.Database, tt.wantCfg.Database)
			}
			if cfg.Schema != tt.wantCfg.Schema {
				t.Errorf("Schema = %q, want %q", cfg.Schema, tt.wantCfg.Schema)
			}
		})
	}
}

func TestPlanToGrantRows(t *testing.T) {
	input := []api.ShowGrantsRow{
		{Privilege: "USAGE", GrantedTo: "ROLE", GranteeName: "ANALYST"},
		{Privilege: "OPERATE", GrantedTo: "DATABASE_ROLE", GranteeName: "DB.RUNNER"},
	}
	got := planToGrantRows(input)
	if len(got) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(got))
	}
	if got[0].Privilege != "USAGE" || got[0].GrantedTo != "ROLE" || got[0].GranteeName != "ANALYST" {
		t.Errorf("row 0 mismatch: %+v", got[0])
	}
	if got[1].Privilege != "OPERATE" || got[1].GrantedTo != "DATABASE_ROLE" || got[1].GranteeName != "DB.RUNNER" {
		t.Errorf("row 1 mismatch: %+v", got[1])
	}
}

func TestPlanToGrantRows_Empty(t *testing.T) {
	got := planToGrantRows(nil)
	if len(got) != 0 {
		t.Errorf("expected 0 rows, got %d", len(got))
	}
}

func TestPlanToGrantRows_TypeConversion(t *testing.T) {
	// Verify conversion produces the correct grant.ShowGrantsRow type
	input := []api.ShowGrantsRow{
		{Privilege: "USAGE", GrantedTo: "ROLE", GranteeName: "R1"},
	}
	result := planToGrantRows(input)
	var _ []grant.ShowGrantsRow = result // compile-time type check
	if result[0].Privilege != "USAGE" {
		t.Errorf("unexpected Privilege: %q", result[0].Privilege)
	}
}
