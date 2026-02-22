package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"coragent/internal/auth"
)

// testRSAPEM generates a PKCS8 RSA private key PEM for use in tests that need
// a real auth.Config (BearerToken is called by doJSON).
func testRSAPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal RSA key: %v", err)
	}
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: der}
	return string(pem.EncodeToMemory(block))
}

// newDescribeTestClient creates a Client whose baseURL points to srv.
// It uses a freshly generated RSA key so that BearerToken succeeds without
// real Snowflake credentials.
func newDescribeTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	base, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	return &Client{
		baseURL:   base,
		http:      srv.Client(),
		userAgent: "test",
		authCfg: auth.Config{
			Account:    "TEST",
			User:       "TESTUSER",
			PrivateKey: testRSAPEM(t),
		},
		log: discardLogger(),
	}
}

// buildSQLResponse serialises a sqlStatementResponse with the given column
// names and a single data row.
func buildSQLResponse(t *testing.T, cols []string, row []any) []byte {
	t.Helper()
	rowTypes := make([]sqlRowType, len(cols))
	for i, c := range cols {
		rowTypes[i] = sqlRowType{Name: c}
	}
	resp := sqlStatementResponse{
		Data: [][]any{row},
		ResultSetMetaData: struct {
			RowType []sqlRowType `json:"rowType"`
		}{RowType: rowTypes},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal SQL response: %v", err)
	}
	return data
}

// notFoundResponse returns a 400 with a body that isNotFoundError recognises.
func notFoundResponse(w http.ResponseWriter) {
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte(`{"message":"object does not exist"}`))
}

// TestDescribeAgentFull_NotFound verifies that a 400 "does not exist" response
// is converted to Exists=false without an error.
func TestDescribeAgentFull_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		notFoundResponse(w)
	}))
	defer srv.Close()

	c := newDescribeTestClient(t, srv)
	result, err := c.describeAgentFull(context.Background(), "MY_DB", "PUBLIC", "ghost")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Exists {
		t.Error("expected Exists=false for not-found agent")
	}
}

// TestDescribeAgentFull_EmptyData verifies that an empty SQL result set is
// treated as a missing agent.
func TestDescribeAgentFull_EmptyData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		empty := sqlStatementResponse{}
		data, _ := json.Marshal(empty)
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))
	defer srv.Close()

	c := newDescribeTestClient(t, srv)
	result, err := c.describeAgentFull(context.Background(), "MY_DB", "PUBLIC", "agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Exists {
		t.Error("expected Exists=false for empty result set")
	}
}

// TestDescribeAgentFull_NameAndComment verifies that the name and comment
// columns are mapped to the returned AgentSpec.
func TestDescribeAgentFull_NameAndComment(t *testing.T) {
	cols := []string{"name", "comment", "agent_spec"}
	row := []any{"my_agent", "test comment", `{"name":"my_agent"}`}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(buildSQLResponse(t, cols, row))
	}))
	defer srv.Close()

	c := newDescribeTestClient(t, srv)
	result, err := c.describeAgentFull(context.Background(), "MY_DB", "PUBLIC", "my_agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Exists {
		t.Fatal("expected Exists=true")
	}
	if result.Spec.Name != "my_agent" {
		t.Errorf("Name = %q, want %q", result.Spec.Name, "my_agent")
	}
	if result.Spec.Comment != "test comment" {
		t.Errorf("Comment = %q, want %q", result.Spec.Comment, "test comment")
	}
}

// TestDescribeAgentFull_ProfileColumn verifies that a JSON profile string in
// the profile column is decoded into Spec.Profile.
func TestDescribeAgentFull_ProfileColumn(t *testing.T) {
	cols := []string{"name", "comment", "agent_spec", "profile"}
	profileJSON := `{"display_name":"My Agent","avatar":"GlobeAgentIcon","color":"#ff0000"}`
	row := []any{"my_agent", "", `{}`, profileJSON}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(buildSQLResponse(t, cols, row))
	}))
	defer srv.Close()

	c := newDescribeTestClient(t, srv)
	result, err := c.describeAgentFull(context.Background(), "MY_DB", "PUBLIC", "my_agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Spec.Profile == nil {
		t.Fatal("expected non-nil Profile")
	}
	if result.Spec.Profile.DisplayName != "My Agent" {
		t.Errorf("Profile.DisplayName = %q, want %q", result.Spec.Profile.DisplayName, "My Agent")
	}
	if result.Spec.Profile.Avatar != "GlobeAgentIcon" {
		t.Errorf("Profile.Avatar = %q, want %q", result.Spec.Profile.Avatar, "GlobeAgentIcon")
	}
}

// TestDescribeAgentFull_AgentSpecJSON verifies that the agent_spec JSON string
// is decoded and its fields populate Spec.
func TestDescribeAgentFull_AgentSpecJSON(t *testing.T) {
	agentSpec := `{
		"name": "spec_agent",
		"comment": "from spec",
		"instructions": {"response": "Be helpful."},
		"models": {"orchestration": "llama4-scout"},
		"tools": [{"tool_spec": {"type": "cortex_analyst_text_to_sql", "name": "sales"}}]
	}`
	cols := []string{"name", "comment", "agent_spec"}
	row := []any{"spec_agent", "", agentSpec}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(buildSQLResponse(t, cols, row))
	}))
	defer srv.Close()

	c := newDescribeTestClient(t, srv)
	result, err := c.describeAgentFull(context.Background(), "MY_DB", "PUBLIC", "spec_agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Exists {
		t.Fatal("expected Exists=true")
	}
	if result.Spec.Name != "spec_agent" {
		t.Errorf("Name = %q, want %q", result.Spec.Name, "spec_agent")
	}
	if result.Spec.Comment != "from spec" {
		t.Errorf("Comment = %q, want %q", result.Spec.Comment, "from spec")
	}
	if result.Spec.Instructions == nil {
		t.Fatal("expected non-nil Instructions")
	}
	if result.Spec.Models == nil {
		t.Fatal("expected non-nil Models")
	}
	if len(result.Spec.Tools) != 1 {
		t.Errorf("len(Tools) = %d, want 1", len(result.Spec.Tools))
	}
}

