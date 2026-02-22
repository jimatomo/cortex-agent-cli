package cli

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"coragent/internal/agent"
	"coragent/internal/api"
	"coragent/internal/diff"
	"coragent/internal/grant"
)

// applyFakeService tracks API calls and supports per-method error injection.
// It is used by apply_core tests to verify which operations were performed.
type applyFakeService struct {
	// State
	Agents map[string]agent.AgentSpec

	// Call tracking
	CreateCalls []string // agent names passed to CreateAgent
	UpdateCalls []string // agent names passed to UpdateAgent
	GrantCalls  []string // "privilege:roleType:roleName" per ExecuteGrant call
	RevokeCalls []string // "privilege:roleType:roleName" per ExecuteRevoke call

	// Error injection
	CreateErr error
	UpdateErr error
	GrantErr  error
	RevokeErr error
}

func (f *applyFakeService) key(db, schema, name string) string {
	return fmt.Sprintf("%s.%s.%s", db, schema, name)
}

func (f *applyFakeService) CreateAgent(_ context.Context, _, _ string, spec agent.AgentSpec) error {
	if f.CreateErr != nil {
		return f.CreateErr
	}
	f.CreateCalls = append(f.CreateCalls, spec.Name)
	return nil
}

func (f *applyFakeService) UpdateAgent(_ context.Context, _, _, name string, _ any) error {
	if f.UpdateErr != nil {
		return f.UpdateErr
	}
	f.UpdateCalls = append(f.UpdateCalls, name)
	return nil
}

func (f *applyFakeService) DeleteAgent(_ context.Context, _, _, _ string) error { return nil }

func (f *applyFakeService) GetAgent(_ context.Context, db, schema, name string) (agent.AgentSpec, bool, error) {
	spec, ok := f.Agents[f.key(db, schema, name)]
	return spec, ok, nil
}

func (f *applyFakeService) DescribeAgent(_ context.Context, _, _, _ string) (api.DescribeResult, error) {
	return api.DescribeResult{}, nil
}

func (f *applyFakeService) ListAgents(_ context.Context, _, _ string) ([]api.AgentListItem, error) {
	return nil, nil
}

func (f *applyFakeService) ShowGrants(_ context.Context, _, _, _ string) ([]api.ShowGrantsRow, error) {
	return nil, nil
}

func (f *applyFakeService) ExecuteGrant(_ context.Context, _, _, _, roleType, roleName, privilege string) error {
	if f.GrantErr != nil {
		return f.GrantErr
	}
	f.GrantCalls = append(f.GrantCalls, privilege+":"+roleType+":"+roleName)
	return nil
}

func (f *applyFakeService) ExecuteRevoke(_ context.Context, _, _, _, roleType, roleName, privilege string) error {
	if f.RevokeErr != nil {
		return f.RevokeErr
	}
	f.RevokeCalls = append(f.RevokeCalls, privilege+":"+roleType+":"+roleName)
	return nil
}

// newApplyItem constructs an applyItem suitable for executeApply tests.
func newApplyItem(name string, exists bool, changes []diff.Change, gd grant.GrantDiff) applyItem {
	return applyItem{
		Parsed:    agent.ParsedAgent{Path: name + ".yaml", Spec: agent.AgentSpec{Name: name}},
		Target:    Target{Database: "DB", Schema: "PUBLIC"},
		Exists:    exists,
		Changes:   changes,
		GrantDiff: gd,
	}
}

// --- executeApply tests ---

