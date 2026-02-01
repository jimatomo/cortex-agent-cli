package grant

import (
	"testing"

	"coragent/internal/agent"
)

func TestComputeDiff_AddGrants(t *testing.T) {
	desired := GrantState{
		Entries: []GrantEntry{
			{Privilege: "USAGE", RoleType: "ROLE", RoleName: "ANALYST_ROLE"},
			{Privilege: "MODIFY", RoleType: "ROLE", RoleName: "ADMIN_ROLE"},
		},
	}
	current := GrantState{}

	diff := ComputeDiff(desired, current)

	if len(diff.ToGrant) != 2 {
		t.Errorf("expected 2 grants, got %d", len(diff.ToGrant))
	}
	if len(diff.ToRevoke) != 0 {
		t.Errorf("expected 0 revokes, got %d", len(diff.ToRevoke))
	}
	if !diff.HasChanges() {
		t.Error("expected HasChanges() to return true")
	}
}

func TestComputeDiff_RemoveGrants(t *testing.T) {
	desired := GrantState{}
	current := GrantState{
		Entries: []GrantEntry{
			{Privilege: "USAGE", RoleType: "ROLE", RoleName: "OLD_ROLE"},
		},
	}

	diff := ComputeDiff(desired, current)

	if len(diff.ToGrant) != 0 {
		t.Errorf("expected 0 grants, got %d", len(diff.ToGrant))
	}
	if len(diff.ToRevoke) != 1 {
		t.Errorf("expected 1 revoke, got %d", len(diff.ToRevoke))
	}
	if diff.ToRevoke[0].RoleName != "OLD_ROLE" {
		t.Errorf("expected revoke role OLD_ROLE, got %s", diff.ToRevoke[0].RoleName)
	}
}

func TestComputeDiff_Mixed(t *testing.T) {
	desired := GrantState{
		Entries: []GrantEntry{
			{Privilege: "USAGE", RoleType: "ROLE", RoleName: "NEW_ROLE"},
			{Privilege: "USAGE", RoleType: "ROLE", RoleName: "KEEP_ROLE"},
		},
	}
	current := GrantState{
		Entries: []GrantEntry{
			{Privilege: "USAGE", RoleType: "ROLE", RoleName: "OLD_ROLE"},
			{Privilege: "USAGE", RoleType: "ROLE", RoleName: "KEEP_ROLE"},
		},
	}

	diff := ComputeDiff(desired, current)

	if len(diff.ToGrant) != 1 {
		t.Errorf("expected 1 grant, got %d", len(diff.ToGrant))
	}
	if len(diff.ToRevoke) != 1 {
		t.Errorf("expected 1 revoke, got %d", len(diff.ToRevoke))
	}
	if diff.ToGrant[0].RoleName != "NEW_ROLE" {
		t.Errorf("expected grant role NEW_ROLE, got %s", diff.ToGrant[0].RoleName)
	}
	if diff.ToRevoke[0].RoleName != "OLD_ROLE" {
		t.Errorf("expected revoke role OLD_ROLE, got %s", diff.ToRevoke[0].RoleName)
	}
}

func TestComputeDiff_NoChanges(t *testing.T) {
	entries := []GrantEntry{
		{Privilege: "USAGE", RoleType: "ROLE", RoleName: "SAME_ROLE"},
	}
	desired := GrantState{Entries: entries}
	current := GrantState{Entries: entries}

	diff := ComputeDiff(desired, current)

	if len(diff.ToGrant) != 0 {
		t.Errorf("expected 0 grants, got %d", len(diff.ToGrant))
	}
	if len(diff.ToRevoke) != 0 {
		t.Errorf("expected 0 revokes, got %d", len(diff.ToRevoke))
	}
	if diff.HasChanges() {
		t.Error("expected HasChanges() to return false")
	}
}

