package cli

import (
	"bytes"
	"strings"
	"testing"

	"coragent/internal/agent"

	"gopkg.in/yaml.v3"
)

// encodeSpec is a test helper that encodes an AgentSpec through the export pipeline.
func encodeSpec(t *testing.T, spec agent.AgentSpec) string {
	t.Helper()
	var doc yaml.Node
	if err := doc.Encode(spec); err != nil {
		t.Fatal(err)
	}
	setLiteralStyleForMultiline(&doc)
	reorderExportKeys(&doc)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		t.Fatal(err)
	}
	enc.Close()
	return buf.String()
}

func TestExport_MultilineComment(t *testing.T) {
	output := encodeSpec(t, agent.AgentSpec{
		Name:    "test-agent",
		Comment: "line1\nline2\nline3",
	})
	if !strings.Contains(output, "comment: |") {
		t.Errorf("expected literal block style for multiline comment, got:\n%s", output)
	}
	if !strings.Contains(output, "  line1") {
		t.Errorf("expected indented multiline content, got:\n%s", output)
	}
}

func TestExport_SingleLineComment(t *testing.T) {
	output := encodeSpec(t, agent.AgentSpec{
		Name:    "test-agent",
		Comment: "simple comment",
	})
	if strings.Contains(output, "|") {
		t.Errorf("expected inline style for single-line comment, got:\n%s", output)
	}
	if !strings.Contains(output, "comment: simple comment") {
		t.Errorf("expected inline comment, got:\n%s", output)
	}
}

func TestExport_ToolSpecKeyOrder(t *testing.T) {
	spec := agent.AgentSpec{
		Name: "test-agent",
		Tools: []agent.Tool{
			{ToolSpec: map[string]any{
				"description": "A test tool",
				"type":        "cortex_analyst_text_to_sql",
				"name":        "my_tool",
			}},
		},
	}
	output := encodeSpec(t, spec)

	nameIdx := strings.Index(output, "name: my_tool")
	typeIdx := strings.Index(output, "type: cortex_analyst_text_to_sql")
	descIdx := strings.Index(output, "description: A test tool")

	if nameIdx == -1 || typeIdx == -1 || descIdx == -1 {
		t.Fatalf("missing expected keys in output:\n%s", output)
	}
	if nameIdx > typeIdx {
		t.Errorf("name should appear before type in tool_spec:\n%s", output)
	}
	if typeIdx > descIdx {
		t.Errorf("type should appear before description in tool_spec:\n%s", output)
	}
}

func TestExport_ToolResourcesSemanticViewFirst(t *testing.T) {
	spec := agent.AgentSpec{
		Name: "test-agent",
		ToolResources: agent.ToolResources{
			"my_tool": {
				"execution_environment": map[string]any{"type": "warehouse"},
				"semantic_view":         "DB.SCHEMA.VIEW",
			},
		},
	}
	output := encodeSpec(t, spec)

	svIdx := strings.Index(output, "semantic_view:")
	eeIdx := strings.Index(output, "execution_environment:")

	if svIdx == -1 || eeIdx == -1 {
		t.Fatalf("missing expected keys in output:\n%s", output)
	}
	if svIdx > eeIdx {
		t.Errorf("semantic_view should appear before execution_environment:\n%s", output)
	}
}

func TestExport_ToolResourcesSearchServiceFirst(t *testing.T) {
	spec := agent.AgentSpec{
		Name: "test-agent",
		ToolResources: agent.ToolResources{
			"search_tool": {
				"max_results":    4,
				"id_column":      "ID",
				"search_service": "DB.SCHEMA.SVC",
			},
		},
	}
	output := encodeSpec(t, spec)

	ssIdx := strings.Index(output, "search_service:")
	idIdx := strings.Index(output, "id_column:")
	mrIdx := strings.Index(output, "max_results:")

	if ssIdx == -1 || idIdx == -1 || mrIdx == -1 {
		t.Fatalf("missing expected keys in output:\n%s", output)
	}
	if ssIdx > idIdx || ssIdx > mrIdx {
		t.Errorf("search_service should appear first:\n%s", output)
	}
}