// TestExecuteApply_Create verifies that a new agent item calls CreateAgent.
func TestExecuteApply_Create(t *testing.T) {
	svc := &applyFakeService{}
	item := newApplyItem("new-agent", false, nil, grant.GrantDiff{})

	applied, err := executeApply(context.Background(), []applyItem{item}, svc, svc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(applied) != 1 {
		t.Errorf("expected 1 applied item, got %d", len(applied))
	}
	if len(svc.CreateCalls) != 1 || svc.CreateCalls[0] != "new-agent" {
		t.Errorf("CreateCalls = %v, want [new-agent]", svc.CreateCalls)
	}
	if len(svc.UpdateCalls) != 0 {
		t.Errorf("unexpected UpdateCalls: %v", svc.UpdateCalls)
	}
}

// TestExecuteApply_Update verifies that an existing changed item calls UpdateAgent.
func TestExecuteApply_Update(t *testing.T) {
	svc := &applyFakeService{}
	changes := []diff.Change{{Path: "comment", Type: diff.Modified, Before: "old", After: "new"}}
	item := newApplyItem("existing-agent", true, changes, grant.GrantDiff{})

	applied, err := executeApply(context.Background(), []applyItem{item}, svc, svc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(applied) != 1 {
		t.Errorf("expected 1 applied item, got %d", len(applied))
	}
	if len(svc.UpdateCalls) != 1 || svc.UpdateCalls[0] != "existing-agent" {
		t.Errorf("UpdateCalls = %v, want [existing-agent]", svc.UpdateCalls)
	}
	if len(svc.CreateCalls) != 0 {
		t.Errorf("unexpected CreateCalls: %v", svc.CreateCalls)
	}
}

// TestExecuteApply_NoChange verifies that an unchanged existing item is not
// returned in applied items and does not call Create or Update.
func TestExecuteApply_NoChange(t *testing.T) {
	svc := &applyFakeService{}
	item := newApplyItem("unchanged", true, nil, grant.GrantDiff{})

	applied, err := executeApply(context.Background(), []applyItem{item}, svc, svc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(applied) != 0 {
		t.Errorf("expected 0 applied items for no-change, got %d", len(applied))
	}
	if len(svc.CreateCalls) != 0 || len(svc.UpdateCalls) != 0 {
		t.Errorf("unexpected API calls for no-change item: create=%v update=%v",
			svc.CreateCalls, svc.UpdateCalls)
	}
}

// TestExecuteApply_GrantsOnCreate verifies that grants are applied after creating an agent.
func TestExecuteApply_GrantsOnCreate(t *testing.T) {
	svc := &applyFakeService{}
	gd := grant.GrantDiff{
		ToGrant: []grant.GrantEntry{
			{Privilege: "USAGE", RoleType: "ROLE", RoleName: "ANALYST"},
		},
	}
	item := newApplyItem("new-agent", false, nil, gd)

	_, err := executeApply(context.Background(), []applyItem{item}, svc, svc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(svc.GrantCalls) != 1 {
		t.Fatalf("expected 1 grant call, got %d: %v", len(svc.GrantCalls), svc.GrantCalls)
	}
	if svc.GrantCalls[0] != "USAGE:ROLE:ANALYST" {
		t.Errorf("GrantCalls[0] = %q, want %q", svc.GrantCalls[0], "USAGE:ROLE:ANALYST")
	}
}

// TestExecuteApply_GrantsOnNoChange verifies that grants are applied even for
// agents with no spec changes (to converge on desired state).
func TestExecuteApply_GrantsOnNoChange(t *testing.T) {
	svc := &applyFakeService{}
	gd := grant.GrantDiff{
		ToGrant: []grant.GrantEntry{
			{Privilege: "MONITOR", RoleType: "ROLE", RoleName: "OPS"},
		},
	}
	item := newApplyItem("unchanged", true, nil, gd)

	_, err := executeApply(context.Background(), []applyItem{item}, svc, svc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(svc.GrantCalls) != 1 {
		t.Errorf("expected 1 grant call for no-change item, got %d", len(svc.GrantCalls))
	}
}

// TestExecuteApply_CreateError verifies that CreateAgent errors are propagated.
func TestExecuteApply_CreateError(t *testing.T) {
	svc := &applyFakeService{CreateErr: fmt.Errorf("API unavailable")}
	item := newApplyItem("new-agent", false, nil, grant.GrantDiff{})

	_, err := executeApply(context.Background(), []applyItem{item}, svc, svc)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "new-agent") {
		t.Errorf("error = %q, expected to mention agent name", err.Error())
	}
}

// TestExecuteApply_UpdateError verifies that UpdateAgent errors are propagated.
func TestExecuteApply_UpdateError(t *testing.T) {
	svc := &applyFakeService{UpdateErr: fmt.Errorf("update failed")}
	changes := []diff.Change{{Path: "comment", Type: diff.Modified, Before: "a", After: "b"}}
	item := newApplyItem("agent", true, changes, grant.GrantDiff{})

	_, err := executeApply(context.Background(), []applyItem{item}, svc, svc)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestExecuteApply_Multiple verifies mixed create/update/no-change batch.
func TestExecuteApply_Multiple(t *testing.T) {
	svc := &applyFakeService{}
	changes := []diff.Change{{Path: "comment", Type: diff.Modified, Before: "a", After: "b"}}
	items := []applyItem{
		newApplyItem("new-agent", false, nil, grant.GrantDiff{}),
		newApplyItem("changed-agent", true, changes, grant.GrantDiff{}),
		newApplyItem("unchanged-agent", true, nil, grant.GrantDiff{}),
	}

	applied, err := executeApply(context.Background(), items, svc, svc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(applied) != 2 {
		t.Errorf("expected 2 applied items (create+update), got %d", len(applied))
	}
	if len(svc.CreateCalls) != 1 {
		t.Errorf("expected 1 create call, got %v", svc.CreateCalls)
	}
	if len(svc.UpdateCalls) != 1 {
		t.Errorf("expected 1 update call, got %v", svc.UpdateCalls)
	}
}

// --- applyGrantDiff tests ---

// TestApplyGrantDiff_NoChanges verifies that no-op diff causes no API calls.
func TestApplyGrantDiff_NoChanges(t *testing.T) {
	svc := &applyFakeService{}
	err := applyGrantDiff(context.Background(), svc, "DB", "S", "agent", grant.GrantDiff{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(svc.GrantCalls) != 0 || len(svc.RevokeCalls) != 0 {
		t.Errorf("expected no calls for empty diff, got grant=%v revoke=%v",
			svc.GrantCalls, svc.RevokeCalls)
	}
}

// TestApplyGrantDiff_Grant verifies that ToGrant entries call ExecuteGrant.
func TestApplyGrantDiff_Grant(t *testing.T) {
	svc := &applyFakeService{}
	gd := grant.GrantDiff{
		ToGrant: []grant.GrantEntry{
			{Privilege: "USAGE", RoleType: "ROLE", RoleName: "R1"},
			{Privilege: "OPERATE", RoleType: "ROLE", RoleName: "R2"},
		},
	}
	err := applyGrantDiff(context.Background(), svc, "DB", "S", "agent", gd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(svc.GrantCalls) != 2 {
		t.Fatalf("expected 2 grant calls, got %d: %v", len(svc.GrantCalls), svc.GrantCalls)
	}
}

// TestApplyGrantDiff_Revoke verifies that ToRevoke entries call ExecuteRevoke.
func TestApplyGrantDiff_Revoke(t *testing.T) {
	svc := &applyFakeService{}
	gd := grant.GrantDiff{
		ToRevoke: []grant.GrantEntry{
			{Privilege: "USAGE", RoleType: "ROLE", RoleName: "OLD_ROLE"},
		},
	}
	err := applyGrantDiff(context.Background(), svc, "DB", "S", "agent", gd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(svc.RevokeCalls) != 1 {
		t.Fatalf("expected 1 revoke call, got %d", len(svc.RevokeCalls))
	}
	if svc.RevokeCalls[0] != "USAGE:ROLE:OLD_ROLE" {
		t.Errorf("RevokeCalls[0] = %q", svc.RevokeCalls[0])
	}
}

// TestApplyGrantDiff_GrantError verifies that ExecuteGrant errors are collected
// and returned as a combined error.
func TestApplyGrantDiff_GrantError(t *testing.T) {
	svc := &applyFakeService{GrantErr: fmt.Errorf("permission denied")}
	gd := grant.GrantDiff{
		ToGrant: []grant.GrantEntry{
			{Privilege: "USAGE", RoleType: "ROLE", RoleName: "R1"},
		},
	}
	err := applyGrantDiff(context.Background(), svc, "DB", "S", "agent", gd)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "USAGE") {
		t.Errorf("error = %q, expected to mention privilege", err.Error())
	}
}

// --- confirm tests ---

// TestConfirm_Yes verifies that "y" input returns true.
func TestConfirm_Yes(t *testing.T) {
	if !confirm("Continue?", strings.NewReader("y\n")) {
		t.Error("expected true for 'y' input")
	}
}

// TestConfirm_YesFull verifies that "yes" input returns true.
func TestConfirm_YesFull(t *testing.T) {
	if !confirm("Continue?", strings.NewReader("yes\n")) {
		t.Error("expected true for 'yes' input")
	}
}

// TestConfirm_No verifies that "n" input returns false.
func TestConfirm_No(t *testing.T) {
	if confirm("Continue?", strings.NewReader("n\n")) {
		t.Error("expected false for 'n' input")
	}
}

// TestConfirm_DefaultNo verifies that empty (Enter) input returns false.
func TestConfirm_DefaultNo(t *testing.T) {
	if confirm("Continue?", strings.NewReader("\n")) {
		t.Error("expected false for empty input (default no)")
	}
}

// TestConfirm_CaseInsensitive verifies that "Y" and "YES" are accepted.
func TestConfirm_CaseInsensitive(t *testing.T) {
	if !confirm("Continue?", strings.NewReader("Y\n")) {
		t.Error("expected true for 'Y' input")
	}
	if !confirm("Continue?", strings.NewReader("YES\n")) {
		t.Error("expected true for 'YES' input")
	}
}
