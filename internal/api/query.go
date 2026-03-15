package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
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
	Data               [][]any `json:"data"`
	Code               string  `json:"code"`
	Message            string  `json:"message"`
	StatementHandle    string  `json:"statementHandle"`
	StatementStatusURL string  `json:"statementStatusUrl"`
	ResultSetMetaData  struct {
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

// escapeSQLJSONString escapes text that will be embedded in PARSE_JSON('<text>').
// Snowflake can interpret backslash escapes inside SQL string literals, so we
// preserve JSON escapes (e.g. "\n") by doubling backslashes first.
func escapeSQLJSONString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return escapeSQLString(s)
}

// parseSnowflakeTimestamp converts a Snowflake SQL API timestamp string to a
// human-readable UTC time. The SQL API returns TIMESTAMP columns as Unix epoch
// seconds with a fractional part (e.g. "1771595930.421000000").
func parseSnowflakeTimestamp(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	epochPart := raw
	// TIMESTAMP_TZ values can come back as "<epoch_seconds> <tz_token>".
	if parts := strings.Fields(raw); len(parts) > 0 {
		epochPart = parts[0]
	}
	f, err := strconv.ParseFloat(epochPart, 64)
	if err != nil {
		// Keep non-epoch timestamp strings readable and normalized when possible.
		if ts, parseErr := time.Parse(time.RFC3339Nano, raw); parseErr == nil {
			return ts.UTC().Format("2006-01-02 15:04:05.000 UTC")
		}
		return raw // not a numeric epoch — return as-is
	}
	sec := int64(f)
	nsec := int64((f - float64(sec)) * 1e9)
	return time.Unix(sec, nsec).UTC().Format("2006-01-02 15:04:05.000 UTC")
}

// sinceForSQL normalizes a UTC timestamp string (e.g. "2006-01-02 15:04:05.000 UTC")
// to the form used in TO_TIMESTAMP_TZ with TZHTZM, to avoid double-quote escaping
// issues when the SQL is sent via the REST API (e.g. "2006-01-02 15:04:05.000 +0000").
func sinceForSQL(since string) string {
	since = strings.TrimSpace(since)
	if since == "" {
		return ""
	}
	if strings.HasSuffix(since, " UTC") {
		return strings.TrimSuffix(since, " UTC") + " +0000"
	}
	return since
}

// normalizeFeedbackEventTimestamp prepares feedback timestamps for Snowflake
// staging/upsert by converting the CLI's normalized UTC suffix into an offset.
func normalizeFeedbackEventTimestamp(ts string) string {
	return sinceForSQL(ts)
}

// FeedbackRecord holds a single CORTEX_AGENT_FEEDBACK observability event.
type FeedbackRecord struct {
	RecordID        string        `json:"record_id"` // RECORD_ATTRIBUTES['ai.observability.record_id']
	Timestamp       string        `json:"timestamp"`
	AgentName       string        `json:"agent_name"`
	UserName        string        `json:"user_name"`
	Sentiment       string        `json:"sentiment"` // "positive", "negative", "unknown"
	SentimentSource string        `json:"sentiment_source,omitempty"`
	SentimentReason string        `json:"sentiment_reason,omitempty"`
	FeedbackMessage string        `json:"feedback_message"`
	Categories      []string      `json:"categories,omitempty"`
	Question        string        `json:"question,omitempty"`         // user's question from the associated REQUEST event
	Response        string        `json:"response,omitempty"`         // agent's response from the associated REQUEST event
	ToolUses        []ToolUseInfo `json:"tool_uses,omitempty"`        // tool invocations during agent response
	ResponseTimeMs  int64         `json:"response_time_ms,omitempty"` // total agent response latency in ms
	RawRecord       string        `json:"raw_record"`                 // VALUE column raw JSON (fallback display)
	RequestRaw      string        `json:"request_raw,omitempty"`      // CORTEX_AGENT_REQUEST VALUE raw JSON
}

// FeedbackQueryOptions controls optional behavior for feedback retrieval/sync.
type FeedbackQueryOptions struct {
	Since         string
	ExplicitSince string
	RequestSince  string
	InferNegative bool
	JudgeModel    string
}

type negativeInferenceResult struct {
	Negative  bool   `json:"negative"`
	Reasoning string `json:"reasoning"`
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
// CORTEX_AGENT_FEEDBACK events and optionally infers negative sentiment for
// request-only interactions when opts.InferNegative is enabled.
func (c *Client) GetFeedback(ctx context.Context, db, schema, agentName string, opts FeedbackQueryOptions) ([]FeedbackRecord, error) {
	explicitSince := opts.ExplicitSince
	if explicitSince == "" {
		explicitSince = opts.Since
	}
	explicit, err := c.getExplicitFeedback(ctx, db, schema, agentName, explicitSince)
	if err != nil {
		return nil, err
	}
	if !opts.InferNegative {
		return explicit, nil
	}

	requestSince := opts.RequestSince
	if requestSince == "" {
		requestSince = opts.Since
	}
	candidates, err := c.getRequestOnlyFeedbackCandidates(ctx, db, schema, agentName, requestSince)
	if err != nil {
		return nil, err
	}

	var inferred []FeedbackRecord
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.Question) == "" {
			continue
		}
		if result, ok := inferNegativeFeedbackHeuristically(candidate); ok {
			if result.Negative {
				candidate.Sentiment = "negative"
			} else {
				candidate.Sentiment = "positive"
			}
			candidate.SentimentSource = "inferred"
			candidate.SentimentReason = result.Reasoning
			inferred = append(inferred, candidate)
			continue
		}
		result, err := c.inferNegativeFeedback(ctx, opts.JudgeModel, candidate)
		if err != nil {
			return nil, err
		}
		if result.Negative {
			candidate.Sentiment = "negative"
		} else {
			candidate.Sentiment = "positive"
		}
		candidate.SentimentSource = "inferred"
		candidate.SentimentReason = result.Reasoning
		inferred = append(inferred, candidate)
	}

	return mergeFeedbackRecords(explicit, inferred, true), nil
}

