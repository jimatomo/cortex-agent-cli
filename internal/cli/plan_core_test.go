package cli

import (
	"context"
	"fmt"
	"testing"

	"coragent/internal/agent"
	"coragent/internal/api"
	"coragent/internal/auth"
	"coragent/internal/diff"
)

// fakeAgentService implements api.AgentService and api.GrantService for tests.
// Populate Agents with the desired agent state; agents not in the map are
// treated as non-existent.
type fakeAgentService struct {
	Agents map[string]agent.AgentSpec // key: "db.schema.name"
	Grants map[string][]api.ShowGrantsRow
	// GetAgentErr, if non-nil, is returned by every GetAgent call.
	GetAgentErr error
	// ShowGrantsErr, if non-nil, is returned by every ShowGrants call.
	ShowGrantsErr error
}

func (f *fakeAgentService) agentKey(db, schema, name string) string {
	return fmt.Sprintf("%s.%s.%s", db, schema, name)
}

// AgentService methods

func (f *fakeAgentService) GetAgent(_ context.Context, db, schema, name string) (agent.AgentSpec, bool, error) {
	if f.GetAgentErr != nil {
		return agent.AgentSpec{}, false, f.GetAgentErr
	}
	spec, ok := f.Agents[f.agentKey(db, schema, name)]
	return spec, ok, nil
}

func (f *fakeAgentService) CreateAgent(_ context.Context, _, _ string, _ agent.AgentSpec) error {
	return nil
}

func (f *fakeAgentService) UpdateAgent(_ context.Context, _, _, _ string, _ any) error {
	return nil
}

func (f *fakeAgentService) DeleteAgent(_ context.Context, _, _, _ string) error {
	return nil
}

func (f *fakeAgentService) ListAgents(_ context.Context, _, _ string) ([]api.AgentListItem, error) {
	return nil, nil
}

func (f *fakeAgentService) DescribeAgent(_ context.Context, _, _, _ string) (api.DescribeResult, error) {
	return api.DescribeResult{}, nil
}

// GrantService methods

func (f *fakeAgentService) ShowGrants(_ context.Context, db, schema, name string) ([]api.ShowGrantsRow, error) {
	if f.ShowGrantsErr != nil {
		return nil, f.ShowGrantsErr
	}
	rows := f.Grants[f.agentKey(db, schema, name)]
	return rows, nil
}

func (f *fakeAgentService) ExecuteGrant(_ context.Context, _, _, _, _, _, _ string) error {
	return nil
}

func (f *fakeAgentService) ExecuteRevoke(_ context.Context, _, _, _, _, _, _ string) error {
	return nil
}

// testOpts returns minimal RootOptions for tests (database + schema required
// since SNOWFLAKE_DATABASE / SNOWFLAKE_SCHEMA env vars are not set in CI).
func testOpts() *RootOptions {
	return &RootOptions{
		Database: "TEST_DB",
		Schema:   "PUBLIC",
	}
}

func testCfg() auth.Config {
	return auth.Config{
		Database: "TEST_DB",
		Schema:   "PUBLIC",
	}
}

func makeSpec(name string) agent.ParsedAgent {
	return agent.ParsedAgent{
		Path: name + ".yaml",
		Spec: agent.AgentSpec{Name: name},
	}
}

