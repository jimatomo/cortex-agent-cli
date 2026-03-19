package api

import (
	"context"
	"strings"
)

type queryTagContextKey struct{}

// WithQueryTagCommand attaches the originating CLI command to the request context.
func WithQueryTagCommand(ctx context.Context, command string) context.Context {
	return context.WithValue(ctx, queryTagContextKey{}, strings.TrimSpace(command))
}

func (c *Client) resolveQueryTag(ctx context.Context) string {
	base := strings.TrimSpace(c.queryTagBase)
	if base == "" {
		base = "coragent"
	}
	command, _ := ctx.Value(queryTagContextKey{}).(string)
	command = strings.TrimSpace(command)
	if command == "" {
		return base
	}
	return base + ":" + command
}

func (c *Client) sqlRequestWithQueryTag(ctx context.Context, payload sqlStatementRequest) sqlStatementRequest {
	tag := c.resolveQueryTag(ctx)
	if tag == "" {
		return payload
	}
	if payload.Parameters == nil {
		payload.Parameters = map[string]string{}
	}
	payload.Parameters["query_tag"] = tag
	return payload
}
