package agent

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestResolveVars_WithEnv(t *testing.T) {
	vars := VarsConfig{
		"dev":     {"DB": "DEV_DB", "WH": "DEV_WH"},
		"default": {"DB": "MY_DB", "WH": "COMPUTE_WH"},
	}

	resolved, err := resolveVars(vars, "dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved["DB"] != "DEV_DB" {
		t.Errorf("expected DB=DEV_DB, got %s", resolved["DB"])
	}
	if resolved["WH"] != "DEV_WH" {
		t.Errorf("expected WH=DEV_WH, got %s", resolved["WH"])
	}
}

func TestResolveVars_FallbackToDefault(t *testing.T) {
	vars := VarsConfig{
		"dev":     {"DB": "DEV_DB"},
		"default": {"DB": "MY_DB", "WH": "COMPUTE_WH"},
	}

	resolved, err := resolveVars(vars, "dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved["DB"] != "DEV_DB" {
		t.Errorf("expected DB=DEV_DB, got %s", resolved["DB"])
	}
	if resolved["WH"] != "COMPUTE_WH" {
		t.Errorf("expected WH=COMPUTE_WH (fallback to default), got %s", resolved["WH"])
	}
}

func TestResolveVars_DefaultOnly(t *testing.T) {
	vars := VarsConfig{
		"dev":     {"DB": "DEV_DB"},
		"default": {"DB": "MY_DB", "WH": "COMPUTE_WH"},
	}

	resolved, err := resolveVars(vars, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved["DB"] != "MY_DB" {
		t.Errorf("expected DB=MY_DB, got %s", resolved["DB"])
	}
	if resolved["WH"] != "COMPUTE_WH" {
		t.Errorf("expected WH=COMPUTE_WH, got %s", resolved["WH"])
	}
}

func TestResolveVars_NoDefaultNoEnv_Error(t *testing.T) {
	vars := VarsConfig{
		"dev": {"DB": "DEV_DB"},
	}

	_, err := resolveVars(vars, "")
	if err == nil {
		t.Fatal("expected error when no default and no --env, got nil")
	}
}

func TestResolveVars_UnknownEnv_FallbackDefault(t *testing.T) {
	vars := VarsConfig{
		"default": {"DB": "MY_DB", "WH": "COMPUTE_WH"},
	}

	resolved, err := resolveVars(vars, "staging")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved["DB"] != "MY_DB" {
		t.Errorf("expected DB=MY_DB (fallback to default), got %s", resolved["DB"])
	}
	if resolved["WH"] != "COMPUTE_WH" {
		t.Errorf("expected WH=COMPUTE_WH (fallback to default), got %s", resolved["WH"])
	}
}

func TestResolveVars_EmptyVars(t *testing.T) {
	resolved, err := resolveVars(nil, "dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != nil {
		t.Errorf("expected nil, got %v", resolved)
	}
}

func mustParseNode(t *testing.T, input string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(input), &doc); err != nil {
		t.Fatalf("failed to parse YAML: %v", err)
	}
	return &doc
}

func mustEncodeNode(t *testing.T, node *yaml.Node) string {
	t.Helper()
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(node); err != nil {
		t.Fatalf("failed to encode YAML node: %v", err)
	}
	return buf.String()
}

func TestSubstituteVars_FullValue(t *testing.T) {
	input := `name: ${ vars.DB }`
	node := mustParseNode(t, input)
	resolved := map[string]string{"DB": "MY_DATABASE"}

	if err := substituteVars(node, resolved); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := mustEncodeNode(t, node)
	if !strings.Contains(output, "MY_DATABASE") {
		t.Errorf("expected MY_DATABASE in output, got:\n%s", output)
	}
}

func TestSubstituteVars_PartialValue(t *testing.T) {
	input := `name: prefix_${ vars.DB }_suffix`
	node := mustParseNode(t, input)
	resolved := map[string]string{"DB": "MY_DB"}

	if err := substituteVars(node, resolved); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := mustEncodeNode(t, node)
	if !strings.Contains(output, "prefix_MY_DB_suffix") {
		t.Errorf("expected prefix_MY_DB_suffix in output, got:\n%s", output)
	}
}

func TestSubstituteVars_MultipleRefs(t *testing.T) {
	input := `conn: ${ vars.DB }.${ vars.SCHEMA }`
	node := mustParseNode(t, input)
	resolved := map[string]string{"DB": "MY_DB", "SCHEMA": "PUBLIC"}

	if err := substituteVars(node, resolved); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := mustEncodeNode(t, node)
	if !strings.Contains(output, "MY_DB.PUBLIC") {
		t.Errorf("expected MY_DB.PUBLIC in output, got:\n%s", output)
	}
}

func TestSubstituteVars_UndefinedVar_Error(t *testing.T) {
	input := `name: ${ vars.UNDEFINED }`
	node := mustParseNode(t, input)
	resolved := map[string]string{"DB": "MY_DB"}

	err := substituteVars(node, resolved)
	if err == nil {
		t.Fatal("expected error for undefined variable, got nil")
	}
	if !strings.Contains(err.Error(), "UNDEFINED") {
		t.Errorf("expected error to mention UNDEFINED, got: %s", err.Error())
	}
}

func TestSubstituteVars_NoVarsNoop(t *testing.T) {
	input := `name: plain_value`
	node := mustParseNode(t, input)

	if err := substituteVars(node, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := mustEncodeNode(t, node)
	if !strings.Contains(output, "plain_value") {
		t.Errorf("expected plain_value in output, got:\n%s", output)
	}
}

func TestSubstituteVars_NestedValue(t *testing.T) {
	input := `
deploy:
  database: ${ vars.DB }
  nested:
    warehouse: ${ vars.WH }
`
	node := mustParseNode(t, input)
	resolved := map[string]string{"DB": "MY_DB", "WH": "MY_WH"}

	if err := substituteVars(node, resolved); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := mustEncodeNode(t, node)
	if !strings.Contains(output, "MY_DB") || !strings.Contains(output, "MY_WH") {
		t.Errorf("expected substitution in nested values, got:\n%s", output)
	}
}

func TestSubstituteVars_InSequence(t *testing.T) {
	input := `
items:
  - ${ vars.ITEM1 }
  - ${ vars.ITEM2 }
`
	node := mustParseNode(t, input)
	resolved := map[string]string{"ITEM1": "first", "ITEM2": "second"}

	if err := substituteVars(node, resolved); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := mustEncodeNode(t, node)
	if !strings.Contains(output, "first") || !strings.Contains(output, "second") {
		t.Errorf("expected substitution in sequence, got:\n%s", output)
	}
}

func TestStripVarsNode(t *testing.T) {
	input := `
vars:
  dev:
    DB: DEV_DB
name: test
`
	node := mustParseNode(t, input)
	stripVarsNode(node)

	output := mustEncodeNode(t, node)
	if strings.Contains(output, "vars") {
		t.Errorf("expected vars to be stripped, got:\n%s", output)
	}
	if !strings.Contains(output, "name: test") {
		t.Errorf("expected name to remain, got:\n%s", output)
	}
}

func TestStripVarsNode_NoVars(t *testing.T) {
	input := `name: test`
	node := mustParseNode(t, input)
	stripVarsNode(node)

	output := mustEncodeNode(t, node)
	if !strings.Contains(output, "name: test") {
		t.Errorf("expected name to remain, got:\n%s", output)
	}
}
