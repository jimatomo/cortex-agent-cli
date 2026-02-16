package cli

import (
	"bytes"
	"strings"
	"testing"

	"coragent/internal/agent"

	"gopkg.in/yaml.v3"
)

func TestExport_MultilineComment(t *testing.T) {
	spec := agent.AgentSpec{
		Name:    "test-agent",
		Comment: "line1\nline2\nline3",
	}

	var doc yaml.Node
	if err := doc.Encode(spec); err != nil {
		t.Fatal(err)
	}
	setLiteralStyleForMultiline(&doc)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		t.Fatal(err)
	}
	enc.Close()

	output := buf.String()
	// Should use literal block style with "|"
	if !strings.Contains(output, "comment: |") {
		t.Errorf("expected literal block style for multiline comment, got:\n%s", output)
	}
	if !strings.Contains(output, "  line1") {
		t.Errorf("expected indented multiline content, got:\n%s", output)
	}
}

func TestExport_SingleLineComment(t *testing.T) {
	spec := agent.AgentSpec{
		Name:    "test-agent",
		Comment: "simple comment",
	}

	var doc yaml.Node
	if err := doc.Encode(spec); err != nil {
		t.Fatal(err)
	}
	setLiteralStyleForMultiline(&doc)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		t.Fatal(err)
	}
	enc.Close()

	output := buf.String()
	// Should NOT use literal block style
	if strings.Contains(output, "|") {
		t.Errorf("expected inline style for single-line comment, got:\n%s", output)
	}
	if !strings.Contains(output, "comment: simple comment") {
		t.Errorf("expected inline comment, got:\n%s", output)
	}
}