func (c *Client) getExplicitFeedback(ctx context.Context, db, schema, agentName, since string) ([]FeedbackRecord, error) {
	dbEsc := escapeSQLString(unquoteIdentifier(db))
	schemaEsc := escapeSQLString(unquoteIdentifier(schema))
	agentEsc := escapeSQLString(agentName)

	whereExtra := ""
	if since != "" {
		sinceEsc := escapeSQLString(sinceForSQL(since))
		whereExtra = fmt.Sprintf(" AND f.TIMESTAMP >= TO_TIMESTAMP_TZ('%s', 'YYYY-MM-DD HH24:MI:SS.FF3 TZHTZM')", sinceEsc)
	}

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
			"%s"+
			" ORDER BY f.TIMESTAMP DESC",
		dbEsc, schemaEsc, agentEsc,
		dbEsc, schemaEsc, agentEsc,
		whereExtra,
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
						rec.FeedbackMessage = msg
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
				rec.RequestRaw = reqJSON
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

func (c *Client) getRequestOnlyFeedbackCandidates(ctx context.Context, db, schema, agentName, since string) ([]FeedbackRecord, error) {
	dbEsc := escapeSQLString(unquoteIdentifier(db))
	schemaEsc := escapeSQLString(unquoteIdentifier(schema))
	agentEsc := escapeSQLString(agentName)
	whereExtra := ""
	if since != "" {
		sinceEsc := escapeSQLString(sinceForSQL(since))
		whereExtra = fmt.Sprintf(" AND r.TIMESTAMP >= TO_TIMESTAMP_TZ('%s', 'YYYY-MM-DD HH24:MI:SS.FF3 TZHTZM')", sinceEsc)
	}

	stmt := fmt.Sprintf(
		"SELECT"+
			" r.TIMESTAMP,"+
			" r.RESOURCE_ATTRIBUTES,"+
			" r.RECORD_ATTRIBUTES AS REQUEST_ATTRS,"+
			" r.VALUE             AS REQUEST_VALUE,"+
			" r.RECORD_ATTRIBUTES['ai.observability.record_id'] AS RECORD_ID"+
			" FROM TABLE(SNOWFLAKE.LOCAL.GET_AI_OBSERVABILITY_EVENTS('%s', '%s', '%s', 'CORTEX AGENT')) r"+
			" LEFT JOIN TABLE(SNOWFLAKE.LOCAL.GET_AI_OBSERVABILITY_EVENTS('%s', '%s', '%s', 'CORTEX AGENT')) f"+
			"   ON  r.RECORD_ATTRIBUTES['ai.observability.record_id']"+
			"     = f.RECORD_ATTRIBUTES['ai.observability.record_id']"+
			"   AND f.RECORD:name = 'CORTEX_AGENT_FEEDBACK'"+
			" WHERE r.RECORD:name = 'CORTEX_AGENT_REQUEST'"+
			"   AND f.RECORD:name IS NULL"+
			"%s"+
			" ORDER BY r.TIMESTAMP DESC",
		dbEsc, schemaEsc, agentEsc,
		dbEsc, schemaEsc, agentEsc,
		whereExtra,
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
		rec := FeedbackRecord{
			AgentName:       agentName,
			Sentiment:       "unknown",
			SentimentSource: "inferred",
		}

		if idx, ok := colIndex["timestamp"]; ok && idx < len(row) {
			if raw, ok := row[idx].(string); ok {
				rec.Timestamp = parseSnowflakeTimestamp(raw)
			}
		}
		if idx, ok := colIndex["request_attrs"]; ok && idx < len(row) {
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
		if idx, ok := colIndex["request_value"]; ok && idx < len(row) {
			if reqJSON, ok := row[idx].(string); ok && reqJSON != "" {
				rec.RequestRaw = reqJSON
				rec.Question = extractQuestion(reqJSON)
				rec.Response = extractResponse(reqJSON)
				rec.ToolUses = extractToolUses(reqJSON)
				rec.ResponseTimeMs = extractResponseTimeMs(reqJSON)
			}
		}
		if idx, ok := colIndex["record_id"]; ok && idx < len(row) {
			if rid, ok := row[idx].(string); ok {
				rec.RecordID = strings.Trim(rid, `"`)
			}
		}

		records = append(records, rec)
	}

	return records, nil
}

func mergeFeedbackRecords(explicit, inferred []FeedbackRecord, includeInferred bool) []FeedbackRecord {
	if !includeInferred {
		return explicit
	}
	seen := make(map[string]struct{}, len(explicit))
	merged := append([]FeedbackRecord{}, explicit...)
	for _, rec := range explicit {
		seen[rec.RecordID] = struct{}{}
	}
	for _, rec := range inferred {
		if _, ok := seen[rec.RecordID]; ok {
			continue
		}
		merged = append(merged, rec)
		seen[rec.RecordID] = struct{}{}
	}
	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i].Timestamp > merged[j].Timestamp
	})
	return merged
}

// CortexComplete calls SNOWFLAKE.CORTEX.AI_COMPLETE via the SQL API and returns the
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

	raw, err := sqlCellString(resp.Data[0][0])
	if err != nil {
		return "", fmt.Errorf("cortex complete: %w", err)
	}

	return raw, nil
}

func sqlCellString(v any) (string, error) {
	switch val := v.(type) {
	case nil:
		return "", fmt.Errorf("null response")
	case string:
		return val, nil
	case []byte:
		return string(val), nil
	default:
		encoded, err := json.Marshal(val)
		if err != nil {
			return "", fmt.Errorf("unexpected response type %T", v)
		}
		return string(encoded), nil
	}
}

