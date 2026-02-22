package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
)

// sqlStatementRequest is the request body for the Snowflake SQL Statement API.
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

// escapeSQLString escapes single quotes in a string for use in SQL string literals.
func escapeSQLString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// parseSnowflakeTimestamp converts a Snowflake SQL API timestamp string to a
// human-readable UTC time. The SQL API returns TIMESTAMP columns as Unix epoch
// seconds with a fractional part (e.g. "1771595930.421000000").
func parseSnowflakeTimestamp(raw string) string {
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return raw // not a numeric epoch — return as-is
	}
	sec := int64(f)
	nsec := int64((f - float64(sec)) * 1e9)
	return time.Unix(sec, nsec).UTC().Format("2006-01-02 15:04:05.000 UTC")
}

// FeedbackRecord holds a single CORTEX_AGENT_FEEDBACK observability event.
type FeedbackRecord struct {
	RecordID       string        `json:"record_id"`             // RECORD_ATTRIBUTES['ai.observability.record_id']
	Timestamp      string        `json:"timestamp"`
	AgentName      string        `json:"agent_name"`
	UserName       string        `json:"user_name"`
	Sentiment      string        `json:"sentiment"` // "positive", "negative", "unknown"
	Comment        string        `json:"comment"`
	Categories     []string      `json:"categories,omitempty"`
	Question       string        `json:"question,omitempty"`        // user's question from the associated REQUEST event
	Response       string        `json:"response,omitempty"`        // agent's response from the associated REQUEST event
	ToolUses       []ToolUseInfo `json:"tool_uses,omitempty"`       // tool invocations during agent response
	ResponseTimeMs int64         `json:"response_time_ms,omitempty"` // total agent response latency in ms
	RawRecord      string        `json:"raw_record"`                // VALUE column raw JSON (fallback display)
}

// ToolUseInfo captures a single tool invocation from the agent's response.
type ToolUseInfo struct {
	ToolType   string `json:"tool_type"`             // e.g., "cortex_analyst_text_to_sql"
	ToolName   string `json:"tool_name"`             // e.g., "sample_semantic_view"
	Query      string `json:"query,omitempty"`       // input.query if present
	ToolStatus string `json:"tool_status,omitempty"` // "success" or "error" from tool_result
	SQL        string `json:"sql,omitempty"`         // generated SQL from tool_result
}

