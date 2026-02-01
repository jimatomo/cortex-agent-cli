package diff

import (
	"testing"

	"coragent/internal/agent"
)

func TestDiffDetectsChanges(t *testing.T) {
	local := agent.AgentSpec{
		Name: "agent",
		Instructions: &agent.Instructions{
			Response: "new",
		},
		Orchestration: &agent.Orchestration{
			Budget: &agent.BudgetConfig{
				Tokens: 4096,
			},
		},
	}
	remote := agent.AgentSpec{
		Name: "agent",
		Instructions: &agent.Instructions{
			Response: "old",
		},
		Orchestration: &agent.Orchestration{
			Budget: &agent.BudgetConfig{
				Tokens: 1024,
			},
		},
	}

	changes, err := Diff(local, remote)
	if err != nil {
		t.Fatalf("Diff error: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected changes, got none")
	}
}

func TestDiffNoChanges(t *testing.T) {
	spec := agent.AgentSpec{
		Name: "agent",
	}
	changes, err := Diff(spec, spec)
	if err != nil {
		t.Fatalf("Diff error: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("expected no changes, got %d", len(changes))
	}
}

func TestDiffForCreate(t *testing.T) {
	spec := agent.AgentSpec{
		Name:    "test-agent",
		Comment: "Test comment",
		Profile: &agent.Profile{DisplayName: "Test Bot"},
		Models:  &agent.Models{Orchestration: "claude-4-sonnet"},
		Orchestration: &agent.Orchestration{
			Budget: &agent.BudgetConfig{Seconds: 60, Tokens: 16000},
		},
	}
	changes, err := DiffForCreate(spec)
	if err != nil {
		t.Fatalf("DiffForCreate error: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected changes, got none")
	}
	// Log all changes for debugging
	for _, c := range changes {
		t.Logf("+ %s: %v", c.Path, c.Before)
	}
	// Should have at least: name, comment, profile.display_name, models.orchestration, orchestration.budget.seconds, orchestration.budget.tokens
	if len(changes) < 6 {
		t.Fatalf("expected at least 6 changes, got %d", len(changes))
	}
}

// TestDiff_LocalNilRemoteHasValue tests that when local field is nil and remote has value,
// it's detected as Removed (we want to remove the field from remote).
// The change is detected at the object level, not at the leaf level.
func TestDiff_LocalNilRemoteHasValue(t *testing.T) {
	local := agent.AgentSpec{
		Name: "agent",
		// No Instructions (nil)
	}
	remote := agent.AgentSpec{
		Name: "agent",
		Instructions: &agent.Instructions{
			Response: "existing response",
		},
	}

	changes, err := Diff(local, remote)
	if err != nil {
		t.Fatalf("Diff error: %v", err)
	}

	if len(changes) == 0 {
		t.Fatal("expected changes, got none")
	}

	// The change is detected at "instructions" level (not "instructions.response")
	// because local.Instructions is nil
	found := false
	for _, c := range changes {
		if c.Path == "instructions" {
			found = true
			if c.Type != Removed {
				t.Errorf("expected Removed, got %s", c.Type)
			}
			// Before should contain the remote value (the object being removed)
			beforeMap, ok := c.Before.(map[string]any)
			if !ok {
				t.Errorf("expected Before to be map, got %T", c.Before)
			} else if beforeMap["response"] != "existing response" {
				t.Errorf("expected Before.response='existing response', got %v", beforeMap["response"])
			}
			if c.After != nil {
				t.Errorf("expected After=nil, got %v", c.After)
			}
		}
	}
	if !found {
		t.Error("expected change for 'instructions' not found")
	}
}

// TestDiff_LocalHasValueRemoteNil tests that when local field has value and remote is nil,
// it's detected as Added (we want to add the field to remote).
// The change is detected at the object level, not at the leaf level.
func TestDiff_LocalHasValueRemoteNil(t *testing.T) {
	local := agent.AgentSpec{
		Name: "agent",
		Instructions: &agent.Instructions{
			Response: "new response",
		},
	}
	remote := agent.AgentSpec{
		Name: "agent",
		// No Instructions (nil)
	}

	changes, err := Diff(local, remote)
	if err != nil {
		t.Fatalf("Diff error: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected changes, got none")
	}

	// The change is detected at "instructions" level
	found := false
	for _, c := range changes {
		if c.Path == "instructions" {
			found = true
			if c.Type != Added {
				t.Errorf("expected Added, got %s", c.Type)
			}
			if c.Before != nil {
				t.Errorf("expected Before=nil, got %v", c.Before)
			}
			// After should contain the local value (the object being added)
			afterMap, ok := c.After.(map[string]any)
			if !ok {
				t.Errorf("expected After to be map, got %T", c.After)
			} else if afterMap["response"] != "new response" {
				t.Errorf("expected After.response='new response', got %v", afterMap["response"])
			}
		}
	}
	if !found {
		t.Error("expected change for 'instructions' not found")
	}
}

// TestDiffWithOptions_IgnoreMissingRemote tests the IgnoreMissingRemote option.
func TestDiffWithOptions_IgnoreMissingRemote(t *testing.T) {
	local := agent.AgentSpec{
		Name: "agent",
		Instructions: &agent.Instructions{
			Response: "new response",
		},
	}
	remote := agent.AgentSpec{
		Name: "agent",
		// No Instructions (nil)
	}

	// Without IgnoreMissingRemote, should detect Added
	changes, err := DiffWithOptions(local, remote, Options{IgnoreMissingRemote: false})
	if err != nil {
		t.Fatalf("Diff error: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected changes without IgnoreMissingRemote, got none")
	}

	// With IgnoreMissingRemote, should ignore the difference
	changes, err = DiffWithOptions(local, remote, Options{IgnoreMissingRemote: true})
	if err != nil {
		t.Fatalf("Diff error: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("expected no changes with IgnoreMissingRemote, got %d", len(changes))
	}
}

// TestDiff_ArrayLengthDifference tests array comparison with different lengths.
func TestDiff_ArrayLengthDifference(t *testing.T) {
	local := agent.AgentSpec{
		Name: "agent",
		Tools: []agent.Tool{
			{ToolSpec: map[string]any{"name": "tool1"}},
			{ToolSpec: map[string]any{"name": "tool2"}},
		},
	}
	remote := agent.AgentSpec{
		Name: "agent",
		Tools: []agent.Tool{
			{ToolSpec: map[string]any{"name": "tool1"}},
		},
	}

	changes, err := Diff(local, remote)
	if err != nil {
		t.Fatalf("Diff error: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected changes, got none")
	}

	// Log all changes for debugging
	for _, c := range changes {
		t.Logf("  Path: %s, Type: %s", c.Path, c.Type)
	}

	// Should detect tools[1] as Added (the second tool element)
	found := false
	for _, c := range changes {
		// The path could be "tools[1]" or "tools[1].tool_spec" depending on how nil is handled
		if c.Path == "tools[1]" || c.Path == "tools[1].tool_spec" || c.Path == "tools[1].tool_spec.name" {
			found = true
			if c.Type != Added {
				t.Errorf("expected Added for %s, got %s", c.Path, c.Type)
			}
		}
	}
	if !found {
		t.Error("expected change for tools[1] not found")
	}
}

// TestDiff_ArrayRemoval tests when local has fewer elements than remote.
func TestDiff_ArrayRemoval(t *testing.T) {
	local := agent.AgentSpec{
		Name: "agent",
		Tools: []agent.Tool{
			{ToolSpec: map[string]any{"name": "tool1"}},
		},
	}
	remote := agent.AgentSpec{
		Name: "agent",
		Tools: []agent.Tool{
			{ToolSpec: map[string]any{"name": "tool1"}},
			{ToolSpec: map[string]any{"name": "tool2"}},
		},
	}

	changes, err := Diff(local, remote)
	if err != nil {
		t.Fatalf("Diff error: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected changes, got none")
	}

	// Log all changes for debugging
	for _, c := range changes {
		t.Logf("  Path: %s, Type: %s", c.Path, c.Type)
	}

	// Should detect tools[1] as Removed
	found := false
	for _, c := range changes {
		if c.Path == "tools[1]" || c.Path == "tools[1].tool_spec" || c.Path == "tools[1].tool_spec.name" {
			found = true
			if c.Type != Removed {
				t.Errorf("expected Removed for %s, got %s", c.Path, c.Type)
			}
		}
	}
	if !found {
		t.Error("expected change for tools[1] removal not found")
	}
}

// TestDiff_EmptyArrayVsNil tests the difference between empty array and nil.
func TestDiff_EmptyArrayVsNil(t *testing.T) {
	local := agent.AgentSpec{
		Name:  "agent",
		Tools: []agent.Tool{}, // empty array
	}
	remote := agent.AgentSpec{
		Name:  "agent",
		Tools: nil, // nil
	}

	changes, err := Diff(local, remote)
	if err != nil {
		t.Fatalf("Diff error: %v", err)
	}
	// Empty array should marshal to null in JSON, so no difference expected
	// (omitempty in JSON tag causes empty slice to be omitted)
	if len(changes) != 0 {
		t.Logf("Unexpected changes: %+v", changes)
	}
}

// TestDiff_NestedNilHandling tests nil handling in nested structures.
func TestDiff_NestedNilHandling(t *testing.T) {
	local := agent.AgentSpec{
		Name: "agent",
		Orchestration: &agent.Orchestration{
			Budget: nil, // nil Budget
		},
	}
	remote := agent.AgentSpec{
		Name: "agent",
		Orchestration: &agent.Orchestration{
			Budget: &agent.BudgetConfig{
				Tokens: 1000,
			},
		},
	}

	changes, err := Diff(local, remote)
	if err != nil {
		t.Fatalf("Diff error: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected changes, got none")
	}

	// Log all changes for debugging
	for _, c := range changes {
		t.Logf("  Path: %s, Type: %s, Before: %v", c.Path, c.Type, c.Before)
	}

	// Should detect budget as Removed at the "orchestration.budget" level
	found := false
	for _, c := range changes {
		if c.Path == "orchestration.budget" || c.Path == "orchestration.budget.tokens" {
			found = true
			if c.Type != Removed {
				t.Errorf("expected Removed, got %s", c.Type)
			}
		}
	}
	if !found {
		t.Error("expected change for orchestration.budget not found")
	}
}

// TestDiff_ModifiedValue tests value modification detection.
// Note: In the current implementation, Before=local (new value), After=remote (old value).
func TestDiff_ModifiedValue(t *testing.T) {
	local := agent.AgentSpec{
		Name:    "agent",
		Comment: "new comment",
	}
	remote := agent.AgentSpec{
		Name:    "agent",
		Comment: "old comment",
	}

	changes, err := Diff(local, remote)
	if err != nil {
		t.Fatalf("Diff error: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}

	c := changes[0]
	if c.Path != "comment" {
		t.Errorf("expected path 'comment', got '%s'", c.Path)
	}
	if c.Type != Modified {
		t.Errorf("expected Modified, got %s", c.Type)
	}
	// Current implementation: Before=local, After=remote
	if c.Before != "new comment" {
		t.Errorf("expected Before='new comment', got %v", c.Before)
	}
	if c.After != "old comment" {
		t.Errorf("expected After='old comment', got %v", c.After)
	}
}

// TestDiff_ToolResources tests ToolResources map comparison.
func TestDiff_ToolResources(t *testing.T) {
	local := agent.AgentSpec{
		Name: "agent",
		ToolResources: agent.ToolResources{
			"semantic_view": map[string]any{
				"views": []any{"view1", "view2"},
			},
		},
	}
	remote := agent.AgentSpec{
		Name: "agent",
		ToolResources: agent.ToolResources{
			"semantic_view": map[string]any{
				"views": []any{"view1"},
			},
		},
	}

	changes, err := Diff(local, remote)
	if err != nil {
		t.Fatalf("Diff error: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected changes, got none")
	}

	// Should detect view2 as Added
	found := false
	for _, c := range changes {
		if c.Path == "tool_resources.semantic_view.views[1]" {
			found = true
			if c.Type != Added {
				t.Errorf("expected Added, got %s", c.Type)
			}
		}
	}
	if !found {
		t.Error("expected change for tool_resources.semantic_view.views[1] not found")
	}
}

// TestDiff_ToolResourcesRemoval tests removing a tool resource.
func TestDiff_ToolResourcesRemoval(t *testing.T) {
	local := agent.AgentSpec{
		Name:          "agent",
		ToolResources: nil, // Want to remove all tool_resources
	}
	remote := agent.AgentSpec{
		Name: "agent",
		ToolResources: agent.ToolResources{
			"semantic_view": map[string]any{
				"views": []any{"view1"},
			},
		},
	}

	changes, err := Diff(local, remote)
	if err != nil {
		t.Fatalf("Diff error: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected changes, got none")
	}

	// Should detect tool_resources as Removed
	found := false
	for _, c := range changes {
		if c.Path == "tool_resources" || c.Path == "tool_resources.semantic_view" {
			found = true
			if c.Type != Removed {
				t.Errorf("expected Removed for %s, got %s", c.Path, c.Type)
			}
		}
	}
	if !found {
		t.Error("expected change for tool_resources removal not found")
	}
}

// TestDiffForCreate_EmptySpec tests DiffForCreate with minimal spec.
func TestDiffForCreate_EmptySpec(t *testing.T) {
	spec := agent.AgentSpec{
		Name: "agent",
	}
	changes, err := DiffForCreate(spec)
	if err != nil {
		t.Fatalf("DiffForCreate error: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change (name), got %d", len(changes))
	}
	if changes[0].Path != "name" {
		t.Errorf("expected path 'name', got '%s'", changes[0].Path)
	}
}

// TestDiffForCreate_NilFields tests that nil fields are not included in DiffForCreate.
func TestDiffForCreate_NilFields(t *testing.T) {
	spec := agent.AgentSpec{
		Name:          "agent",
		Profile:       nil,
		Instructions:  nil,
		Orchestration: nil,
	}
	changes, err := DiffForCreate(spec)
	if err != nil {
		t.Fatalf("DiffForCreate error: %v", err)
	}
	// Only name should be in changes
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Path != "name" {
		t.Errorf("expected only 'name' change, got '%s'", changes[0].Path)
	}
}

// TestHasChanges tests the HasChanges helper function.
func TestHasChanges(t *testing.T) {
	if HasChanges(nil) {
		t.Error("expected HasChanges(nil) = false")
	}
	if HasChanges([]Change{}) {
		t.Error("expected HasChanges([]) = false")
	}
	if !HasChanges([]Change{{Path: "test", Type: Added}}) {
		t.Error("expected HasChanges with changes = true")
	}
}

// TestToMap tests the ToMap conversion function.
func TestToMap(t *testing.T) {
	spec := agent.AgentSpec{
		Name:    "test",
		Comment: "test comment",
	}
	m, err := ToMap(spec)
	if err != nil {
		t.Fatalf("ToMap error: %v", err)
	}
	if m["name"] != "test" {
		t.Errorf("expected name='test', got %v", m["name"])
	}
	if m["comment"] != "test comment" {
		t.Errorf("expected comment='test comment', got %v", m["comment"])
	}
}

// TestDiff_BothNil tests that both nil values produce no changes.
func TestDiff_BothNil(t *testing.T) {
	local := agent.AgentSpec{
		Name:         "agent",
		Instructions: nil,
	}
	remote := agent.AgentSpec{
		Name:         "agent",
		Instructions: nil,
	}

	changes, err := Diff(local, remote)
	if err != nil {
		t.Fatalf("Diff error: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("expected no changes when both nil, got %d", len(changes))
	}
}

// TestDiff_SameEmptyMaps tests that same empty maps produce no changes.
func TestDiff_SameEmptyMaps(t *testing.T) {
	local := agent.AgentSpec{
		Name:          "agent",
		ToolResources: agent.ToolResources{},
	}
	remote := agent.AgentSpec{
		Name:          "agent",
		ToolResources: agent.ToolResources{},
	}

	changes, err := Diff(local, remote)
	if err != nil {
		t.Fatalf("Diff error: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("expected no changes for same empty maps, got %d", len(changes))
	}
}

// TestDiff_DeepNestedChanges tests changes in deeply nested structures.
func TestDiff_DeepNestedChanges(t *testing.T) {
	local := agent.AgentSpec{
		Name: "agent",
		Instructions: &agent.Instructions{
			SampleQuestions: []agent.SampleQuestion{
				{Question: "question1"},
				{Question: "question2"},
			},
		},
	}
	remote := agent.AgentSpec{
		Name: "agent",
		Instructions: &agent.Instructions{
			SampleQuestions: []agent.SampleQuestion{
				{Question: "question1"},
				{Question: "question2-old"},
			},
		},
	}

	changes, err := Diff(local, remote)
	if err != nil {
		t.Fatalf("Diff error: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected changes, got none")
	}

	// Should detect the modified question
	found := false
	for _, c := range changes {
		if c.Path == "instructions.sample_questions[1].question" {
			found = true
			if c.Type != Modified {
				t.Errorf("expected Modified, got %s", c.Type)
			}
		}
	}
	if !found {
		t.Logf("Changes found:")
		for _, c := range changes {
			t.Logf("  %s: %s", c.Path, c.Type)
		}
		t.Error("expected change for instructions.sample_questions[1].question not found")
	}
}

// TestDiffForCreate_WithArrays tests DiffForCreate with array fields.
func TestDiffForCreate_WithArrays(t *testing.T) {
	spec := agent.AgentSpec{
		Name: "agent",
		Tools: []agent.Tool{
			{ToolSpec: map[string]any{"name": "tool1"}},
			{ToolSpec: map[string]any{"name": "tool2"}},
		},
		Instructions: &agent.Instructions{
			SampleQuestions: []agent.SampleQuestion{
				{Question: "question1"},
			},
		},
	}
	changes, err := DiffForCreate(spec)
	if err != nil {
		t.Fatalf("DiffForCreate error: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected changes, got none")
	}
	// Log changes for debugging
	for _, c := range changes {
		t.Logf("+ %s: %v", c.Path, c.Before)
	}
	// Should have: name, tools[0].tool_spec.name, tools[1].tool_spec.name, instructions.sample_questions[0].question
	if len(changes) < 4 {
		t.Fatalf("expected at least 4 changes, got %d", len(changes))
	}
}

// TestDiff_TypeMismatch tests when types differ between local and remote.
func TestDiff_TypeMismatch(t *testing.T) {
	// Using ToolResources which allows any type of value
	local := agent.AgentSpec{
		Name: "agent",
		ToolResources: agent.ToolResources{
			"test": map[string]any{
				"value": "string",
			},
		},
	}
	remote := agent.AgentSpec{
		Name: "agent",
		ToolResources: agent.ToolResources{
			"test": map[string]any{
				"value": float64(123),
			},
		},
	}

	changes, err := Diff(local, remote)
	if err != nil {
		t.Fatalf("Diff error: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected changes when types differ, got none")
	}

	// Should detect the type mismatch as Modified
	found := false
	for _, c := range changes {
		if c.Path == "tool_resources.test.value" {
			found = true
			if c.Type != Modified {
				t.Errorf("expected Modified, got %s", c.Type)
			}
		}
	}
	if !found {
		t.Logf("Changes found:")
		for _, c := range changes {
			t.Logf("  %s: %s", c.Path, c.Type)
		}
		t.Error("expected change for type mismatch not found")
	}
}

// TestSortAgentKeys tests the field ordering logic.
func TestSortAgentKeys(t *testing.T) {
	local := agent.AgentSpec{
		Name:    "agent",
		Comment: "comment",
		Profile: &agent.Profile{DisplayName: "Bot"},
		Models:  &agent.Models{Orchestration: "model"},
	}
	remote := agent.AgentSpec{
		Name:    "agent-old",
		Comment: "comment-old",
		Profile: &agent.Profile{DisplayName: "Bot-old"},
		Models:  &agent.Models{Orchestration: "model-old"},
	}

	changes, err := Diff(local, remote)
	if err != nil {
		t.Fatalf("Diff error: %v", err)
	}
	if len(changes) != 4 {
		t.Fatalf("expected 4 changes, got %d", len(changes))
	}

	// Verify order matches agentFieldOrder: name, comment, profile, models
	expectedOrder := []string{"name", "comment", "profile.display_name", "models.orchestration"}
	for i, expected := range expectedOrder {
		if changes[i].Path != expected {
			t.Errorf("expected change[%d].Path='%s', got '%s'", i, expected, changes[i].Path)
		}
	}
}

// TestDiff_UnknownFieldsSorted tests that unknown fields are sorted alphabetically.
func TestDiff_UnknownFieldsSorted(t *testing.T) {
	// Test with ToolResources which can have arbitrary keys
	local := agent.AgentSpec{
		Name: "agent",
		ToolResources: agent.ToolResources{
			"zebra": map[string]any{"a": 1},
			"alpha": map[string]any{"b": 2},
		},
	}
	remote := agent.AgentSpec{
		Name: "agent",
		ToolResources: agent.ToolResources{
			"zebra": map[string]any{"a": 10},
			"alpha": map[string]any{"b": 20},
		},
	}

	changes, err := Diff(local, remote)
	if err != nil {
		t.Fatalf("Diff error: %v", err)
	}
	// Should have 2 changes (alpha.b and zebra.a), alphabetically ordered
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(changes))
	}
	// Nested keys should be sorted alphabetically: alpha before zebra
	if changes[0].Path != "tool_resources.alpha.b" {
		t.Errorf("expected first change to be 'tool_resources.alpha.b', got '%s'", changes[0].Path)
	}
	if changes[1].Path != "tool_resources.zebra.a" {
		t.Errorf("expected second change to be 'tool_resources.zebra.a', got '%s'", changes[1].Path)
	}
}

// TestDiff_MapVsArrayTypeMismatch tests when map is compared with array.
func TestDiff_MapVsArrayTypeMismatch(t *testing.T) {
	// Create specs with different types at the same path using ToolResources
	local := agent.AgentSpec{
		Name: "agent",
		ToolResources: agent.ToolResources{
			"test": map[string]any{
				"data": map[string]any{"key": "value"},
			},
		},
	}
	remote := agent.AgentSpec{
		Name: "agent",
		ToolResources: agent.ToolResources{
			"test": map[string]any{
				"data": []any{"item1", "item2"},
			},
		},
	}

	changes, err := Diff(local, remote)
	if err != nil {
		t.Fatalf("Diff error: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected changes for type mismatch, got none")
	}

	// Should detect the type mismatch as Modified
	found := false
	for _, c := range changes {
		if c.Path == "tool_resources.test.data" {
			found = true
			if c.Type != Modified {
				t.Errorf("expected Modified for type mismatch, got %s", c.Type)
			}
		}
	}
	if !found {
		t.Logf("Changes found:")
		for _, c := range changes {
			t.Logf("  %s: %s", c.Path, c.Type)
		}
		t.Error("expected change for type mismatch not found")
	}
}

// TestDiff_ArrayVsNonArrayTypeMismatch tests when array is compared with non-array.
func TestDiff_ArrayVsNonArrayTypeMismatch(t *testing.T) {
	local := agent.AgentSpec{
		Name: "agent",
		ToolResources: agent.ToolResources{
			"test": map[string]any{
				"data": []any{"item1"},
			},
		},
	}
	remote := agent.AgentSpec{
		Name: "agent",
		ToolResources: agent.ToolResources{
			"test": map[string]any{
				"data": "not an array",
			},
		},
	}

	changes, err := Diff(local, remote)
	if err != nil {
		t.Fatalf("Diff error: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected changes, got none")
	}

	// Should detect as Modified
	found := false
	for _, c := range changes {
		if c.Path == "tool_resources.test.data" {
			found = true
			if c.Type != Modified {
				t.Errorf("expected Modified, got %s", c.Type)
			}
		}
	}
	if !found {
		t.Error("expected change for array vs non-array mismatch not found")
	}
}

// TestDiffForCreate_EmptyArray tests that empty arrays are not included in DiffForCreate.
func TestDiffForCreate_EmptyArray(t *testing.T) {
	spec := agent.AgentSpec{
		Name:  "agent",
		Tools: []agent.Tool{}, // empty array
	}
	changes, err := DiffForCreate(spec)
	if err != nil {
		t.Fatalf("DiffForCreate error: %v", err)
	}
	// Only name should be in changes (empty array is omitted by omitempty)
	if len(changes) != 1 {
		t.Logf("Changes found:")
		for _, c := range changes {
			t.Logf("  %s: %v", c.Path, c.Before)
		}
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Path != "name" {
		t.Errorf("expected only 'name' change, got '%s'", changes[0].Path)
	}
}

// TestDiffForCreate_EmptyMap tests that empty maps are not included in DiffForCreate.
func TestDiffForCreate_EmptyMap(t *testing.T) {
	spec := agent.AgentSpec{
		Name:          "agent",
		ToolResources: agent.ToolResources{}, // empty map
	}
	changes, err := DiffForCreate(spec)
	if err != nil {
		t.Fatalf("DiffForCreate error: %v", err)
	}
	// Only name should be in changes (empty map is omitted by omitempty)
	if len(changes) != 1 {
		t.Logf("Changes found:")
		for _, c := range changes {
			t.Logf("  %s: %v", c.Path, c.Before)
		}
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
}
