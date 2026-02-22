package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strings"

	"coragent/internal/agent"
)

// AgentListItem is a summary entry returned by the list agents endpoint.
type AgentListItem struct {
	Name    string `json:"name"`
	Comment string `json:"comment"`
}

// DescribeResult holds the full result of a DESCRIBE AGENT call, including
// any columns or agent_spec keys that are not mapped to AgentSpec fields.
type DescribeResult struct {
	Spec             agent.AgentSpec
	Exists           bool
	UnmappedColumns  []string       // DESCRIBE AGENT SQL columns not processed
	UnmappedSpecKeys []string       // agent_spec JSON keys not mapped
	RawColumns       map[string]any // all column data (for debug)
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

// CreateAgent creates a new agent with the given spec.
func (c *Client) CreateAgent(ctx context.Context, db, schema string, spec agent.AgentSpec) error {
	payload := normalizeAgentSpec(spec)
	return c.doJSON(ctx, http.MethodPost, c.agentsURL(db, schema), payload, nil)
}

// UpdateAgent updates an existing agent with the given payload.
func (c *Client) UpdateAgent(ctx context.Context, db, schema, name string, payload any) error {
	payload = normalizePayload(payload)
	return c.doJSON(ctx, http.MethodPut, c.agentURL(db, schema, name), payload, nil)
}

// DeleteAgent deletes the named agent.
func (c *Client) DeleteAgent(ctx context.Context, db, schema, name string) error {
	return c.doJSON(ctx, http.MethodDelete, c.agentURL(db, schema, name), nil, nil)
}

// GetAgent returns the agent spec and a boolean indicating whether the agent exists.
func (c *Client) GetAgent(ctx context.Context, db, schema, name string) (agent.AgentSpec, bool, error) {
	result, err := c.describeAgentFull(ctx, db, schema, name)
	if err != nil || !result.Exists {
		return agent.AgentSpec{}, result.Exists, err
	}
	return result.Spec, true, nil
}

// DescribeAgent returns the full describe result including unmapped column/key detection.
func (c *Client) DescribeAgent(ctx context.Context, db, schema, name string) (DescribeResult, error) {
	return c.describeAgentFull(ctx, db, schema, name)
}

// ListAgents returns a summary list of agents in the given database and schema.
func (c *Client) ListAgents(ctx context.Context, db, schema string) ([]AgentListItem, error) {
	var out []AgentListItem
	if err := c.doJSON(ctx, http.MethodGet, c.agentsURL(db, schema), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) describeAgentFull(ctx context.Context, db, schema, name string) (DescribeResult, error) {
	stmt := fmt.Sprintf("DESCRIBE AGENT %s.%s.%s", identifierSegment(db), identifierSegment(schema), identifierSegment(name))
	payload := sqlStatementRequest{
		Statement: stmt,
		Database:  unquoteIdentifier(db),
		Schema:    unquoteIdentifier(schema),
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
			return DescribeResult{Exists: false}, nil
		}
		return DescribeResult{}, err
	}
	if len(resp.Data) == 0 || len(resp.ResultSetMetaData.RowType) == 0 {
		return DescribeResult{Exists: false}, nil
	}
	row := resp.Data[0]
	raw := make(map[string]any)
	for i, col := range resp.ResultSetMetaData.RowType {
		if i < len(row) {
			raw[strings.ToLower(col.Name)] = row[i]
		}
	}

	c.log.Debug("DESCRIBE AGENT raw columns", "columns", mapKeys(raw))
	if specJSON, ok := raw["agent_spec"]; ok {
		c.log.Debug("DESCRIBE AGENT agent_spec", "value", specJSON)
	}

	spec := agent.AgentSpec{}
	var unmappedSpecKeys []string
	if specJSON, ok := raw["agent_spec"]; ok {
		decoded, ok, err := decodeAgentSpecJSON(specJSON, spec, raw)
		if err != nil {
			return DescribeResult{}, err
		}
		if ok {
			spec = decoded
		}
		unmappedSpecKeys = detectUnmappedSpecKeys(specJSON)
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
			return DescribeResult{}, err
		}
	}

	return DescribeResult{
		Spec:             spec,
		Exists:           true,
		UnmappedColumns:  unmappedColumns(raw),
		UnmappedSpecKeys: unmappedSpecKeys,
		RawColumns:       raw,
	}, nil
}

// knownDescribeColumns are the SQL column names from DESCRIBE AGENT that
// the CLI knows how to handle.
var knownDescribeColumns = map[string]bool{
	"agent_spec":    true,
	"name":          true,
	"comment":       true,
	"profile":       true,
	"created_on":    true,
	"database_name": true,
	"owner":         true,
	"schema_name":   true,
}

// unmappedColumns returns column names from the DESCRIBE AGENT result that
// are not in the known set.
func unmappedColumns(raw map[string]any) []string {
	var cols []string
	for key := range raw {
		if !knownDescribeColumns[key] {
			cols = append(cols, key)
		}
	}
	sort.Strings(cols)
	return cols
}

// knownSpecKeys are the agent_spec JSON keys that the CLI maps to AgentSpec fields.
var knownSpecKeys = map[string]bool{
	"name":           true,
	"comment":        true,
	"profile":        true,
	"models":         true,
	"instructions":   true,
	"orchestration":  true,
	"tools":          true,
	"tool_resources": true,
}

// detectUnmappedSpecKeys parses the agent_spec JSON value and returns any
// top-level keys that are not in the known set (after normalizeAgentKey).
func detectUnmappedSpecKeys(specJSON any) []string {
	specStr, ok := specJSON.(string)
	if !ok || strings.TrimSpace(specStr) == "" {
		return nil
	}
	var specMap map[string]any
	if err := json.Unmarshal([]byte(specStr), &specMap); err != nil {
		return nil
	}
	var keys []string
	for key := range specMap {
		normalized := normalizeAgentKey(key)
		if !knownSpecKeys[normalized] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// mapKeys returns the keys of a map for debug output.
func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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
		case "tool_resources":
			if m, ok := value.(map[string]any); ok {
				out[normalizedKey] = normalizeToolResources(m)
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
		case strings.EqualFold(key, "avatar"):
			out["avatar"] = value
		case strings.EqualFold(key, "color"):
			out["color"] = value
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

// normalizeToolResources converts API response format to expected format.
// API response format: {"tool_name": [{"semantic_view": "...", ...}]} (array with single element).
// Expected format: {"tool_name": {"semantic_view": "...", ...}} (direct object).
func normalizeToolResources(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))

	for toolName, value := range input {
		switch v := value.(type) {
		case []any:
			// Array format - take first element
			if len(v) > 0 {
				if resource, ok := v[0].(map[string]any); ok {
					out[toolName] = resource
				}
			}
		case []map[string]any:
			// Array format - take first element
			if len(v) > 0 {
				out[toolName] = v[0]
			}
		case map[string]any:
			// Already in expected format
			out[toolName] = v
		default:
			out[toolName] = value
		}
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