func (c *Client) inferNegativeFeedback(ctx context.Context, model string, record FeedbackRecord) (negativeInferenceResult, error) {
	toolSummary := summarizeToolUses(record.ToolUses)
	if strings.TrimSpace(model) == "" {
		model = "llama4-scout"
	}
	prompt := fmt.Sprintf(
		"You are classifying whether an agent interaction should be treated as implicit negative feedback.\n\n"+
			"Question: %s\n\nAgent Response: %s\n\nTool Summary: %s\n\n"+
			"Return negative=true only when the response clearly failed to achieve the user's goal, was materially unhelpful, refused without solving the need, or otherwise left the request substantially unmet. "+
			"If the agent says the requested data is unavailable, missing, not recorded, or only offers an alternative timeframe/source instead of answering the original request, that is negative because the user's goal was unmet. "+
			"Do not mark negative for minor wording issues or partially useful answers that still satisfy the request. Provide a brief reasoning.",
		record.Question,
		record.Response,
		toolSummary,
	)
	escapedPrompt := strings.ReplaceAll(prompt, "'", "''")
	stmt := fmt.Sprintf(`SELECT SNOWFLAKE.CORTEX.AI_COMPLETE(
    model => '%s',
    prompt => '%s',
    model_parameters => {
        'temperature': 0
    },
    response_format => {
        'type': 'json',
        'schema': {
            'type': 'object',
            'properties': {
                'negative': {'type': 'boolean'},
                'reasoning': {'type': 'string'}
            },
            'required': ['negative', 'reasoning']
        }
    },
    show_details => TRUE
) AS response;`, model, escapedPrompt)

	raw, err := c.CortexComplete(ctx, stmt)
	if err != nil {
		return negativeInferenceResult{}, err
	}
	return parseNegativeInferenceResponse(raw)
}

func inferNegativeFeedbackHeuristically(record FeedbackRecord) (negativeInferenceResult, bool) {
	response := strings.ToLower(strings.TrimSpace(record.Response))
	if response == "" {
		return negativeInferenceResult{
			Negative:  true,
			Reasoning: "The agent did not provide a substantive answer to the user's request.",
		}, true
	}

	hardFailurePhrases := []string{
		"i can't answer", "i cannot answer", "cannot provide", "can't provide",
		"unable to provide", "not enough information", "insufficient information",
		"回答できません", "お答えできません", "提供できません", "情報がありません",
	}
	for _, phrase := range hardFailurePhrases {
		if strings.Contains(response, phrase) {
			return negativeInferenceResult{
				Negative:  true,
				Reasoning: "The response explicitly says the agent could not answer the user's request.",
			}, true
		}
	}

	unavailableDataPhrases := []string{
		"no data", "data does not exist", "data is unavailable", "not available",
		"not recorded", "not in the database", "database does not contain",
		"doesn't exist", "does not exist", "missing data",
		"データが存在しません", "データは存在しません", "データがありません",
		"データベースには", "記録されていない", "まだ記録されていない", "利用可能なデータは",
	}
	for _, phrase := range unavailableDataPhrases {
		if strings.Contains(response, phrase) {
			return negativeInferenceResult{
				Negative:  true,
				Reasoning: "The response says the requested data or information is unavailable, so the original request was not fulfilled.",
			}, true
		}
	}

	return negativeInferenceResult{}, false
}

func parseNegativeInferenceResponse(raw string) (negativeInferenceResult, error) {
	var direct negativeInferenceResult
	if err := json.Unmarshal([]byte(raw), &direct); err == nil {
		if direct.Reasoning != "" || direct.Negative {
			return direct, nil
		}
	}

	var completeResp struct {
		StructuredOutput []struct {
			RawMessage negativeInferenceResult `json:"raw_message"`
		} `json:"structured_output"`
	}
	if err := json.Unmarshal([]byte(raw), &completeResp); err != nil {
		return negativeInferenceResult{}, fmt.Errorf("parse negative inference response: %w", err)
	}
	if len(completeResp.StructuredOutput) == 0 {
		return negativeInferenceResult{}, fmt.Errorf("no structured_output in complete response")
	}
	return completeResp.StructuredOutput[0].RawMessage, nil
}

func summarizeToolUses(toolUses []ToolUseInfo) string {
	if len(toolUses) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(toolUses))
	for _, tu := range toolUses {
		name := strings.TrimSpace(tu.ToolName)
		typ := strings.TrimSpace(tu.ToolType)
		switch {
		case typ != "" && name != "":
			parts = append(parts, typ+" ("+name+")")
		case name != "":
			parts = append(parts, name)
		case typ != "":
			parts = append(parts, typ)
		}
	}
	if len(parts) == 0 {
		return "(none)"
	}
	return strings.Join(parts, ", ")
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

// FeedbackTableRow is a feedback record as stored in the remote table, including checked state.
type FeedbackTableRow struct {
	FeedbackRecord
	Checked   bool   `json:"checked"`
	CheckedAt string `json:"checked_at,omitempty"`
}

// FeedbackTableExists returns true if the given table exists in the database and schema.
// It uses SHOW TABLES for compatibility with Snowflake metadata visibility.
func (c *Client) FeedbackTableExists(ctx context.Context, db, schema, table string) (bool, error) {
	stmt := fmt.Sprintf(
		"SHOW TABLES LIKE '%s' IN SCHEMA %s.%s",
		escapeSQLString(unquoteIdentifier(table)),
		identifierSegment(db),
		identifierSegment(schema),
	)
	resp, err := c.executeStatement(ctx, db, schema, stmt)
	if err != nil {
		return false, err
	}
	if len(resp.Data) == 0 {
		return false, nil
	}
	colIndex := make(map[string]int)
	for i, col := range resp.ResultSetMetaData.RowType {
		colIndex[strings.ToLower(col.Name)] = i
	}
	nameIdx, ok := colIndex["name"]
	if !ok {
		// SHOW TABLES normally returns rows only when the table exists; if schema is unexpected,
		// fallback to row presence.
		return len(resp.Data) > 0, nil
	}
	for _, row := range resp.Data {
		if nameIdx < len(row) {
			if name, ok := row[nameIdx].(string); ok && strings.EqualFold(name, unquoteIdentifier(table)) {
				return true, nil
			}
		}
	}
	return false, nil
}

