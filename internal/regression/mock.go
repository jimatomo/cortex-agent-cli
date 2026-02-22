// Package regression provides end-to-end scenarios that run against mock HTTP
// servers so no real Snowflake credentials are required.
package regression

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// AgentStore is a simple in-memory store that simulates the Snowflake agent API.
type AgentStore struct {
	mu     sync.Mutex
	agents map[string]map[string]any // name → spec JSON payload
}

func newAgentStore() *AgentStore {
	return &AgentStore{agents: make(map[string]map[string]any)}
}

func (s *AgentStore) set(name string, payload map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agents[name] = payload
}

func (s *AgentStore) get(name string) (map[string]any, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.agents[name]
	return v, ok
}

func (s *AgentStore) del(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.agents, name)
}

func (s *AgentStore) list() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]map[string]any, 0, len(s.agents))
	for _, v := range s.agents {
		out = append(out, v)
	}
	return out
}

// sqlStatementResponse mirrors the Snowflake SQL Statement API response.
type sqlStatementResponse struct {
	Data              [][]any `json:"data"`
	ResultSetMetaData struct {
		RowType []struct {
			Name string `json:"name"`
		} `json:"rowType"`
	} `json:"resultSetMetaData"`
}

// MockServer is a test HTTP server that simulates the Snowflake Cortex Agent API.
type MockServer struct {
	srv    *httptest.Server
	store  *AgentStore
	grants map[string][]string // agentKey → []"PRIVILEGE:GRANTED_TO:GRANTEE_NAME"
	mu     sync.Mutex
}

// NewMockServer creates and starts a MockServer. The caller must call Close() when done.
func NewMockServer(t *testing.T) *MockServer {
	t.Helper()
	ms := &MockServer{
		store:  newAgentStore(),
		grants: make(map[string][]string),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/statements", ms.handleSQL)
	mux.HandleFunc("/api/v2/databases/", ms.handleAgents)
	ms.srv = httptest.NewServer(mux)
	t.Cleanup(ms.srv.Close)
	return ms
}

// URL returns the base URL of the mock server.
func (ms *MockServer) URL() string {
	return ms.srv.URL
}

// SetGrants sets the grants for an agent (used to prime the store for test scenarios).
// Each entry is "PRIVILEGE:GRANTED_TO:GRANTEE_NAME" (e.g., "USAGE:ROLE:MY_ROLE").
func (ms *MockServer) SetGrants(agentKey string, grants []string) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.grants[agentKey] = grants
}