// TestDescribeAgentFull_ToolResources verifies that tool_resources in the
// agent_spec JSON (both array and object formats) are decoded correctly.
func TestDescribeAgentFull_ToolResources(t *testing.T) {
	// API returns tool_resources as an array; normalizeToolResources should unwrap it.
	agentSpec := `{
		"name": "tr_agent",
		"tool_resources": {"my_tool": [{"semantic_view": "sv1"}]}
	}`
	cols := []string{"name", "agent_spec"}
	row := []any{"tr_agent", agentSpec}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(buildSQLResponse(t, cols, row))
	}))
	defer srv.Close()

	c := newDescribeTestClient(t, srv)
	result, err := c.describeAgentFull(context.Background(), "MY_DB", "PUBLIC", "tr_agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Spec.ToolResources) == 0 {
		t.Fatal("expected non-empty ToolResources")
	}
}

// TestDescribeAgentFull_AllKnownColumns verifies that all known SQL columns
// are handled without appearing in UnmappedColumns.
func TestDescribeAgentFull_AllKnownColumns(t *testing.T) {
	cols := []string{
		"name", "comment", "agent_spec", "profile",
		"created_on", "database_name", "owner", "schema_name",
	}
	row := []any{
		"my_agent", "comment", `{}`, `{}`,
		"2024-01-01 00:00:00", "MY_DB", "ADMIN", "PUBLIC",
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(buildSQLResponse(t, cols, row))
	}))
	defer srv.Close()

	c := newDescribeTestClient(t, srv)
	result, err := c.describeAgentFull(context.Background(), "MY_DB", "PUBLIC", "my_agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.UnmappedColumns) != 0 {
		t.Errorf("unexpected UnmappedColumns: %v", result.UnmappedColumns)
	}
}

// TestDescribeAgentFull_UnmappedColumn verifies that an unknown SQL column
// appears in UnmappedColumns.
func TestDescribeAgentFull_UnmappedColumn(t *testing.T) {
	cols := []string{"name", "agent_spec", "unknown_future_col"}
	row := []any{"my_agent", `{}`, "some_value"}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(buildSQLResponse(t, cols, row))
	}))
	defer srv.Close()

	c := newDescribeTestClient(t, srv)
	result, err := c.describeAgentFull(context.Background(), "MY_DB", "PUBLIC", "my_agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, col := range result.UnmappedColumns {
		if col == "unknown_future_col" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'unknown_future_col' in UnmappedColumns, got %v", result.UnmappedColumns)
	}
}

// TestDescribeAgentFull_UnmappedSpecKey verifies that an unknown agent_spec
// JSON key appears in UnmappedSpecKeys.
func TestDescribeAgentFull_UnmappedSpecKey(t *testing.T) {
	agentSpec := `{"name":"a","future_field":"value"}`
	cols := []string{"name", "agent_spec"}
	row := []any{"a", agentSpec}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(buildSQLResponse(t, cols, row))
	}))
	defer srv.Close()

	c := newDescribeTestClient(t, srv)
	result, err := c.describeAgentFull(context.Background(), "MY_DB", "PUBLIC", "a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, k := range result.UnmappedSpecKeys {
		if k == "future_field" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'future_field' in UnmappedSpecKeys, got %v", result.UnmappedSpecKeys)
	}
}

// TestDescribeAgentFull_CommentFallback verifies that when agent_spec has no
// comment field, the top-level comment SQL column is used.
func TestDescribeAgentFull_CommentFallback(t *testing.T) {
	cols := []string{"name", "comment", "agent_spec"}
	// agent_spec has no comment; the column comment should be used instead
	row := []any{"my_agent", "column comment", `{"name":"my_agent"}`}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(buildSQLResponse(t, cols, row))
	}))
	defer srv.Close()

	c := newDescribeTestClient(t, srv)
	result, err := c.describeAgentFull(context.Background(), "MY_DB", "PUBLIC", "my_agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Spec.Comment != "column comment" {
		t.Errorf("Comment = %q, want %q", result.Spec.Comment, "column comment")
	}
}

// TestDescribeAgentFull_RawColumnsPresent verifies that RawColumns is populated
// with all column values, enabling debug inspection.
func TestDescribeAgentFull_RawColumnsPresent(t *testing.T) {
	cols := []string{"name", "comment", "agent_spec"}
	row := []any{"raw_agent", "raw comment", `{}`}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(buildSQLResponse(t, cols, row))
	}))
	defer srv.Close()

	c := newDescribeTestClient(t, srv)
	result, err := c.describeAgentFull(context.Background(), "MY_DB", "PUBLIC", "raw_agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RawColumns == nil {
		t.Fatal("expected non-nil RawColumns")
	}
	if _, ok := result.RawColumns["name"]; !ok {
		t.Error("expected 'name' in RawColumns")
	}
	if _, ok := result.RawColumns["comment"]; !ok {
		t.Error("expected 'comment' in RawColumns")
	}
}