func (c *Client) feedbackTableColumnSet(ctx context.Context, db, schema, table string) (map[string]struct{}, error) {
	fq := fmt.Sprintf("%s.%s.%s",
		identifierSegment(db), identifierSegment(schema), identifierSegment(table))
	stmt := fmt.Sprintf("SHOW COLUMNS IN TABLE %s", fq)
	resp, err := c.executeStatement(ctx, db, schema, stmt)
	if err != nil {
		return nil, err
	}
	colIndex := make(map[string]int)
	for i, col := range resp.ResultSetMetaData.RowType {
		colIndex[strings.ToLower(col.Name)] = i
	}
	nameIdx, ok := colIndex["column_name"]
	if !ok {
		nameIdx, ok = colIndex["name"]
	}
	if !ok {
		return map[string]struct{}{}, nil
	}
	colSet := make(map[string]struct{}, len(resp.Data))
	for _, row := range resp.Data {
		if nameIdx >= len(row) {
			continue
		}
		if name, ok := row[nameIdx].(string); ok {
			colSet[strings.ToLower(strings.Trim(name, `"`))] = struct{}{}
		}
	}
	return colSet, nil
}

// CreateFeedbackTable creates or replaces the feedback persistence table.
// Table columns: record_id, event_ts, agent_name, user_name, sentiment,
// sentiment_source, sentiment_reason, feedback_message, categories, question,
// response, response_time_ms, tool_uses, request_value, checked, checked_at,
// created_at, updated_at.
func (c *Client) CreateFeedbackTable(ctx context.Context, db, schema, table string) error {
	fq := fmt.Sprintf("%s.%s.%s",
		identifierSegment(db), identifierSegment(schema), identifierSegment(table))
	stmt := fmt.Sprintf(`CREATE OR REPLACE TABLE %s (
  record_id VARCHAR PRIMARY KEY,
  event_ts TIMESTAMP_TZ,
  agent_name VARCHAR,
  user_name VARCHAR,
  sentiment VARCHAR,
  sentiment_source VARCHAR,
  sentiment_reason VARCHAR,
  feedback_message VARCHAR,
  categories VARCHAR,
  question VARCHAR,
  response VARCHAR,
  response_time_ms NUMBER,
  tool_uses VARIANT,
  request_value VARIANT,
  checked BOOLEAN DEFAULT FALSE,
  checked_at TIMESTAMP_TZ,
  created_at TIMESTAMP_TZ DEFAULT CURRENT_TIMESTAMP(),
  updated_at TIMESTAMP_TZ DEFAULT CURRENT_TIMESTAMP()
)`, fq)
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
	return c.doJSON(ctx, http.MethodPost, c.sqlURL(), payload, nil)
}

// RenameFeedbackTable renames an existing feedback table within the same schema.
func (c *Client) RenameFeedbackTable(ctx context.Context, db, schema, fromTable, toTable string) error {
	fq := fmt.Sprintf("%s.%s.%s",
		identifierSegment(db), identifierSegment(schema), identifierSegment(fromTable))
	stmt := fmt.Sprintf("ALTER TABLE %s RENAME TO %s", fq, identifierSegment(toTable))
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
	return c.doJSON(ctx, http.MethodPost, c.sqlURL(), payload, nil)
}

func (c *Client) FeedbackInferenceColumnsExist(ctx context.Context, db, schema, table string) (bool, error) {
	colSet, err := c.feedbackTableColumnSet(ctx, db, schema, table)
	if err != nil {
		return false, err
	}
	_, hasSource := colSet["sentiment_source"]
	_, hasReason := colSet["sentiment_reason"]
	return hasSource && hasReason, nil
}

// executeStatement runs a single statement in the given database and schema.
func (c *Client) executeStatement(ctx context.Context, db, schema, stmt string) (*sqlStatementResponse, error) {
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
	// Long-running SQL statements can return 202 with statementStatusUrl.
	// Poll only while Snowflake reports the statement is still in progress.
	isInProgress := func(r sqlStatementResponse) bool {
		switch r.Code {
		case "333333", "333334":
			return true
		default:
			return false
		}
	}
	for resp.StatementStatusURL != "" && isInProgress(resp) {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("wait statement completion: %w", ctx.Err())
		case <-time.After(300 * time.Millisecond):
		}
		statusURL := resp.StatementStatusURL
		if !strings.HasPrefix(statusURL, "http://") && !strings.HasPrefix(statusURL, "https://") {
			base := *c.baseURL
			ref, err := url.Parse(statusURL)
			if err != nil {
				return nil, fmt.Errorf("parse statement status url: %w", err)
			}
			statusURL = base.ResolveReference(ref).String()
		}
		if err := c.doJSON(ctx, http.MethodGet, statusURL, nil, &resp); err != nil {
			return nil, err
		}
	}
	return &resp, nil
}

