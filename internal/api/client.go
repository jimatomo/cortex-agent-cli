package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"coragent/internal/agent"
	"coragent/internal/auth"
)

type Client struct {
	baseURL      *url.URL
	role         string
	userAgent    string
	http         *http.Client
	authCfg      auth.Config
	sessionToken *auth.SessionToken
	debug        bool
}

type APIError struct {
	StatusCode int
	Body       string
}

func (e APIError) Error() string {
	return fmt.Sprintf("api error: status=%d body=%s", e.StatusCode, e.Body)
}

// isNotFoundError checks if the error indicates a resource does not exist.
// This includes HTTP 404 errors and Snowflake SQL errors for non-existent objects.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	if apiErr, ok := err.(APIError); ok {
		if apiErr.StatusCode == 404 {
			return true
		}
		// Check for Snowflake SQL error messages indicating object does not exist
		bodyLower := strings.ToLower(apiErr.Body)
		if strings.Contains(bodyLower, "does not exist") ||
			strings.Contains(bodyLower, "object does not exist") ||
			strings.Contains(bodyLower, "agent") && strings.Contains(bodyLower, "not found") ||
			strings.Contains(bodyLower, "002003") { // Snowflake error code for object not found
			return true
		}
	}
	errMsg := strings.ToLower(err.Error())
	if strings.Contains(errMsg, "does not exist") ||
		strings.Contains(errMsg, "not found") {
		return true
	}
	return false
}

func NewClient(cfg auth.Config) (*Client, error) {
	return NewClientWithDebug(cfg, false)
}

func NewClientWithDebug(cfg auth.Config, debug bool) (*Client, error) {
	if cfg.Account == "" {
		return nil, fmt.Errorf("SNOWFLAKE_ACCOUNT is required")
	}
	base, err := url.Parse(fmt.Sprintf("https://%s.snowflakecomputing.com", cfg.Account))
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}

	// Login to get session token
	ctx := context.Background()
	sessionToken, err := auth.Login(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("login failed: %w", err)
	}

	return &Client{
		baseURL:      base,
		role:         strings.ToUpper(strings.TrimSpace(cfg.Role)),
		userAgent:    "coragent",
		http:         &http.Client{Timeout: 60 * time.Second},
		authCfg:      cfg,
		sessionToken: sessionToken,
		debug:        debug,
	}, nil
}

func (c *Client) CreateAgent(ctx context.Context, db, schema string, spec agent.AgentSpec) error {
	payload := normalizeAgentSpec(spec)
	return c.doJSON(ctx, http.MethodPost, c.agentsURL(db, schema), payload, nil)
}

func (c *Client) UpdateAgent(ctx context.Context, db, schema, name string, payload any) error {
	payload = normalizePayload(payload)
	return c.doJSON(ctx, http.MethodPut, c.agentURL(db, schema, name), payload, nil)
}

func (c *Client) DeleteAgent(ctx context.Context, db, schema, name string) error {
	return c.doJSON(ctx, http.MethodDelete, c.agentURL(db, schema, name), nil, nil)
}

func (c *Client) GetAgent(ctx context.Context, db, schema, name string) (agent.AgentSpec, bool, error) {
	spec, exists, err := c.describeAgent(ctx, db, schema, name)
	if err != nil || !exists {
		return agent.AgentSpec{}, exists, err
	}
	return spec, true, nil
}

func (c *Client) agentsURL(db, schema string) string {
	u := *c.baseURL
	u.Path = path.Join(
		u.Path,
		"api/v2/databases",
		identifierSegment(db),
		"schemas",
		identifierSegment(schema),
		"agents",
	)
	return u.String()
}

type agentListItem struct {
	Name    string `json:"name"`
	Comment string `json:"comment"`
}

func (c *Client) listAgents(ctx context.Context, db, schema string) ([]agentListItem, error) {
	var out []agentListItem
	if err := c.doJSON(ctx, http.MethodGet, c.agentsURL(db, schema), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) findAgentListEntry(ctx context.Context, db, schema, name string) (*agentListItem, error) {
	items, err := c.listAgents(ctx, db, schema)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].Name == name {
			return &items[i], nil
		}
	}
	return nil, nil
}

type sqlStatementRequest struct {
	Statement string `json:"statement"`
	Database  string `json:"database,omitempty"`
	Schema    string `json:"schema,omitempty"`
	Warehouse string `json:"warehouse,omitempty"`
	Role      string `json:"role,omitempty"`
}

type sqlRowType struct {
	Name string `json:"name"`
}

