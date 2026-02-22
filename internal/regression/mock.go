// Package regression provides end-to-end scenarios that run against mock HTTP
// servers so no real Snowflake credentials are required.
package regression

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
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
	srv      *httptest.Server
	store    *AgentStore
	grants   map[string][]string // agentKey → []"PRIVILEGE:GRANTED_TO:GRANTEE_NAME"
	runReply map[string]string   // agentKey → raw SSE body to stream on :run
	threads  map[string]map[string]any
	nextTID  int64
	mu       sync.Mutex
}

// NewMockServer creates and starts a MockServer. The caller must call Close() when done.
func NewMockServer(t *testing.T) *MockServer {
	t.Helper()
	ms := &MockServer{
		store:    newAgentStore(),
		grants:   make(map[string][]string),
		runReply: make(map[string]string),
		threads:  make(map[string]map[string]any),
		nextTID:  1,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/statements", ms.handleSQL)
	mux.HandleFunc("/api/v2/databases/", ms.handleAgents)
	mux.HandleFunc("/api/v2/cortex/threads", ms.handleThreads)
	mux.HandleFunc("/api/v2/cortex/threads/", ms.handleThread)
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

// SetRunReply registers a raw SSE body to stream when the :run endpoint is called
// for the given agent name. Use BuildSSEReply to construct well-formed SSE bodies.
func (ms *MockServer) SetRunReply(agentName, sseBody string) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.runReply[agentName] = sseBody
}

// BuildSSEReply constructs a minimal SSE stream that delivers textReply as a
// text response with an optional list of tool names called before the final text.
func BuildSSEReply(textReply string, toolNames ...string) string {
	var b strings.Builder
	seq := 1
	fmt.Fprintf(&b, "event: response.status\ndata: {\"status\":\"running\",\"message\":\"\",\"sequence_number\":%d}\n\n", seq)
	seq++
	for i, tool := range toolNames {
		fmt.Fprintf(&b, "event: response.tool_use\ndata: {\"name\":%q,\"tool_use_id\":\"id%d\",\"input\":{},\"content_index\":%d,\"sequence_number\":%d}\n\n",
			tool, i, i, seq)
		seq++
		fmt.Fprintf(&b, "event: response.tool_result\ndata: {\"name\":%q,\"tool_use_id\":\"id%d\",\"status\":\"success\",\"content\":{},\"content_index\":%d,\"sequence_number\":%d}\n\n",
			tool, i, i, seq)
		seq++
	}
	fmt.Fprintf(&b, "event: response.text.delta\ndata: {\"text\":%q,\"content_index\":0,\"sequence_number\":%d}\n\n", textReply, seq)
	seq++
	fmt.Fprintf(&b, "event: metadata\ndata: {\"metadata\":{\"thread_id\":\"mock-thread\",\"message_id\":1,\"role\":\"assistant\"}}\n\n")
	fmt.Fprintf(&b, "event: response.complete\ndata: {\"content\":[{\"type\":\"text\",\"text\":%q}]}\n\n", textReply)
	_ = seq
	return b.String()
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
	// URL pattern: /api/v2/databases/{db}/schemas/{schema}/agents[/{name}[:{action}]]
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v2/databases/"), "/")
	// parts: [db, "schemas", schema, "agents"] or [db, "schemas", schema, "agents", name[:action]]

	hasName := len(parts) >= 5 && parts[4] != ""
	var agentName string
	if hasName {
		seg := parts[4]
		// Strip action suffix (e.g. "my-agent:run" → "my-agent")
		if idx := strings.Index(seg, ":"); idx >= 0 {
			action := seg[idx+1:]
			agentName = stripQuotes(seg[:idx])
			if action == "run" && r.Method == http.MethodPost {
				ms.handleRun(w, r, agentName)
				return
			}
		} else {
			agentName = stripQuotes(seg)
		}
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

// handleThreads handles the /api/v2/cortex/threads collection endpoint.
func (ms *MockServer) handleThreads(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		ms.mu.Lock()
		id := fmt.Sprintf("%d", ms.nextTID)
		ms.nextTID++
		now := int64(1000000) // fake epoch ms
		t := map[string]any{
			"thread_id":          id,
			"thread_name":        "",
			"origin_application": "coragent",
			"created_on":         now,
			"updated_on":         now,
		}
		ms.threads[id] = t
		ms.mu.Unlock()
		writeJSON(w, t)
	case http.MethodGet:
		ms.mu.Lock()
		out := make([]map[string]any, 0, len(ms.threads))
		for _, t := range ms.threads {
			out = append(out, t)
		}
		ms.mu.Unlock()
		writeJSON(w, out)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleThread handles /api/v2/cortex/threads/{id} (GET, DELETE).
func (ms *MockServer) handleThread(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v2/cortex/threads/")
	switch r.Method {
	case http.MethodGet:
		ms.mu.Lock()
		t, ok := ms.threads[id]
		ms.mu.Unlock()
		if !ok {
			writeNotFound(w)
			return
		}
		writeJSON(w, t)
	case http.MethodDelete:
		ms.mu.Lock()
		delete(ms.threads, id)
		ms.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleRun serves the agent :run streaming endpoint.
// It returns the pre-registered SSE body for the agent, or an empty response.
func (ms *MockServer) handleRun(w http.ResponseWriter, _ *http.Request, agentName string) {
	ms.mu.Lock()
	body, ok := ms.runReply[agentName]
	ms.mu.Unlock()

	if !ok {
		http.Error(w, "no run reply configured", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, body)
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