// UpsertFeedbackRecords inserts or updates feedback records in the remote table.
// Existing rows keep their checked state while mutable feedback fields are refreshed.
// It stages rows into a transient table and executes a single MERGE for better throughput.
func (c *Client) UpsertFeedbackRecords(ctx context.Context, db, schema, table string, records []FeedbackRecord) error {
	if len(records) == 0 {
		return nil
	}

	fq := fmt.Sprintf("%s.%s.%s",
		identifierSegment(db), identifierSegment(schema), identifierSegment(table))

	tmpTable := fmt.Sprintf("TMP_FEEDBACK_STAGE_%d", time.Now().UnixNano())
	tmpFQ := fmt.Sprintf("%s.%s.%s",
		identifierSegment(db), identifierSegment(schema), identifierSegment(tmpTable))

	// SQL API calls can run on different sessions, so TEMP tables are not reliable
	// across create/insert/merge statements. Use a uniquely named transient stage table.
	// Keep stage columns as scalar/text, and parse/cast in the MERGE's USING SELECT.
	createTmpStmt := fmt.Sprintf(`CREATE TRANSIENT TABLE %s (
  record_id VARCHAR,
  event_ts_raw VARCHAR,
  agent_name VARCHAR,
  user_name VARCHAR,
  sentiment VARCHAR,
  sentiment_source VARCHAR,
  sentiment_reason VARCHAR,
  feedback_message VARCHAR,
  categories_raw VARCHAR,
  question VARCHAR,
  response VARCHAR,
  response_time_ms NUMBER,
  tool_uses_raw VARCHAR,
  request_value_raw VARCHAR
)`, tmpFQ)
	if _, err := c.executeStatement(ctx, db, schema, createTmpStmt); err != nil {
		return fmt.Errorf("create temporary feedback stage table: %w", err)
	}
	defer func() {
		// Best-effort cleanup; temporary tables are session-scoped and auto-dropped.
		dropStmt := fmt.Sprintf("DROP TABLE IF EXISTS %s", tmpFQ)
		_, _ = c.executeStatement(context.Background(), db, schema, dropStmt)
	}()

	const batchSize = 200
	for start := 0; start < len(records); start += batchSize {
		end := start + batchSize
		if end > len(records) {
			end = len(records)
		}

		var selects []string
		for _, r := range records[start:end] {
			// Build one staging SELECT row with raw scalar/text values.
			rid := r.RecordID
			if rid == "" {
				rid = r.Timestamp + "|" + r.UserName
			}
			eventTsRaw := "NULL"
			if r.Timestamp != "" {
				eventTsRaw = fmt.Sprintf("'%s'::VARCHAR", escapeSQLString(normalizeFeedbackEventTimestamp(r.Timestamp)))
			}
			categoriesRaw := "NULL"
			if len(r.Categories) > 0 {
				b, _ := json.Marshal(r.Categories)
				categoriesRaw = fmt.Sprintf("'%s'::VARCHAR", escapeSQLJSONString(string(b)))
			}
			toolUsesRaw := "NULL"
			if len(r.ToolUses) > 0 {
				b, _ := json.Marshal(r.ToolUses)
				toolUsesRaw = fmt.Sprintf("'%s'::VARCHAR", escapeSQLJSONString(string(b)))
			}
			requestValueRaw := "NULL"
			if r.RequestRaw != "" {
				requestValueRaw = fmt.Sprintf("'%s'::VARCHAR", escapeSQLJSONString(r.RequestRaw))
			}
			selects = append(selects, fmt.Sprintf(
				`SELECT '%s'::VARCHAR AS record_id, %s AS event_ts_raw, '%s'::VARCHAR AS agent_name, '%s'::VARCHAR AS user_name, '%s'::VARCHAR AS sentiment, '%s'::VARCHAR AS sentiment_source, '%s'::VARCHAR AS sentiment_reason, '%s'::VARCHAR AS feedback_message, %s AS categories_raw, '%s'::VARCHAR AS question, '%s'::VARCHAR AS response, %d::NUMBER AS response_time_ms, %s AS tool_uses_raw, %s AS request_value_raw`,
				escapeSQLString(rid),
				eventTsRaw,
				escapeSQLString(r.AgentName),
				escapeSQLString(r.UserName),
				escapeSQLString(r.Sentiment),
				escapeSQLString(r.SentimentSource),
				escapeSQLString(r.SentimentReason),
				escapeSQLString(r.FeedbackMessage),
				categoriesRaw,
				escapeSQLString(r.Question),
				escapeSQLString(r.Response),
				r.ResponseTimeMs,
				toolUsesRaw,
				requestValueRaw,
			))
		}

		insertStmt := fmt.Sprintf(
			`INSERT INTO %s (record_id, event_ts_raw, agent_name, user_name, sentiment, sentiment_source, sentiment_reason, feedback_message, categories_raw, question, response, response_time_ms, tool_uses_raw, request_value_raw)
%s`,
			tmpFQ,
			strings.Join(selects, "\nUNION ALL\n"),
		)
		if _, err := c.executeStatement(ctx, db, schema, insertStmt); err != nil {
			return fmt.Errorf("stage feedback records (%d-%d): %w", start, end-1, err)
		}
	}

	mergeStmt := fmt.Sprintf(
		`MERGE INTO %s AS t USING (
  SELECT
    record_id,
    COALESCE(
      TRY_TO_TIMESTAMP_TZ(event_ts_raw, 'YYYY-MM-DD HH24:MI:SS.FF3 TZHTZM'),
      TRY_TO_TIMESTAMP_TZ(event_ts_raw),
      TRY_TO_TIMESTAMP_NTZ(event_ts_raw)::TIMESTAMP_TZ
    ) AS event_ts,
    agent_name,
    user_name,
    sentiment,
    sentiment_source,
    sentiment_reason,
    feedback_message,
    TRY_PARSE_JSON(categories_raw) AS categories,
    question,
    response,
    response_time_ms,
    TRY_PARSE_JSON(tool_uses_raw) AS tool_uses,
    TRY_PARSE_JSON(request_value_raw) AS request_value
  FROM %s
) AS s ON t.record_id = s.record_id
WHEN MATCHED THEN UPDATE SET
  event_ts = s.event_ts,
  agent_name = s.agent_name,
  user_name = s.user_name,
  sentiment = s.sentiment,
  sentiment_source = s.sentiment_source,
  sentiment_reason = s.sentiment_reason,
  feedback_message = s.feedback_message,
  categories = s.categories,
  question = s.question,
  response = s.response,
  response_time_ms = s.response_time_ms,
  tool_uses = s.tool_uses,
  request_value = s.request_value,
  updated_at = CURRENT_TIMESTAMP()
WHEN NOT MATCHED THEN INSERT (record_id, event_ts, agent_name, user_name, sentiment, sentiment_source, sentiment_reason, feedback_message, categories, question, response, response_time_ms, tool_uses, request_value)
VALUES (s.record_id, s.event_ts, s.agent_name, s.user_name, s.sentiment, s.sentiment_source, s.sentiment_reason, s.feedback_message, s.categories, s.question, s.response, s.response_time_ms, s.tool_uses, s.request_value)`,
		fq, tmpFQ,
	)
	if _, err := c.executeStatement(ctx, db, schema, mergeStmt); err != nil {
		return fmt.Errorf("merge staged feedback records: %w", err)
	}

	return nil
}