type sqlStatementResponse struct {
	Data              [][]any `json:"data"`
	ResultSetMetaData struct {
		RowType []sqlRowType `json:"rowType"`
	} `json:"resultSetMetaData"`
}

func (c *Client) sqlURL() string {
	u := *c.baseURL
	u.Path = path.Join(u.Path, "api/v2/statements")
	return u.String()
}

func (c *Client) describeAgent(ctx context.Context, db, schema, name string) (agent.AgentSpec, bool, error) {
	stmt := fmt.Sprintf("DESCRIBE AGENT %s.%s.%s", identifierSegment(db), identifierSegment(schema), identifierSegment(name))
	payload := sqlStatementRequest{
		Statement: stmt,
		Database:  db,
		Schema:    schema,
	}
	if strings.TrimSpace(c.authCfg.Warehouse) != "" {
		payload.Warehouse = c.authCfg.Warehouse
	}
	if strings.TrimSpace(c.role) != "" {
		payload.Role = c.role
	}
	var resp sqlStatementResponse
	if err := c.doJSON(ctx, http.MethodPost, c.sqlURL(), payload, &resp); err != nil {
		// Check if the error indicates the agent does not exist
		if isNotFoundError(err) {
			return agent.AgentSpec{}, false, nil
		}
		return agent.AgentSpec{}, false, err
	}
	if len(resp.Data) == 0 || len(resp.ResultSetMetaData.RowType) == 0 {
		return agent.AgentSpec{}, false, nil
	}
	row := resp.Data[0]
	raw := make(map[string]any)
	for i, col := range resp.ResultSetMetaData.RowType {
		if i < len(row) {
			raw[strings.ToLower(col.Name)] = row[i]
		}
	}

	spec := agent.AgentSpec{}
	if specJSON, ok := raw["agent_spec"]; ok {
		decoded, ok, err := decodeAgentSpecJSON(specJSON, spec, raw)
		if err != nil {
			return agent.AgentSpec{}, false, err
		}
		if ok {
			spec = decoded
		}
	}

	if nameVal, ok := raw["name"].(string); ok && strings.TrimSpace(spec.Name) == "" {
		spec.Name = nameVal
	}
	if commentVal, ok := raw["comment"].(string); ok && strings.TrimSpace(commentVal) != "" {
		spec.Comment = commentVal
	}
	if profileVal, ok := raw["profile"]; ok {
		if profile, err := decodeProfile(profileVal); err == nil && profile != nil {
			spec.Profile = profile
		} else if err != nil {
			return agent.AgentSpec{}, false, err
		}
	}
	return spec, true, nil
}

func decodeProfile(value any) (*agent.Profile, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		data, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshal profile: %w", err)
		}
		var profile agent.Profile
		if err := json.Unmarshal(data, &profile); err != nil {
			return nil, fmt.Errorf("decode profile: %w", err)
		}
		return &profile, nil
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, nil
		}
		var profile agent.Profile
		if err := json.Unmarshal([]byte(v), &profile); err != nil {
			return nil, fmt.Errorf("decode profile JSON: %w", err)
		}
		return &profile, nil
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshal profile value: %w", err)
		}
		var profile agent.Profile
		if err := json.Unmarshal(data, &profile); err != nil {
			return nil, fmt.Errorf("decode profile value: %w", err)
		}
		return &profile, nil
	}
}

func mergeAgentSpecs(base, extra agent.AgentSpec) agent.AgentSpec {
	if strings.TrimSpace(extra.Name) != "" {
		base.Name = extra.Name
	}
	if strings.TrimSpace(extra.Comment) != "" {
		base.Comment = extra.Comment
	}
	if extra.Profile != nil {
		base.Profile = extra.Profile
	}
	if extra.Models != nil {
		base.Models = extra.Models
	}
	if extra.Instructions != nil {
		base.Instructions = extra.Instructions
	}
	if extra.Orchestration != nil {
		base.Orchestration = extra.Orchestration
	}
	if len(extra.Tools) > 0 {
		base.Tools = extra.Tools
	}
	if len(extra.ToolResources) > 0 {
		base.ToolResources = extra.ToolResources
	}
	return base
}

func (c *Client) agentURL(db, schema, name string) string {
	u := *c.baseURL
	u.Path = path.Join(
		u.Path,
		"api/v2/databases",
		identifierSegment(db),
		"schemas",
		identifierSegment(schema),
		"agents",
		identifierSegment(name),
	)
	return u.String()
}