// GetFeedback queries SNOWFLAKE.LOCAL.GET_AI_OBSERVABILITY_EVENTS for
// CORTEX_AGENT_FEEDBACK events and returns them ordered by timestamp descending.
// It LEFT JOINs the REQUEST events on ai.observability.record_id to include the
// user's question and the agent's response alongside each feedback record.
func (c *Client) GetFeedback(ctx context.Context, db, schema, agentName string) ([]FeedbackRecord, error) {
	dbEsc := escapeSQLString(unquoteIdentifier(db))
	schemaEsc := escapeSQLString(unquoteIdentifier(schema))
	agentEsc := escapeSQLString(agentName)

	stmt := fmt.Sprintf(
		"SELECT"+
			" f.TIMESTAMP,"+
			" f.RESOURCE_ATTRIBUTES,"+
			" f.RECORD_ATTRIBUTES AS FEEDBACK_ATTRS,"+
			" f.VALUE             AS FEEDBACK_VALUE,"+
			" r.VALUE             AS REQUEST_VALUE,"+
			" f.RECORD_ATTRIBUTES['ai.observability.record_id'] AS RECORD_ID"+
			" FROM TABLE(SNOWFLAKE.LOCAL.GET_AI_OBSERVABILITY_EVENTS('%s', '%s', '%s', 'CORTEX AGENT')) f"+
			" LEFT JOIN TABLE(SNOWFLAKE.LOCAL.GET_AI_OBSERVABILITY_EVENTS('%s', '%s', '%s', 'CORTEX AGENT')) r"+
			"   ON  f.RECORD_ATTRIBUTES['ai.observability.record_id']"+
			"     = r.RECORD_ATTRIBUTES['ai.observability.record_id']"+
			"   AND r.RECORD:name = 'CORTEX_AGENT_REQUEST'"+
			" WHERE f.RECORD:name = 'CORTEX_AGENT_FEEDBACK'"+
			" ORDER BY f.TIMESTAMP DESC",
		dbEsc, schemaEsc, agentEsc,
		dbEsc, schemaEsc, agentEsc,
	)

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
		return nil, err
	}

	colIndex := make(map[string]int)
	for i, col := range resp.ResultSetMetaData.RowType {
		colIndex[strings.ToLower(col.Name)] = i
	}

	var records []FeedbackRecord
	for _, row := range resp.Data {
		rec := FeedbackRecord{AgentName: agentName, Sentiment: "unknown"}

		if idx, ok := colIndex["timestamp"]; ok && idx < len(row) {
			if raw, ok := row[idx].(string); ok {
				rec.Timestamp = parseSnowflakeTimestamp(raw)
			}
		}

		// FEEDBACK_VALUE column contains the actual feedback payload:
		// {"positive": bool, "feedback_message": string, "categories": [...], "entity_type": string}
		if idx, ok := colIndex["feedback_value"]; ok && idx < len(row) {
			if valJSON, ok := row[idx].(string); ok && valJSON != "" {
				rec.RawRecord = valJSON
				var valMap map[string]any
				if err := json.Unmarshal([]byte(valJSON), &valMap); err == nil {
					if pos, ok := valMap["positive"].(bool); ok {
						if pos {
							rec.Sentiment = "positive"
						} else {
							rec.Sentiment = "negative"
						}
					}
					if msg, ok := valMap["feedback_message"].(string); ok {
						rec.Comment = msg
					}
					if cats, ok := valMap["categories"].([]any); ok {
						for _, c := range cats {
							if s, ok := c.(string); ok {
								rec.Categories = append(rec.Categories, s)
							}
						}
					}
				}
			}
		}

		// FEEDBACK_ATTRS contains agent and user identity fields.
		if idx, ok := colIndex["feedback_attrs"]; ok && idx < len(row) {
			if attrJSON, ok := row[idx].(string); ok && attrJSON != "" {
				var attrs map[string]any
				if err := json.Unmarshal([]byte(attrJSON), &attrs); err == nil {
					if name, ok := attrs["snow.ai.observability.object.name"].(string); ok && name != "" {
						rec.AgentName = name
					}
					if user, ok := attrs["snow.ai.observability.user.name"].(string); ok && user != "" {
						rec.UserName = user
					}
				}
			}
		}

		// Fallback: user name from RESOURCE_ATTRIBUTES.
		if rec.UserName == "" {
			if idx, ok := colIndex["resource_attributes"]; ok && idx < len(row) {
				if attrJSON, ok := row[idx].(string); ok && attrJSON != "" {
					var attrs map[string]any
					if err := json.Unmarshal([]byte(attrJSON), &attrs); err == nil {
						if user, ok := attrs["snow.user.name"].(string); ok && user != "" {
							rec.UserName = user
						}
					}
				}
			}
		}

		// REQUEST_VALUE comes from the LEFT JOIN on CORTEX_AGENT_REQUEST events.
		// It is null when no matching REQUEST event exists.
		if idx, ok := colIndex["request_value"]; ok && idx < len(row) {
			if reqJSON, ok := row[idx].(string); ok && reqJSON != "" {
				rec.Question = extractQuestion(reqJSON)
				rec.Response = extractResponse(reqJSON)
				rec.ToolUses = extractToolUses(reqJSON)
				rec.ResponseTimeMs = extractResponseTimeMs(reqJSON)
			}
		}

		// RECORD_ID is the stable Snowflake-issued UUID for this feedback event.
		if idx, ok := colIndex["record_id"]; ok && idx < len(row) {
			if rid, ok := row[idx].(string); ok {
				rec.RecordID = strings.Trim(rid, `"`)
			}
		}

		records = append(records, rec)
	}

	return records, nil
}