// SyncFeedbackFromEventsToTable merges feedback rows directly from observability
// events into the remote feedback table without Go-side row materialization by
// default. When opts.InferNegative is enabled, it falls back to Go-side
// materialization so request-only interactions can be classified and upserted.
func (c *Client) SyncFeedbackFromEventsToTable(ctx context.Context, srcDB, srcSchema, agentName, dstDB, dstSchema, dstTable string, opts FeedbackQueryOptions) error {
	if opts.InferNegative {
		ok, err := c.FeedbackInferenceColumnsExist(ctx, dstDB, dstSchema, dstTable)
		if err != nil {
			return fmt.Errorf("check inference columns: %w", err)
		}
		if !ok {
			return fmt.Errorf("remote feedback table %s.%s.%s is missing infer-negative columns; run `coragent feedback --init` to recreate it", dstDB, dstSchema, dstTable)
		}
		records, err := c.GetFeedback(ctx, srcDB, srcSchema, agentName, FeedbackQueryOptions{
			Since:         opts.Since,
			InferNegative: true,
			JudgeModel:    opts.JudgeModel,
		})
		if err != nil {
			return err
		}
		return c.UpsertFeedbackRecords(ctx, dstDB, dstSchema, dstTable, records)
	}

	dstFQ := fmt.Sprintf("%s.%s.%s",
		identifierSegment(dstDB), identifierSegment(dstSchema), identifierSegment(dstTable))
	srcDBEsc := escapeSQLString(unquoteIdentifier(srcDB))
	srcSchemaEsc := escapeSQLString(unquoteIdentifier(srcSchema))
	agentEsc := escapeSQLString(agentName)

	srcWhereExtra := ""
	if opts.Since != "" {
		sinceEsc := escapeSQLString(sinceForSQL(opts.Since))
		srcWhereExtra = fmt.Sprintf(" AND f.event_ts >= TO_TIMESTAMP_TZ('%s', 'YYYY-MM-DD HH24:MI:SS.FF3 TZHTZM')", sinceEsc)
	}

	stmt := fmt.Sprintf(
		`MERGE INTO %s AS tgt
USING (
WITH all_events AS (
    SELECT *
    FROM TABLE(SNOWFLAKE.LOCAL.GET_AI_OBSERVABILITY_EVENTS('%s', '%s', '%s', 'CORTEX AGENT'))
  ),

  transformed_events AS (
    SELECT
      RECORD_ATTRIBUTES['ai.observability.record_id']::STRING AS record_id,
      RECORD:name::STRING AS event_name,
      TIMESTAMP::TIMESTAMP_TZ AS event_ts,
      RECORD_ATTRIBUTES['snow.ai.observability.object.name']::STRING AS agent_name,
      COALESCE(
        RECORD_ATTRIBUTES['snow.ai.observability.user.name']::STRING,
        RESOURCE_ATTRIBUTES['snow.user.name']::STRING
      ) AS user_name,
      CASE WHEN TRY_TO_BOOLEAN(VALUE:positive::STRING) THEN 'positive' ELSE 'negative' END AS sentiment,
      VALUE:feedback_message::STRING AS feedback_message,
      TO_JSON(VALUE:categories) AS categories,
      TRY_TO_NUMBER(VALUE:"snow.ai.observability.response_time_ms"::STRING) AS response_time_ms,
      TRY_PARSE_JSON(VALUE:"snow.ai.observability.response"::STRING) AS request_response_json,
      VALUE,
    FROM all_events
  	WHERE event_name IN ('CORTEX_AGENT_FEEDBACK', 'CORTEX_AGENT_REQUEST')
  ),

  joined_feedback_and_request AS (
    SELECT
      f.record_id,
      f.event_name,
      f.event_ts,
      f.agent_name,
      f.user_name,
      f.sentiment,
      f.feedback_message,
      f.categories,
      r.response_time_ms,
      r.VALUE AS request_value,
      r.request_response_json,
    FROM transformed_events f
    LEFT JOIN transformed_events r
      ON f.record_id = r.record_id
  	WHERE f.event_name = 'CORTEX_AGENT_FEEDBACK'
      AND r.event_name = 'CORTEX_AGENT_REQUEST'
      -- where clause to filter events by timestamp
	    %s
  ),

  question_agg AS (
    SELECT
      s.record_id,
      LISTAGG(msg_part.value:text::STRING, '') WITHIN GROUP (ORDER BY msg.index, msg_part.index) AS question,
    FROM joined_feedback_and_request s,
         LATERAL FLATTEN(input => s.request_value:"snow.ai.observability.request_body":messages) msg,
         LATERAL FLATTEN(input => msg.value:content) msg_part
    WHERE msg.value:role::STRING = 'user'
      AND msg_part.value:type::STRING = 'text'
    GROUP BY s.record_id
  ),

  response_agg AS (
    SELECT
      s.record_id,
      LISTAGG(resp_part.value:text::STRING, '') WITHIN GROUP (ORDER BY resp_part.index) AS response,
    FROM joined_feedback_and_request s,
         LATERAL FLATTEN(input => s.request_response_json:content) resp_part
    WHERE resp_part.value:type::STRING = 'text'
    GROUP BY s.record_id
  ),

  tool_use_rows AS (
    SELECT
      s.record_id,
      ROW_NUMBER() OVER (PARTITION BY s.record_id ORDER BY tool_part.index) AS tool_ordinal,
      tool_part.value:tool_use:tool_use_id::STRING AS tool_use_id,
      tool_part.value:tool_use:type::STRING AS tool_type,
      tool_part.value:tool_use:name::STRING AS tool_name,
      tool_part.value:tool_use:input:query::STRING AS query,
      tool_part.value:tool_use:input:previous_related_tool_result_id::STRING AS previous_related_tool_result_id,
    FROM joined_feedback_and_request s,
         LATERAL FLATTEN(input => s.request_response_json:content) tool_part
    WHERE tool_part.value:type::STRING = 'tool_use'
  ),

  tool_result_rows AS (
    SELECT
      s.record_id,
      result_part.value:tool_result:tool_use_id::STRING AS tool_use_id,
      result_part.value:tool_result:type::STRING AS tool_result_type,
      result_part.value:tool_result:status::STRING AS tool_status,
      result_part.value:tool_result AS result_payload,
    FROM joined_feedback_and_request s,
         LATERAL FLATTEN(input => s.request_response_json:content) result_part
    WHERE result_part.value:type::STRING = 'tool_result'
  ),

  tool_result_agg AS (
    SELECT
      tr.record_id,
      tr.tool_use_id,
      tr.tool_status,
      LISTAGG(result_content.value:json:sql::STRING, ',') AS sql,
    FROM tool_result_rows tr,
         LATERAL FLATTEN(input => tr.result_payload:content) result_content
    GROUP BY tr.record_id, tr.tool_use_id, tr.tool_status
  ),

  tool_agg AS (
    SELECT
      tu.record_id,
      ARRAY_AGG(
        OBJECT_CONSTRUCT(
          'tool_type', tu.tool_type,
          'tool_name', tu.tool_name,
          'query', tu.query,
          'tool_status', tr.tool_status,
          'sql', tr.sql
        )
      ) WITHIN GROUP (ORDER BY tu.tool_ordinal) AS tool_uses
    FROM tool_use_rows tu
    LEFT JOIN tool_result_agg tr
      ON tr.record_id = tu.record_id
      AND tr.tool_use_id = tu.tool_use_id
    GROUP BY tu.record_id
  ),

  enriched_feedback_and_request AS (
    SELECT
      s.*,
      q.question,
      r.response,
      tu.tool_uses
    FROM joined_feedback_and_request s
    LEFT JOIN tool_agg tu ON tu.record_id = s.record_id
    LEFT JOIN question_agg q ON q.record_id = s.record_id
    LEFT JOIN response_agg r ON r.record_id = s.record_id
  ),

  final AS (
    SELECT
      record_id,
      event_ts,
      agent_name,
      user_name,
      sentiment,
      feedback_message,
      categories,
      question,
      response,
      response_time_ms,
      tool_uses,
      request_value,
    FROM enriched_feedback_and_request
  )

  SELECT * FROM final
) AS s
ON tgt.record_id = s.record_id
WHEN NOT MATCHED THEN INSERT (
  record_id, event_ts, agent_name, user_name, sentiment, feedback_message, categories, question, response, response_time_ms, tool_uses, request_value
)
VALUES (
  s.record_id, s.event_ts, s.agent_name, s.user_name, s.sentiment, s.feedback_message, s.categories, s.question, s.response, s.response_time_ms, s.tool_uses, s.request_value
)`,
		dstFQ,
		srcDBEsc, srcSchemaEsc, agentEsc,
		srcWhereExtra,
	)
	_, err := c.executeStatement(ctx, dstDB, dstSchema, stmt)
	return err
}