func (c *Client) doJSON(ctx context.Context, method, urlStr string, payload any, out any) error {
	var body io.Reader
	var reqBody []byte
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal payload: %w", err)
		}
		reqBody = data
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, urlStr, body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Use session token for authorization
	req.Header.Set("Authorization", "Snowflake Token=\""+c.sessionToken.Token+"\"")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.role != "" {
		req.Header.Set("X-Snowflake-Role", c.role)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if c.debug {
		bodyBytes, _ := io.ReadAll(resp.Body)
		c.debugf("HTTP %s %s -> %d", method, urlStr, resp.StatusCode)
		if len(reqBody) > 0 {
			c.debugf("request body: %s", truncateDebug(reqBody))
		}
		if len(bodyBytes) > 0 {
			c.debugf("response body: %s", truncateDebug(bodyBytes))
		}
		if resp.StatusCode >= 300 {
			return APIError{StatusCode: resp.StatusCode, Body: string(bodyBytes)}
		}
		if out != nil {
			if err := json.NewDecoder(bytes.NewReader(bodyBytes)).Decode(out); err != nil && err != io.EOF {
				return fmt.Errorf("decode response: %w", err)
			}
		}
		return nil
	}

	if resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return APIError{StatusCode: resp.StatusCode, Body: string(bodyBytes)}
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil && err != io.EOF {
			return fmt.Errorf("decode response: %w", err)
		}
	}

	return nil
}

func identifierSegment(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) >= 2 && strings.HasPrefix(trimmed, "\"") && strings.HasSuffix(trimmed, "\"") {
		trimmed = trimmed[1 : len(trimmed)-1]
	}
	if !isSimpleIdentifier(trimmed) {
		trimmed = `"` + strings.ReplaceAll(trimmed, `"`, `""`) + `"`
	}
	return trimmed
}

func normalizeAgentSpec(spec agent.AgentSpec) agent.AgentSpec {
	spec.Name = identifierSegment(spec.Name)
	return spec
}

func normalizePayload(payload any) any {
	switch value := payload.(type) {
	case map[string]any:
		if raw, ok := value["name"]; ok {
			if name, ok := raw.(string); ok {
				value["name"] = identifierSegment(name)
			}
		}
		return value
	default:
		return payload
	}
}

func isSimpleIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if i == 0 {
			if !isIdentifierStart(r) {
				return false
			}
			continue
		}
		if !isIdentifierPart(r) {
			return false
		}
	}
	return true
}

func decodeAgentSpec(data []byte) (agent.AgentSpec, error) {
	if len(data) == 0 {
		return agent.AgentSpec{}, nil
	}
	var direct agent.AgentSpec
	if err := json.Unmarshal(data, &direct); err != nil {
		return agent.AgentSpec{}, fmt.Errorf("decode agent: %w", err)
	}

	var rawAny any
	if err := json.Unmarshal(data, &rawAny); err == nil {
		switch raw := rawAny.(type) {
		case map[string]any:
			if decoded, ok, err := decodeAgentSpecFromMap(raw, direct); err != nil {
				return agent.AgentSpec{}, err
			} else if ok {
				return decoded, nil
			}
		case []any:
			if specMap, name := findAgentSpecFromSlice(raw, 0); specMap != nil {
				normalized := normalizeAgentSpecMap(specMap)
				decoded, err := decodeSpecMap(normalized)
				if err != nil {
					return agent.AgentSpec{}, err
				}
				if decoded.Name == "" {
					if name != "" {
						decoded.Name = name
					} else if direct.Name != "" {
						decoded.Name = direct.Name
					}
				}
				return decoded, nil
			}
		}
	}
	return direct, nil
}

func (c *Client) debugf(format string, args ...any) {
	if !c.debug {
		return
	}
	fmt.Fprintf(os.Stderr, "DEBUG: "+format+"\n", args...)
}

func truncateDebug(data []byte) string {
	const limit = 4000
	if len(data) <= limit {
		return string(data)
	}
	return string(data[:limit]) + "...(truncated)"
}