func (ms *MockServer) handleSQL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Statement string `json:"statement"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	stmt := strings.TrimSpace(req.Statement)
	upper := strings.ToUpper(stmt)

	switch {
	case strings.HasPrefix(upper, "DESCRIBE AGENT "):
		ms.handleDescribeAgent(w, stmt)
	case strings.HasPrefix(upper, "SHOW GRANTS ON AGENT "):
		ms.handleShowGrants(w, stmt)
	case strings.HasPrefix(upper, "GRANT "):
		ms.handleGrant(w, stmt, true)
	case strings.HasPrefix(upper, "REVOKE "):
		ms.handleGrant(w, stmt, false)
	default:
		// Unknown SQL — return empty result.
		writeJSON(w, sqlStatementResponse{})
	}
}

func (ms *MockServer) handleDescribeAgent(w http.ResponseWriter, stmt string) {
	// Extract agent name from "DESCRIBE AGENT db.schema.name".
	parts := strings.Fields(stmt)
	if len(parts) < 3 {
		writeNotFound(w)
		return
	}
	fq := parts[2] // e.g. "MYDB.MYSCHEMA.MY_AGENT"
	segs := strings.Split(fq, ".")
	name := stripQuotes(segs[len(segs)-1])

	payload, ok := ms.store.get(name)
	if !ok {
		writeNotFound(w)
		return
	}

	specJSON, _ := json.Marshal(payload)
	var resp sqlStatementResponse
	resp.ResultSetMetaData.RowType = []struct {
		Name string `json:"name"`
	}{
		{Name: "agent_spec"},
		{Name: "name"},
		{Name: "comment"},
		{Name: "profile"},
	}
	resp.Data = [][]any{
		{string(specJSON), name, "", nil},
	}
	writeJSON(w, resp)
}

func (ms *MockServer) handleShowGrants(w http.ResponseWriter, stmt string) {
	parts := strings.Fields(stmt)
	if len(parts) < 5 {
		writeJSON(w, sqlStatementResponse{})
		return
	}
	fq := parts[4]
	segs := strings.Split(fq, ".")
	name := stripQuotes(segs[len(segs)-1])

	ms.mu.Lock()
	grantList := ms.grants[name]
	ms.mu.Unlock()

	var resp sqlStatementResponse
	resp.ResultSetMetaData.RowType = []struct {
		Name string `json:"name"`
	}{
		{Name: "created_on"},
		{Name: "privilege"},
		{Name: "granted_on"},
		{Name: "name"},
		{Name: "granted_to"},
		{Name: "grantee_name"},
		{Name: "grant_option"},
		{Name: "granted_by"},
	}
	for _, g := range grantList {
		parts2 := strings.SplitN(g, ":", 3)
		if len(parts2) != 3 {
			continue
		}
		resp.Data = append(resp.Data, []any{
			"", parts2[0], "AGENT", name, parts2[1], parts2[2], "false", "",
		})
	}
	writeJSON(w, resp)
}

func (ms *MockServer) handleGrant(w http.ResponseWriter, stmt string, isGrant bool) {
	// Parse: GRANT <priv> ON AGENT <fq> TO [DATABASE] ROLE <grantee>
	//        REVOKE <priv> ON AGENT <fq> FROM [DATABASE] ROLE <grantee>
	parts := strings.Fields(stmt)
	upper := strings.Fields(strings.ToUpper(stmt))

	// Locate "AGENT" keyword to find the FQ agent name.
	agentIdx := -1
	for i, p := range upper {
		if p == "AGENT" {
			agentIdx = i
			break
		}
	}
	if agentIdx < 0 || agentIdx+1 >= len(parts) {
		writeJSON(w, sqlStatementResponse{})
		return
	}

	privilege := parts[1]
	fq := parts[agentIdx+1]
	segs := strings.Split(fq, ".")
	agentName := stripQuotes(segs[len(segs)-1])

	// Find ROLE or DATABASE ROLE keyword after TO/FROM.
	var grantedTo, grantee string
	for i := agentIdx + 2; i < len(upper); i++ {
		if upper[i] == "ROLE" {
			if i > 0 && upper[i-1] == "DATABASE" {
				grantedTo = "DATABASE_ROLE"
			} else {
				grantedTo = "ROLE"
			}
			if i+1 < len(parts) {
				grantee = parts[i+1]
			}
			break
		}
	}
	if grantedTo == "" || grantee == "" {
		writeJSON(w, sqlStatementResponse{})
		return
	}

	entry := privilege + ":" + grantedTo + ":" + grantee

	ms.mu.Lock()
	defer ms.mu.Unlock()
	if isGrant {
		for _, g := range ms.grants[agentName] {
			if g == entry {
				writeJSON(w, sqlStatementResponse{}) // already granted
				return
			}
		}
		ms.grants[agentName] = append(ms.grants[agentName], entry)
	} else {
		var updated []string
		for _, g := range ms.grants[agentName] {
			if g != entry {
				updated = append(updated, g)
			}
		}
		ms.grants[agentName] = updated
	}

	writeJSON(w, sqlStatementResponse{})
}

func (ms *MockServer) handleAgents(w http.ResponseWriter, r *http.Request) {
	// URL pattern: /api/v2/databases/{db}/schemas/{schema}/agents[/{name}]
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v2/databases/"), "/")
	// parts: [db, "schemas", schema, "agents"] or [db, "schemas", schema, "agents", name]

	hasName := len(parts) >= 5 && parts[4] != ""
	var agentName string
	if hasName {
		agentName = stripQuotes(parts[4])
	}

	switch r.Method {
	case http.MethodGet:
		if hasName {
			// Single agent GET (not commonly used; describe uses SQL)
			payload, ok := ms.store.get(agentName)
			if !ok {
				writeNotFound(w)
				return
			}
			writeJSON(w, payload)
		} else {
			// List agents
			list := ms.store.list()
			if list == nil {
				list = []map[string]any{}
			}
			writeJSON(w, list)
		}
	case http.MethodPost:
		// Create agent
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		name, _ := payload["name"].(string)
		name = stripQuotes(name)   // client may send SQL-quoted names
		payload["name"] = name     // normalize name in stored payload
		ms.store.set(name, payload)
		w.WriteHeader(http.StatusOK)
		writeJSON(w, payload)
	case http.MethodPut:
		// Update agent
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if !hasName {
			http.Error(w, "missing name", http.StatusBadRequest)
			return
		}
		ms.store.set(agentName, payload)
		w.WriteHeader(http.StatusOK)
	case http.MethodDelete:
		if !hasName {
			http.Error(w, "missing name", http.StatusBadRequest)
			return
		}
		ms.store.del(agentName)
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// TestRSAPEM generates a PKCS8 RSA private key PEM for use in tests.
func TestRSAPEM(t *testing.T) string {
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

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func writeNotFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte(`{"message":"object does not exist"}`)) //nolint:errcheck
}

func stripQuotes(s string) string {
	s = strings.TrimPrefix(s, `"`)
	s = strings.TrimSuffix(s, `"`)
	return s
}