func TestFromGrantConfig(t *testing.T) {
	cfg := &agent.GrantConfig{
		AccountRoles: []agent.RoleGrant{
			{Role: "ANALYST_ROLE", Privileges: []string{"usage", "monitor"}},
		},
		DatabaseRoles: []agent.RoleGrant{
			{Role: "TEST_DB.DATA_READER", Privileges: []string{"USAGE"}},
		},
	}

	state := FromGrantConfig(cfg)

	if len(state.Entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(state.Entries))
	}

	// Check account role entries
	found := make(map[string]bool)
	for _, e := range state.Entries {
		key := e.Privilege + "|" + e.RoleType + "|" + e.RoleName
		found[key] = true
	}

	expected := []string{
		"USAGE|ROLE|ANALYST_ROLE",
		"MONITOR|ROLE|ANALYST_ROLE",
		"USAGE|DATABASE ROLE|TEST_DB.DATA_READER",
	}
	for _, exp := range expected {
		if !found[exp] {
			t.Errorf("expected entry %s not found", exp)
		}
	}
}

func TestFromGrantConfig_Nil(t *testing.T) {
	state := FromGrantConfig(nil)

	if len(state.Entries) != 0 {
		t.Errorf("expected 0 entries for nil config, got %d", len(state.Entries))
	}
}

func TestFromShowGrantsRows(t *testing.T) {
	rows := []ShowGrantsRow{
		{Privilege: "USAGE", GrantedTo: "ROLE", GranteeName: "ANALYST_ROLE"},
		{Privilege: "MODIFY", GrantedTo: "DATABASE_ROLE", GranteeName: "TEST_DB.ADMIN"},
	}

	state := FromShowGrantsRows(rows)

	if len(state.Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(state.Entries))
	}

	// Check first entry (account role)
	if state.Entries[0].Privilege != "USAGE" {
		t.Errorf("expected privilege USAGE, got %s", state.Entries[0].Privilege)
	}
	if state.Entries[0].RoleType != "ROLE" {
		t.Errorf("expected role type ROLE, got %s", state.Entries[0].RoleType)
	}
	if state.Entries[0].RoleName != "ANALYST_ROLE" {
		t.Errorf("expected role name ANALYST_ROLE, got %s", state.Entries[0].RoleName)
	}

	// Check second entry (database role)
	if state.Entries[1].Privilege != "MODIFY" {
		t.Errorf("expected privilege MODIFY, got %s", state.Entries[1].Privilege)
	}
	if state.Entries[1].RoleType != "DATABASE ROLE" {
		t.Errorf("expected role type DATABASE ROLE, got %s", state.Entries[1].RoleType)
	}
	if state.Entries[1].RoleName != "TEST_DB.ADMIN" {
		t.Errorf("expected role name TEST_DB.ADMIN, got %s", state.Entries[1].RoleName)
	}
}

func TestFromShowGrantsRows_Empty(t *testing.T) {
	state := FromShowGrantsRows(nil)

	if len(state.Entries) != 0 {
		t.Errorf("expected 0 entries for nil rows, got %d", len(state.Entries))
	}
}

func TestFromShowGrantsRows_IgnoresOwnership(t *testing.T) {
	rows := []ShowGrantsRow{
		{Privilege: "OWNERSHIP", GrantedTo: "ROLE", GranteeName: "OWNER_ROLE"},
		{Privilege: "USAGE", GrantedTo: "ROLE", GranteeName: "ANALYST_ROLE"},
		{Privilege: "ownership", GrantedTo: "ROLE", GranteeName: "OTHER_ROLE"},
	}

	state := FromShowGrantsRows(rows)

	// Should only have USAGE, OWNERSHIP entries should be filtered out
	if len(state.Entries) != 1 {
		t.Errorf("expected 1 entry (OWNERSHIP filtered), got %d", len(state.Entries))
	}
	if state.Entries[0].Privilege != "USAGE" {
		t.Errorf("expected privilege USAGE, got %s", state.Entries[0].Privilege)
	}
}