// TestBuildPlanItems_Create verifies that a non-existent agent is classified
// as a create.
func TestBuildPlanItems_Create(t *testing.T) {
	svc := &fakeAgentService{Agents: map[string]agent.AgentSpec{}}
	specs := []agent.ParsedAgent{makeSpec("new-agent")}

	items, err := buildPlanItems(context.Background(), specs, testOpts(), testCfg(), svc, svc)
	if err != nil {
		t.Fatalf("buildPlanItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Exists {
		t.Error("expected Exists=false for new agent")
	}
	if len(items[0].Changes) != 0 {
		t.Errorf("expected 0 changes for create, got %d", len(items[0].Changes))
	}
}

// TestBuildPlanItems_NoChange verifies that an identical agent is classified
// as unchanged.
func TestBuildPlanItems_NoChange(t *testing.T) {
	spec := agent.AgentSpec{Name: "existing", Comment: "hello"}
	key := "TEST_DB.PUBLIC.existing"
	svc := &fakeAgentService{
		Agents: map[string]agent.AgentSpec{key: spec},
	}
	specs := []agent.ParsedAgent{{Path: "a.yaml", Spec: spec}}

	items, err := buildPlanItems(context.Background(), specs, testOpts(), testCfg(), svc, svc)
	if err != nil {
		t.Fatalf("buildPlanItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if !items[0].Exists {
		t.Error("expected Exists=true for existing agent")
	}
	if diff.HasChanges(items[0].Changes) {
		t.Errorf("expected no changes for identical agent, got %v", items[0].Changes)
	}
	if items[0].GrantDiff.HasChanges() {
		t.Error("expected no grant changes")
	}
}

// TestBuildPlanItems_Update verifies that a changed agent is classified as
// an update with non-empty Changes.
func TestBuildPlanItems_Update(t *testing.T) {
	remote := agent.AgentSpec{Name: "agent", Comment: "old"}
	local := agent.AgentSpec{Name: "agent", Comment: "new"}
	key := "TEST_DB.PUBLIC.agent"
	svc := &fakeAgentService{
		Agents: map[string]agent.AgentSpec{key: remote},
	}
	specs := []agent.ParsedAgent{{Path: "a.yaml", Spec: local}}

	items, err := buildPlanItems(context.Background(), specs, testOpts(), testCfg(), svc, svc)
	if err != nil {
		t.Fatalf("buildPlanItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if !items[0].Exists {
		t.Error("expected Exists=true for existing agent")
	}
	if !diff.HasChanges(items[0].Changes) {
		t.Error("expected changes for updated agent")
	}
}

// TestBuildPlanItems_Multiple verifies handling of multiple specs at once.
func TestBuildPlanItems_Multiple(t *testing.T) {
	existing := agent.AgentSpec{Name: "existing", Comment: "same"}
	key := "TEST_DB.PUBLIC.existing"
	svc := &fakeAgentService{
		Agents: map[string]agent.AgentSpec{key: existing},
	}
	specs := []agent.ParsedAgent{
		{Path: "new.yaml", Spec: agent.AgentSpec{Name: "new-agent"}},
		{Path: "existing.yaml", Spec: existing},
	}

	items, err := buildPlanItems(context.Background(), specs, testOpts(), testCfg(), svc, svc)
	if err != nil {
		t.Fatalf("buildPlanItems: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Exists {
		t.Error("expected items[0] (new-agent) to be a create")
	}
	if !items[1].Exists {
		t.Error("expected items[1] (existing) to already exist")
	}
}

// TestBuildPlanItems_GetAgentError verifies that GetAgent errors are propagated.
func TestBuildPlanItems_GetAgentError(t *testing.T) {
	svc := &fakeAgentService{
		GetAgentErr: fmt.Errorf("api unavailable"),
	}
	specs := []agent.ParsedAgent{makeSpec("agent")}

	_, err := buildPlanItems(context.Background(), specs, testOpts(), testCfg(), svc, svc)
	if err == nil {
		t.Fatal("expected error from GetAgent failure")
	}
}

// TestBuildPlanItems_ShowGrantsError verifies that ShowGrants errors are
// propagated for existing agents.
func TestBuildPlanItems_ShowGrantsError(t *testing.T) {
	spec := agent.AgentSpec{Name: "agent"}
	key := "TEST_DB.PUBLIC.agent"
	svc := &fakeAgentService{
		Agents:        map[string]agent.AgentSpec{key: spec},
		ShowGrantsErr: fmt.Errorf("grants unavailable"),
	}
	specs := []agent.ParsedAgent{{Path: "a.yaml", Spec: spec}}

	_, err := buildPlanItems(context.Background(), specs, testOpts(), testCfg(), svc, svc)
	if err == nil {
		t.Fatal("expected error from ShowGrants failure")
	}
}