// GetLatestFeedbackEventTs returns the latest event_ts for the given agent in the
// remote feedback table, in normalized UTC string form, or empty string if no rows.
func (c *Client) GetLatestFeedbackEventTs(ctx context.Context, db, schema, table, agentName string) (string, error) {
	fq := fmt.Sprintf("%s.%s.%s",
		identifierSegment(db), identifierSegment(schema), identifierSegment(table))
	agentEsc := escapeSQLString(unquoteIdentifier(agentName))
	stmt := fmt.Sprintf(
		`SELECT MAX(event_ts) AS max_ts FROM %s WHERE agent_name = '%s'`,
		fq, agentEsc)
	resp, err := c.executeStatement(ctx, db, schema, stmt)
	if err != nil {
		return "", err
	}
	if len(resp.Data) == 0 || len(resp.Data[0]) == 0 {
		return "", nil
	}
	colIndex := make(map[string]int)
	for i, col := range resp.ResultSetMetaData.RowType {
		colIndex[strings.ToLower(col.Name)] = i
	}
	idx, ok := colIndex["max_ts"]
	if !ok || idx >= len(resp.Data[0]) {
		return "", nil
	}
	v := resp.Data[0][idx]
	if v == nil {
		return "", nil
	}
	raw, ok := v.(string)
	if !ok {
		return "", nil
	}
	return parseSnowflakeTimestamp(strings.Trim(raw, `"`)), nil
}