func decodeAgentSpecFromMap(raw map[string]any, direct agent.AgentSpec) (agent.AgentSpec, bool, error) {
	if specJSON, ok := raw["agent_spec"]; ok {
		if decoded, ok, err := decodeAgentSpecJSON(specJSON, direct, raw); err != nil {
			return agent.AgentSpec{}, false, err
		} else if ok {
			return decoded, true, nil
		}
	}
	if specMap, name := findAgentSpec(raw, 0); specMap != nil {
		normalized := normalizeAgentSpecMap(specMap)
		normalized = mergeSpecFallbacks(normalized, raw)
		decoded, err := decodeSpecMap(normalized)
		if err != nil {
			return agent.AgentSpec{}, false, err
		}
		if decoded.Name == "" {
			if name != "" {
				decoded.Name = name
			} else if direct.Name != "" {
				decoded.Name = direct.Name
			}
		}
		return decoded, true, nil
	}
	return agent.AgentSpec{}, false, nil
}

func decodeAgentSpecJSON(specJSON any, direct agent.AgentSpec, raw map[string]any) (agent.AgentSpec, bool, error) {
	specStr, ok := specJSON.(string)
	if !ok || strings.TrimSpace(specStr) == "" {
		return agent.AgentSpec{}, false, nil
	}
	var specMap map[string]any
	if err := json.Unmarshal([]byte(specStr), &specMap); err != nil {
		return agent.AgentSpec{}, false, fmt.Errorf("decode agent_spec JSON: %w", err)
	}
	normalized := normalizeAgentSpecMap(specMap)
	normalized = mergeSpecFallbacks(normalized, raw)
	decoded, err := decodeSpecMap(normalized)
	if err != nil {
		return agent.AgentSpec{}, false, err
	}
	if decoded.Name == "" {
		if name, ok := raw["name"].(string); ok {
			decoded.Name = name
		} else if direct.Name != "" {
			decoded.Name = direct.Name
		}
	}
	return decoded, true, nil
}

func decodeSpecMap(specMap map[string]any) (agent.AgentSpec, error) {
	data, err := json.Marshal(specMap)
	if err != nil {
		return agent.AgentSpec{}, fmt.Errorf("marshal spec map: %w", err)
	}
	var spec agent.AgentSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return agent.AgentSpec{}, fmt.Errorf("decode spec map: %w", err)
	}
	return spec, nil
}

func normalizeAgentSpecMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		normalizedKey := normalizeAgentKey(key)
		switch normalizedKey {
		case "profile":
			switch v := value.(type) {
			case map[string]any:
				out[normalizedKey] = normalizeProfileMap(v)
				continue
			case string:
				if strings.TrimSpace(v) == "" {
					out[normalizedKey] = value
					continue
				}
				var profileMap map[string]any
				if err := json.Unmarshal([]byte(v), &profileMap); err == nil {
					out[normalizedKey] = normalizeProfileMap(profileMap)
					continue
				}
			}
		case "models":
			if m, ok := value.(map[string]any); ok {
				out[normalizedKey] = normalizeModelsMap(m)
				continue
			}
		case "instructions":
			if m, ok := value.(map[string]any); ok {
				out[normalizedKey] = normalizeInstructionsMap(m)
				continue
			}
		case "orchestration":
			if m, ok := value.(map[string]any); ok {
				out[normalizedKey] = normalizeOrchestrationMap(m)
				continue
			}
		case "tools":
			if list, ok := value.([]any); ok {
				out[normalizedKey] = normalizeToolsList(list)
				continue
			}
		}
		out[normalizedKey] = value
	}
	return out
}

func mergeSpecFallbacks(spec map[string]any, raw map[string]any) map[string]any {
	normalizedRaw := normalizeAgentSpecMap(raw)
	for _, key := range []string{
		"name",
		"comment",
		"profile",
		"models",
		"instructions",
		"orchestration",
		"tools",
		"tool_resources",
	} {
		if _, ok := spec[key]; ok {
			continue
		}
		if val, ok := normalizedRaw[key]; ok {
			spec[key] = val
		}
	}
	return spec
}

func normalizeAgentKey(key string) string {
	switch {
	case strings.EqualFold(key, "toolResources"),
		strings.EqualFold(key, "tool_resources"),
		strings.EqualFold(key, "toolresources"):
		return "tool_resources"
	default:
		return key
	}
}

func normalizeProfileMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		switch {
		case strings.EqualFold(key, "displayName"),
			strings.EqualFold(key, "display_name"):
			out["display_name"] = value
		default:
			out[key] = value
		}
	}
	return out
}

func normalizeModelsMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		switch {
		case strings.EqualFold(key, "orchestration"):
			out["orchestration"] = value
		default:
			out[key] = value
		}
	}
	return out
}

func normalizeInstructionsMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		switch {
		case strings.EqualFold(key, "response"),
			strings.EqualFold(key, "orchestration"),
			strings.EqualFold(key, "system"),
			strings.EqualFold(key, "examples"):
			out[key] = value
		default:
			out[key] = value
		}
	}
	return out
}

func normalizeOrchestrationMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		switch {
		case strings.EqualFold(key, "budget"):
			if budgetMap, ok := value.(map[string]any); ok {
				out["budget"] = normalizeBudgetMap(budgetMap)
				continue
			}
		case strings.EqualFold(key, "budgetSecs"),
			strings.EqualFold(key, "budget_secs"):
			out["budget"] = normalizeBudgetMap(map[string]any{"seconds": value})
			continue
		case strings.EqualFold(key, "maxTokens"),
			strings.EqualFold(key, "max_tokens"):
			out["budget"] = normalizeBudgetMap(map[string]any{"tokens": value})
			continue
		default:
			out[key] = value
		}
	}
	return out
}

func normalizeBudgetMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		switch {
		case strings.EqualFold(key, "seconds"),
			strings.EqualFold(key, "budget_secs"),
			strings.EqualFold(key, "budgetSecs"):
			out["seconds"] = value
		case strings.EqualFold(key, "tokens"),
			strings.EqualFold(key, "max_tokens"),
			strings.EqualFold(key, "maxTokens"):
			out["tokens"] = value
		default:
			out[key] = value
		}
	}
	return out
}

func normalizeToolsList(input []any) []any {
	out := make([]any, 0, len(input))
	for _, item := range input {
		m, ok := item.(map[string]any)
		if !ok {
			out = append(out, item)
			continue
		}
		tool := make(map[string]any, len(m))
		for key, value := range m {
			switch {
			case strings.EqualFold(key, "toolSpec"),
				strings.EqualFold(key, "tool_spec"):
				tool["tool_spec"] = value
			default:
				tool[key] = value
			}
		}
		out = append(out, tool)
	}
	return out
}

func findAgentSpec(raw map[string]any, depth int) (map[string]any, string) {
	if depth > 4 {
		return nil, ""
	}
	if spec, ok := pickSpecMap(raw); ok {
		return spec, pickName(raw)
	}
	for _, key := range []string{"agent", "data", "result", "payload"} {
		switch nested := raw[key].(type) {
		case map[string]any:
			if spec, name := findAgentSpec(nested, depth+1); spec != nil {
				if name == "" {
					name = pickName(raw)
				}
				return spec, name
			}
		case []any:
			if spec, name := findAgentSpecFromSlice(nested, depth+1); spec != nil {
				if name == "" {
					name = pickName(raw)
				}
				return spec, name
			}
		}
	}
	if looksLikeAgentSpec(raw) {
		return raw, pickName(raw)
	}
	for _, value := range raw {
		switch nested := value.(type) {
		case map[string]any:
			if spec, name := findAgentSpec(nested, depth+1); spec != nil {
				return spec, name
			}
		case []any:
			if spec, name := findAgentSpecFromSlice(nested, depth+1); spec != nil {
				return spec, name
			}
		}
	}
	return nil, ""
}

func findAgentSpecFromSlice(items []any, depth int) (map[string]any, string) {
	if depth > 4 {
		return nil, ""
	}
	for _, item := range items {
		switch value := item.(type) {
		case map[string]any:
			if spec, name := findAgentSpec(value, depth+1); spec != nil {
				return spec, name
			}
		}
	}
	return nil, ""
}

func pickSpecMap(raw map[string]any) (map[string]any, bool) {
	for _, key := range []string{"spec", "agent_spec", "agentSpec"} {
		if spec, ok := raw[key].(map[string]any); ok {
			return spec, true
		}
	}
	return nil, false
}

func pickName(raw map[string]any) string {
	if name, ok := raw["name"].(string); ok {
		return name
	}
	return ""
}

func looksLikeAgentSpec(raw map[string]any) bool {
	if _, ok := raw["name"]; ok {
		return true
	}
	if _, ok := raw["comment"]; ok {
		return true
	}
	if _, ok := raw["profile"]; ok {
		return true
	}
	if _, ok := raw["models"]; ok {
		return true
	}
	if _, ok := raw["instructions"]; ok {
		return true
	}
	if _, ok := raw["orchestration"]; ok {
		return true
	}
	if _, ok := raw["tools"]; ok {
		return true
	}
	if _, ok := raw["tool_resources"]; ok {
		return true
	}
	return false
}

func isIdentifierStart(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_'
}

func isIdentifierPart(r rune) bool {
	return isIdentifierStart(r) || (r >= '0' && r <= '9') || r == '$'
}
