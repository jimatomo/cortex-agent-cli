package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"coragent/internal/auth"
)

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

	// Set authorization header
	token, tokenType, err := auth.BearerToken(ctx, c.authCfg)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Snowflake-Authorization-Token-Type", tokenType)
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

// identifierSegment returns a properly-quoted SQL identifier segment.
// If the value is already double-quoted, inner quotes are escaped.
// If the value contains special characters, it is wrapped in double-quotes.
func identifierSegment(value string) string {
	trimmed := strings.TrimSpace(value)
	// Preserve existing double-quotes (user/system explicitly quoted it)
	if len(trimmed) >= 2 && strings.HasPrefix(trimmed, `"`) && strings.HasSuffix(trimmed, `"`) {
		inner := trimmed[1 : len(trimmed)-1]
		return `"` + strings.ReplaceAll(inner, `"`, `""`) + `"`
	}
	if !isSimpleIdentifier(trimmed) {
		trimmed = `"` + strings.ReplaceAll(trimmed, `"`, `""`) + `"`
	}
	return trimmed
}

// unquoteIdentifier strips surrounding double-quotes from a value.
// Used for sqlStatementRequest payload fields which need raw (unquoted) values.
func unquoteIdentifier(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) >= 2 && strings.HasPrefix(trimmed, `"`) && strings.HasSuffix(trimmed, `"`) {
		return trimmed[1 : len(trimmed)-1]
	}
	return trimmed
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

func isIdentifierStart(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_'
}

func isIdentifierPart(r rune) bool {
	return isIdentifierStart(r) || (r >= '0' && r <= '9') || r == '$'
}
