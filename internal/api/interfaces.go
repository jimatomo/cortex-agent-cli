package api

import (
	"context"

	"coragent/internal/agent"
)

// AgentService defines the contract for agent lifecycle operations.
type AgentService interface {
	CreateAgent(ctx context.Context, db, schema string, spec agent.AgentSpec) error
	UpdateAgent(ctx context.Context, db, schema, name string, payload any) error
	DeleteAgent(ctx context.Context, db, schema, name string) error
	GetAgent(ctx context.Context, db, schema, name string) (agent.AgentSpec, bool, error)
	DescribeAgent(ctx context.Context, db, schema, name string) (DescribeResult, error)
	ListAgents(ctx context.Context, db, schema string) ([]AgentListItem, error)
}

// RunService defines the contract for agent execution.
type RunService interface {
	RunAgent(ctx context.Context, db, schema, name string, req RunAgentRequest, opts RunAgentOptions) (*ResponseEvent, error)
}

// ThreadService defines the contract for thread management.
type ThreadService interface {
	CreateThread(ctx context.Context) (string, error)
	ListThreads(ctx context.Context) ([]Thread, error)
	GetThread(ctx context.Context, threadID string) (*Thread, error)
	DeleteThread(ctx context.Context, threadID string) error
}

// GrantService defines the contract for privilege management.
type GrantService interface {
	ShowGrants(ctx context.Context, db, schema, agentName string) ([]ShowGrantsRow, error)
	ExecuteGrant(ctx context.Context, db, schema, agentName, roleType, roleName, privilege string) error
	ExecuteRevoke(ctx context.Context, db, schema, agentName, roleType, roleName, privilege string) error
}

// QueryService defines the contract for SQL-based query operations.
type QueryService interface {
	GetFeedback(ctx context.Context, db, schema, agentName string) ([]FeedbackRecord, error)
	CortexComplete(ctx context.Context, sqlStmt string) (string, error)
}

// Compile-time assertions: *Client must implement all service interfaces.
var (
	_ AgentService  = (*Client)(nil)
	_ RunService    = (*Client)(nil)
	_ ThreadService = (*Client)(nil)
	_ GrantService  = (*Client)(nil)
	_ QueryService  = (*Client)(nil)
)
