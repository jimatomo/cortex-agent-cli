# API Component

The API package provides the Snowflake Cortex Agent REST client and service interfaces.

## Key Files

- `internal/api/client.go` — `Client`, `NewClient`, `NewClientWithDebug`, `NewClientForTest`
- `internal/api/interfaces.go` — `AgentService`, `RunService`, `ThreadService`, `GrantService`, `QueryService`
- `internal/api/agent.go` — Agent CRUD implementation
- `internal/api/run.go` — RunAgent (streaming)
- `internal/api/threads.go` — Thread CRUD
- `internal/api/grant.go` — ShowGrants, ExecuteGrant, ExecuteRevoke
- `internal/api/query.go` — GetFeedback, CortexComplete, feedback-table helpers (`FeedbackTableExists`, `CreateFeedbackTable`, `SyncFeedbackFromEventsToTable`, `GetFeedbackFromTable`, `UpdateFeedbackChecked`, `ClearFeedbackForAgent`)
- `internal/api/http.go` — HTTP helpers, auth header injection

## Client Construction

- **Production:** `api.NewClientWithDebug(cfg, debug)` — Uses `https://<account>.snowflakecomputing.com`; `CORAGENT_API_BASE_URL` env overrides base URL for testing
- **Test:** `api.NewClientForTest(baseURL, cfg)` — No real Snowflake; for mock HTTP servers

## Service Interfaces

| Interface | Methods | Used By |
|-----------|---------|---------|
| `AgentService` | CreateAgent, UpdateAgent, DeleteAgent, GetAgent, DescribeAgent, ListAgents | plan, apply, delete, export, run |
| `RunService` | RunAgent | run, eval |
| `ThreadService` | CreateThread, ListThreads, GetThread, DeleteThread | run, threads |
| `GrantService` | ShowGrants, ExecuteGrant, ExecuteRevoke | plan, apply |
| `QueryService` | GetFeedback, CortexComplete | feedback |

`*api.Client` implements all five interfaces (compile-time assertions in `interfaces.go`). Feedback-table helpers (`FeedbackTableExists`, `CreateFeedbackTable`, `SyncFeedbackFromEventsToTable`, `GetFeedbackFromTable`, `UpdateFeedbackChecked`, `ClearFeedbackForAgent`) are methods on `*Client` directly and are **not** part of any service interface — they are used only by the `feedback` command.

## Error Handling

- **APIError** — Non-2xx HTTP response; `StatusCode`, `Body`
- **IsNotFoundError(err)** — True for 404 or Snowflake "does not exist" / "not found" / 002003
- Plan/apply use `(spec, exists, error)` from `GetAgent` rather than inspecting errors directly

## Auth Integration

Each request adds `Authorization: Bearer <token>` via `auth.AuthHeader(ctx, cfg)`. The client holds `auth.Config` and obtains tokens on demand (JWT or OAuth refresh).

## Related Docs

- [components/auth.md](auth.md) — Config and token acquisition
- [flows/plan-apply-flow.md](../flows/plan-apply-flow.md) — AgentService, GrantService usage
