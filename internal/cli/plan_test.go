package cli

import (
	"bytes"
	"strings"
	"testing"

	"coragent/internal/agent"
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
		{"81 chars full output", strings.Repeat("x", 81), `"` + strings.Repeat("x", 81) + `"`},
		{"long string full output", strings.Repeat("a", 200), `"` + strings.Repeat("a", 200) + `"`},
		{"long japanese string full output", strings.Repeat("あ", 40), `"` + strings.Repeat("あ", 40) + `"`},
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

func TestFormatChange_ModifiedScalarUsesMinusAndPlus(t *testing.T) {
	got := formatChange(diff.Change{
		Type:   diff.Modified,
		Before: "before",
		After:  "after",
	})

	want := []renderedChangeLine{
		{Type: diff.Removed, Text: `"before"`},
		{Type: diff.Added, Text: `"after"`},
	}
	if len(got) != len(want) {
		t.Fatalf("len(formatChange()) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("formatChange()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestFormatChange_ModifiedMultilineUsesLineDiff(t *testing.T) {
	got := formatChange(diff.Change{
		Type: diff.Modified,
		Before: strings.Join([]string{
			"line one",
			"line two",
			"line three",
		}, "\n"),
		After: strings.Join([]string{
			"line one",
			"line B",
			"line three",
		}, "\n"),
	})

	want := []renderedChangeLine{
		{Text: "line one", IsContext: true},
		{Type: diff.Removed, Text: "line two"},
		{Type: diff.Added, Text: "line B"},
		{Text: "line three", IsContext: true},
	}
	if len(got) != len(want) {
		t.Fatalf("len(formatChange()) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("formatChange()[%d] = %+v, want %+v", i, got[i], want[i])
		}
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
	got := convertGrantRows(input)
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
	got := convertGrantRows(nil)
	if len(got) != 0 {
		t.Errorf("expected 0 rows, got %d", len(got))
	}
}

func TestPlanToGrantRows_TypeConversion(t *testing.T) {
	input := []api.ShowGrantsRow{
		{Privilege: "USAGE", GrantedTo: "ROLE", GranteeName: "R1"},
	}
	result := convertGrantRows(input)
	if result[0].Privilege != "USAGE" {
		t.Errorf("unexpected Privilege: %q", result[0].Privilege)
	}
}

func TestWritePlanPreview_HidesUnchangedItems(t *testing.T) {
	items := []applyItem{
		{
			Parsed: agent.ParsedAgent{
				Path: "unchanged.yaml",
				Spec: agent.AgentSpec{Name: "UNCHANGED"},
			},
			Target: Target{Database: "TEST_DB", Schema: "PUBLIC"},
			Exists: true,
		},
		{
			Parsed: agent.ParsedAgent{
				Path: "updated.yaml",
				Spec: agent.AgentSpec{Name: "UPDATED"},
			},
			Target: Target{Database: "TEST_DB", Schema: "PUBLIC"},
			Exists: true,
			Changes: []diff.Change{
				{Path: "comment", Type: diff.Modified, Before: "old", After: "new"},
			},
		},
		{
			Parsed: agent.ParsedAgent{
				Path: "created.yaml",
				Spec: agent.AgentSpec{Name: "CREATED", Comment: "brand new"},
			},
			Target:    Target{Database: "TEST_DB", Schema: "PUBLIC"},
			Exists:    false,
			GrantDiff: grant.GrantDiff{},
		},
	}

	var buf bytes.Buffer
	summary, err := writePlanPreview(&buf, items)
	if err != nil {
		t.Fatalf("writePlanPreview: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "UNCHANGED:") {
		t.Fatalf("unchanged item should be hidden, got output:\n%s", out)
	}
	if strings.Contains(out, "= no changes") {
		t.Fatalf("no-change marker should not be shown, got output:\n%s", out)
	}
	if !strings.Contains(out, "UPDATED:") {
		t.Fatalf("updated item missing from output:\n%s", out)
	}
	if !strings.Contains(out, "  ~ comment =") {
		t.Fatalf("modified field header missing from output:\n%s", out)
	}
	if !strings.Contains(out, "      - \"old\"") {
		t.Fatalf("removed scalar line missing from output:\n%s", out)
	}
	if !strings.Contains(out, "      + \"new\"") {
		t.Fatalf("added scalar line missing from output:\n%s", out)
	}
	if !strings.Contains(out, "CREATED:") {
		t.Fatalf("created item missing from output:\n%s", out)
	}
	if !strings.Contains(out, `Plan: 1 to create, 1 to update, 1 unchanged`) {
		t.Fatalf("summary missing expected counts, got output:\n%s", out)
	}
	if summary.createCount != 1 || summary.updateCount != 1 || summary.noChangeCount != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestWritePlanPreview_ShowsMultilineStringDiff(t *testing.T) {
	items := []applyItem{
		{
			Parsed: agent.ParsedAgent{
				Path: "updated.yaml",
				Spec: agent.AgentSpec{Name: "UPDATED"},
			},
			Target: Target{Database: "TEST_DB", Schema: "PUBLIC"},
			Exists: true,
			Changes: []diff.Change{
				{
					Path: "instructions",
					Type: diff.Modified,
					Before: strings.Join([]string{
						"line 1",
						"line 2",
						"line 3",
						"line 4",
						"line 5 old",
						"line 6",
						"line 7",
						"line 8",
						"line 9",
					}, "\n"),
					After: strings.Join([]string{
						"line 1",
						"line 2",
						"line 3",
						"line 4",
						"line 5 new",
						"line 6",
						"line 7",
						"line 8",
						"line 9",
					}, "\n"),
				},
			},
		},
	}

	var buf bytes.Buffer
	_, err := writePlanPreview(&buf, items)
	if err != nil {
		t.Fatalf("writePlanPreview: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "  ~ instructions =") {
		t.Fatalf("multiline field header missing from output:\n%s", out)
	}
	for _, want := range []string{"        line 4", "      - line 5 old", "      + line 5 new", "        line 6"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected multiline diff context %q missing from output:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"        line 1", "        line 2", "        line 3", "        line 7", "        line 8", "        line 9"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("lines outside the 1-line context window should be omitted, got:\n%s", out)
		}
	}
}