// CortexComplete calls SNOWFLAKE.CORTEX.COMPLETE via the SQL API and returns the
// raw text content from the model response. The caller provides the model name
// and the full SQL statement (so structured-output options can be embedded).
func (c *Client) CortexComplete(ctx context.Context, sqlStmt string) (string, error) {
	payload := sqlStatementRequest{
		Statement: sqlStmt,
	}
	if strings.TrimSpace(c.authCfg.Warehouse) != "" {
		payload.Warehouse = c.authCfg.Warehouse
	}
	if strings.TrimSpace(c.role) != "" {
		payload.Role = c.role
	}

	var resp sqlStatementResponse
	if err := c.doJSON(ctx, http.MethodPost, c.sqlURL(), payload, &resp); err != nil {
		return "", fmt.Errorf("cortex complete: %w", err)
	}

	if len(resp.Data) == 0 || len(resp.Data[0]) == 0 {
		return "", fmt.Errorf("cortex complete: empty response")
	}

	raw, ok := resp.Data[0][0].(string)
	if !ok {
		return "", fmt.Errorf("cortex complete: unexpected response type")
	}

	return raw, nil
}

// extractQuestion extracts the last user message text from a CORTEX_AGENT_REQUEST VALUE JSON.
// The VALUE has the shape:
//
//	{"snow.ai.observability.request_body": {"messages": [{"role": "user", "content": [{"type": "text", "text": "..."}]}]}}
func extractQuestion(requestJSON string) string {
	var valMap map[string]any
	if err := json.Unmarshal([]byte(requestJSON), &valMap); err != nil {
		return ""
	}
	reqBody, ok := valMap["snow.ai.observability.request_body"].(map[string]any)
	if !ok {
		return ""
	}
	messages, ok := reqBody["messages"].([]any)
	if !ok {
		return ""
	}
	// Find the last message with role == "user"
	var lastUserMsg map[string]any
	for _, msg := range messages {
		m, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		if role, ok := m["role"].(string); ok && role == "user" {
			lastUserMsg = m
		}
	}
	if lastUserMsg == nil {
		return ""
	}
	contents, ok := lastUserMsg["content"].([]any)
	if !ok {
		return ""
	}
	var parts []string
	for _, c := range contents {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if t, ok := cm["type"].(string); ok && t == "text" {
			if text, ok := cm["text"].(string); ok {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "")
}

// extractResponse extracts the assistant's text response from a CORTEX_AGENT_REQUEST VALUE JSON.
// The VALUE has the shape:
//
//	{"snow.ai.observability.response": "{\"content\":[{\"type\":\"text\",\"text\":\"...\"}],\"role\":\"assistant\"}"}
//
// The response field is a nested JSON string that must be unmarshalled a second time.
// Only content entries with type == "text" are included; tool_use / tool_result entries are skipped.
func extractResponse(requestJSON string) string {
	var valMap map[string]any
	if err := json.Unmarshal([]byte(requestJSON), &valMap); err != nil {
		return ""
	}
	responseRaw, ok := valMap["snow.ai.observability.response"].(string)
	if !ok || responseRaw == "" {
		return ""
	}
	var responseMap map[string]any
	if err := json.Unmarshal([]byte(responseRaw), &responseMap); err != nil {
		return ""
	}
	contents, ok := responseMap["content"].([]any)
	if !ok {
		return ""
	}
	var parts []string
	for _, c := range contents {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if t, ok := cm["type"].(string); ok && t == "text" {
			if text, ok := cm["text"].(string); ok {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "")
}

// extractToolUses extracts the ordered list of tool invocations from a
// CORTEX_AGENT_REQUEST VALUE JSON.
// It performs two passes over the content array: first to index tool_result blocks
// by tool_use_id, then to collect tool_use blocks and merge in status and SQL.
func extractToolUses(requestJSON string) []ToolUseInfo {
	var valMap map[string]any
	if err := json.Unmarshal([]byte(requestJSON), &valMap); err != nil {
		return nil
	}
	responseRaw, ok := valMap["snow.ai.observability.response"].(string)
	if !ok || responseRaw == "" {
		return nil
	}
	var responseMap map[string]any
	if err := json.Unmarshal([]byte(responseRaw), &responseMap); err != nil {
		return nil
	}
	contents, ok := responseMap["content"].([]any)
	if !ok {
		return nil
	}

	// First pass: index tool_result blocks by tool_use_id.
	type toolResult struct {
		status string
		sql    string
	}
	results := make(map[string]toolResult)
	for _, c := range contents {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if t, ok := cm["type"].(string); !ok || t != "tool_result" {
			continue
		}
		tr, ok := cm["tool_result"].(map[string]any)
		if !ok {
			continue
		}
		id, _ := tr["tool_use_id"].(string)
		if id == "" {
			continue
		}
		res := toolResult{}
		if s, ok := tr["status"].(string); ok {
			res.status = s
		}
		if conts, ok := tr["content"].([]any); ok {
			for _, cv := range conts {
				cvm, ok := cv.(map[string]any)
				if !ok {
					continue
				}
				if j, ok := cvm["json"].(map[string]any); ok {
					if sql, ok := j["sql"].(string); ok && sql != "" {
						res.sql = sql
						break
					}
				}
			}
		}
		results[id] = res
	}

	// Second pass: collect tool_use blocks and merge tool_result data.
	var uses []ToolUseInfo
	for _, c := range contents {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if t, ok := cm["type"].(string); !ok || t != "tool_use" {
			continue
		}
		tu, ok := cm["tool_use"].(map[string]any)
		if !ok {
			continue
		}
		info := ToolUseInfo{}
		if v, ok := tu["type"].(string); ok {
			info.ToolType = v
		}
		if v, ok := tu["name"].(string); ok {
			info.ToolName = v
		}
		if input, ok := tu["input"].(map[string]any); ok {
			if q, ok := input["query"].(string); ok {
				info.Query = q
			}
		}
		if id, ok := tu["tool_use_id"].(string); ok && id != "" {
			if res, ok := results[id]; ok {
				info.ToolStatus = res.status
				info.SQL = res.sql
			}
		}
		uses = append(uses, info)
	}
	return uses
}

// extractResponseTimeMs extracts the agent response latency from a CORTEX_AGENT_REQUEST VALUE JSON.
func extractResponseTimeMs(requestJSON string) int64 {
	var valMap map[string]any
	if err := json.Unmarshal([]byte(requestJSON), &valMap); err != nil {
		return 0
	}
	if v, ok := valMap["snow.ai.observability.response_time_ms"].(float64); ok {
		return int64(v)
	}
	return 0
}

// probeSentiment inspects a RECORD map for sentiment signals,
// checking the "attributes" sub-map first and then the top level.
func probeSentiment(rec map[string]any) string {
	if attrs, ok := rec["attributes"].(map[string]any); ok {
		if s := probeString(attrs, []string{"sentiment", "feedback_type", "rating", "thumbs_up"}); s != "" {
			return classifySentiment(s)
		}
	}
	if s := probeString(rec, []string{"sentiment", "feedback_type", "rating", "thumbs_up"}); s != "" {
		return classifySentiment(s)
	}
	return "unknown"
}

// classifySentiment maps a raw string value to "positive", "negative", or "unknown".
func classifySentiment(s string) string {
	lower := strings.ToLower(s)
	switch {
	case strings.Contains(lower, "neg"), lower == "false", lower == "0", lower == "thumbsdown", strings.Contains(lower, "bad"):
		return "negative"
	case strings.Contains(lower, "pos"), lower == "true", lower == "1", lower == "thumbsup", strings.Contains(lower, "good"):
		return "positive"
	}
	return "unknown"
}

// probeStringNested checks the "attributes" sub-map first, then the top level.
func probeStringNested(rec map[string]any, keys []string) string {
	if attrs, ok := rec["attributes"].(map[string]any); ok {
		if s := probeString(attrs, keys); s != "" {
			return s
		}
	}
	return probeString(rec, keys)
}

// probeString looks for one of the candidate keys (case-insensitive) in a map
// and returns the first string value found.
func probeString(m map[string]any, keys []string) string {
	for _, k := range keys {
		for mk, mv := range m {
			if strings.EqualFold(mk, k) {
				if s, ok := mv.(string); ok {
					return s
				}
				if mv != nil {
					return fmt.Sprintf("%v", mv)
				}
			}
		}
	}
	return ""
}