func TestComputeDiff_CaseInsensitive(t *testing.T) {
	// YAML specifies lowercase
	desired := GrantState{
		Entries: []GrantEntry{
			{Privilege: "usage", RoleType: "ROLE", RoleName: "analyst_role"},
			{Privilege: "MONITOR", RoleType: "DATABASE ROLE", RoleName: "test_db.data_reader"},
		},
	}
	// SHOW GRANTS returns uppercase
	current := GrantState{
		Entries: []GrantEntry{
			{Privilege: "USAGE", RoleType: "ROLE", RoleName: "ANALYST_ROLE"},
			{Privilege: "MONITOR", RoleType: "DATABASE ROLE", RoleName: "TEST_DB.DATA_READER"},
		},
	}

	diff := ComputeDiff(desired, current)

	// Should be no changes due to case-insensitive comparison
	if diff.HasChanges() {
		t.Errorf("expected no changes (case-insensitive), got %d grants and %d revokes",
			len(diff.ToGrant), len(diff.ToRevoke))
	}
}

func TestComputeDiff_DatabaseRoles(t *testing.T) {
	desired := GrantState{
		Entries: []GrantEntry{
			{Privilege: "USAGE", RoleType: "DATABASE ROLE", RoleName: "DB.NEW_READER"},
		},
	}
	current := GrantState{
		Entries: []GrantEntry{
			{Privilege: "USAGE", RoleType: "DATABASE ROLE", RoleName: "DB.OLD_READER"},
		},
	}

	diff := ComputeDiff(desired, current)

	if len(diff.ToGrant) != 1 {
		t.Errorf("expected 1 grant, got %d", len(diff.ToGrant))
	}
	if len(diff.ToRevoke) != 1 {
		t.Errorf("expected 1 revoke, got %d", len(diff.ToRevoke))
	}
	if diff.ToGrant[0].RoleName != "DB.NEW_READER" {
		t.Errorf("expected grant role DB.NEW_READER, got %s", diff.ToGrant[0].RoleName)
	}
	if diff.ToRevoke[0].RoleName != "DB.OLD_READER" {
		t.Errorf("expected revoke role DB.OLD_READER, got %s", diff.ToRevoke[0].RoleName)
	}
}

func TestComputeDiff_MultiplePrivileges(t *testing.T) {
	desired := GrantState{
		Entries: []GrantEntry{
			{Privilege: "USAGE", RoleType: "ROLE", RoleName: "ANALYST"},
			{Privilege: "MODIFY", RoleType: "ROLE", RoleName: "ANALYST"},
			{Privilege: "MONITOR", RoleType: "ROLE", RoleName: "ANALYST"},
		},
	}
	current := GrantState{
		Entries: []GrantEntry{
			{Privilege: "USAGE", RoleType: "ROLE", RoleName: "ANALYST"},
		},
	}

	diff := ComputeDiff(desired, current)

	if len(diff.ToGrant) != 2 {
		t.Errorf("expected 2 grants, got %d", len(diff.ToGrant))
	}
	if len(diff.ToRevoke) != 0 {
		t.Errorf("expected 0 revokes, got %d", len(diff.ToRevoke))
	}

	// Verify the specific privileges being added
	privs := make(map[string]bool)
	for _, e := range diff.ToGrant {
		privs[e.Privilege] = true
	}
	if !privs["MODIFY"] || !privs["MONITOR"] {
		t.Error("expected MODIFY and MONITOR to be added")
	}
}

func TestGrantDiff_HasChanges(t *testing.T) {
	tests := []struct {
		name     string
		diff     GrantDiff
		expected bool
	}{
		{
			name:     "empty diff",
			diff:     GrantDiff{},
			expected: false,
		},
		{
			name: "grants only",
			diff: GrantDiff{
				ToGrant: []GrantEntry{{Privilege: "USAGE", RoleType: "ROLE", RoleName: "R"}},
			},
			expected: true,
		},
		{
			name: "revokes only",
			diff: GrantDiff{
				ToRevoke: []GrantEntry{{Privilege: "USAGE", RoleType: "ROLE", RoleName: "R"}},
			},
			expected: true,
		},
		{
			name: "both grants and revokes",
			diff: GrantDiff{
				ToGrant:  []GrantEntry{{Privilege: "USAGE", RoleType: "ROLE", RoleName: "R1"}},
				ToRevoke: []GrantEntry{{Privilege: "USAGE", RoleType: "ROLE", RoleName: "R2"}},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.diff.HasChanges(); got != tt.expected {
				t.Errorf("HasChanges() = %v, expected %v", got, tt.expected)
			}
		})
	}
}