// GetFeedbackFromTable returns all feedback rows for the given agent from the remote table.
func (c *Client) GetFeedbackFromTable(ctx context.Context, db, schema, table, agentName string) ([]FeedbackTableRow, error) {
	fq := fmt.Sprintf("%s.%s.%s",
		identifierSegment(db), identifierSegment(schema), identifierSegment(table))
	agentEsc := escapeSQLString(unquoteIdentifier(agentName))
	colSet, err := c.feedbackTableColumnSet(ctx, db, schema, table)
	if err != nil {
		return nil, err
	}
	sentimentSourceExpr := `'' AS sentiment_source`
	if _, ok := colSet["sentiment_source"]; ok {
		sentimentSourceExpr = "sentiment_source"
	}
	sentimentReasonExpr := `'' AS sentiment_reason`
	if _, ok := colSet["sentiment_reason"]; ok {
		sentimentReasonExpr = "sentiment_reason"
	}
	stmt := fmt.Sprintf(
		`SELECT record_id, event_ts, agent_name, user_name, sentiment, %s, %s, feedback_message, categories, question, response, response_time_ms, tool_uses, request_value, checked, checked_at
FROM %s WHERE agent_name = '%s' ORDER BY event_ts DESC NULLS LAST`,
		sentimentSourceExpr, sentimentReasonExpr, fq, agentEsc)
	resp, err := c.executeStatement(ctx, db, schema, stmt)
	if err != nil {
		return nil, err
	}
	colIndex := make(map[string]int)
	for i, col := range resp.ResultSetMetaData.RowType {
		colIndex[strings.ToLower(col.Name)] = i
	}
	var rows []FeedbackTableRow
	for _, row := range resp.Data {
		r := FeedbackTableRow{}
		getStr := func(key string) string {
			if idx, ok := colIndex[key]; ok && idx < len(row) {
				if s, ok := row[idx].(string); ok {
					return strings.Trim(s, `"`)
				}
			}
			return ""
		}
		getBool := func(key string) bool {
			if idx, ok := colIndex[key]; ok && idx < len(row) {
				if b, ok := row[idx].(bool); ok {
					return b
				}
				if s, ok := row[idx].(string); ok {
					return strings.EqualFold(s, "true") || s == "1"
				}
			}
			return false
		}
		r.RecordID = getStr("record_id")
		r.Timestamp = parseSnowflakeTimestamp(getStr("event_ts"))
		r.AgentName = getStr("agent_name")
		r.UserName = getStr("user_name")
		r.Sentiment = getStr("sentiment")
		r.SentimentSource = getStr("sentiment_source")
		r.SentimentReason = getStr("sentiment_reason")
		r.FeedbackMessage = getStr("feedback_message")
		r.Question = getStr("question")
		r.Response = getStr("response")
		r.Checked = getBool("checked")
		r.CheckedAt = parseSnowflakeTimestamp(getStr("checked_at"))
		if idx, ok := colIndex["categories"]; ok && idx < len(row) {
			if s, ok := row[idx].(string); ok && s != "" {
				json.Unmarshal([]byte(s), &r.Categories)
			}
		}
		if idx, ok := colIndex["response_time_ms"]; ok && idx < len(row) {
			switch v := row[idx].(type) {
			case float64:
				r.ResponseTimeMs = int64(v)
			case string:
				fmt.Sscanf(v, "%d", &r.ResponseTimeMs)
			}
		}
		if idx, ok := colIndex["tool_uses"]; ok && idx < len(row) {
			if s, ok := row[idx].(string); ok && s != "" {
				var rawUses []map[string]any
				if err := json.Unmarshal([]byte(s), &rawUses); err == nil {
					for _, m := range rawUses {
						tu := ToolUseInfo{
							ToolType:   probeString(m, []string{"tool_type", "TOOL_TYPE", "type", "TYPE"}),
							ToolName:   probeString(m, []string{"tool_name", "TOOL_NAME", "name", "NAME"}),
							Query:      probeString(m, []string{"query", "QUERY"}),
							ToolStatus: probeString(m, []string{"tool_status", "TOOL_STATUS", "status", "STATUS"}),
							SQL:        probeString(m, []string{"sql", "SQL"}),
						}
						// Fallback for nested input.query shape.
						if tu.Query == "" {
							for _, key := range []string{"input", "INPUT"} {
								if iv, ok := m[key].(map[string]any); ok {
									tu.Query = probeString(iv, []string{"query", "QUERY"})
									if tu.Query != "" {
										break
									}
								}
							}
						}
						// Fallback for nested tool_use object shape.
						if tu.ToolType == "" || tu.ToolName == "" || tu.Query == "" || tu.ToolStatus == "" || tu.SQL == "" {
							for _, key := range []string{"tool_use", "TOOL_USE"} {
								if uv, ok := m[key].(map[string]any); ok {
									if tu.ToolType == "" {
										tu.ToolType = probeString(uv, []string{"tool_type", "TOOL_TYPE", "type", "TYPE"})
									}
									if tu.ToolName == "" {
										tu.ToolName = probeString(uv, []string{"tool_name", "TOOL_NAME", "name", "NAME"})
									}
									if tu.Query == "" {
										tu.Query = probeString(uv, []string{"query", "QUERY"})
										if tu.Query == "" {
											for _, ik := range []string{"input", "INPUT"} {
												if iv, ok := uv[ik].(map[string]any); ok {
													tu.Query = probeString(iv, []string{"query", "QUERY"})
													if tu.Query != "" {
														break
													}
												}
											}
										}
									}
									if tu.ToolStatus == "" {
										tu.ToolStatus = probeString(uv, []string{"tool_status", "TOOL_STATUS", "status", "STATUS"})
									}
									if tu.SQL == "" {
										tu.SQL = probeString(uv, []string{"sql", "SQL"})
									}
									break
								}
							}
						}
						r.ToolUses = append(r.ToolUses, tu)
					}
				}
			}
		}
		if idx, ok := colIndex["request_value"]; ok && idx < len(row) {
			if s, ok := row[idx].(string); ok {
				r.RequestRaw = s
			}
		}
		rows = append(rows, r)
	}
	return rows, nil
}

// UpdateFeedbackChecked sets the checked flag and checked_at for a record in the remote table.
func (c *Client) UpdateFeedbackChecked(ctx context.Context, db, schema, table, recordID string, checked bool) error {
	fq := fmt.Sprintf("%s.%s.%s",
		identifierSegment(db), identifierSegment(schema), identifierSegment(table))
	ridEsc := escapeSQLString(recordID)
	checkedVal := "FALSE"
	if checked {
		checkedVal = "TRUE"
	}
	stmt := fmt.Sprintf(
		`UPDATE %s SET checked = %s, checked_at = CURRENT_TIMESTAMP(), updated_at = CURRENT_TIMESTAMP() WHERE record_id = '%s'`,
		fq, checkedVal, ridEsc)
	_, err := c.executeStatement(ctx, db, schema, stmt)
	return err
}

// ClearFeedbackForAgent deletes all feedback rows for the given agent from the remote table.
func (c *Client) ClearFeedbackForAgent(ctx context.Context, db, schema, table, agentName string) error {
	fq := fmt.Sprintf("%s.%s.%s",
		identifierSegment(db), identifierSegment(schema), identifierSegment(table))
	agentEsc := escapeSQLString(agentName)
	stmt := fmt.Sprintf(`DELETE FROM %s WHERE agent_name = '%s'`, fq, agentEsc)
	_, err := c.executeStatement(ctx, db, schema, stmt)
	return err
}
