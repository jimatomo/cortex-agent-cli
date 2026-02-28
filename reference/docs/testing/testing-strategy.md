# Testing Strategy

## Test Layout

- **Unit tests** — Same package; `*_test.go` alongside source
- **Integration tests** — `internal/integration_test.go`, `internal/integration_grant_test.go`, `internal/api/client_integration_test.go`, `internal/auth/auth_integration_test.go`; require Snowflake credentials
- **Regression tests** — `internal/regression/`; use mocks for full plan/apply/eval lifecycle

## Build Tags

- **Default:** `go test ./...` runs unit tests only (integration skipped)
- **Integration:** `go test -tags=integration ./...` runs integration tests (requires `SNOWFLAKE_*` env)

## Mock Strategy

- **File:** `internal/regression/mock.go`
- **Implements:** All service interfaces (`AgentService`, `RunService`, `ThreadService`, `GrantService`, `QueryService`)
- **Usage:** Regression tests inject mock implementations to avoid real API calls
- **Client:** `api.NewClientForTest(baseURL, cfg)` for tests against mock HTTP servers

## Test Commands

```bash
# Unit tests (race + short)
go test -race -short ./...

# Integration tests
go test -race -tags=integration ./...

# Code quality
go vet ./...
golangci-lint run
```

## Coverage

- **Target:** 40% minimum (enforced in CI)
- **File I/O:** Use `t.TempDir()` for temporary directories
- **Framework:** Standard `testing.T`; no external test libraries

## Key Test Files

| Area | Files |
|------|-------|
| CLI (plan/apply) | `cli/plan_test.go`, `cli/apply_test.go`, `cli/plan_core_test.go`, `cli/apply_core_test.go` |
| CLI (other) | `cli/validate_test.go`, `cli/export_test.go`, `cli/eval_test.go`, `cli/run_test.go`, `cli/resolve_test.go`, `cli/feedback_test.go`, `cli/errors_test.go` |
| Regression | `regression/lifecycle_test.go`, `regression/grants_test.go`, `regression/eval_test.go`, `regression/threads_test.go`, `regression/vars_test.go` |
| API | `api/agent_test.go`, `api/run_test.go`, `api/client_test.go`, `api/query_test.go`, `api/threads_test.go` |
| Auth | `auth/auth_test.go`, `auth/oauth_test.go`, `auth/oauth_store_test.go`, `auth/snowflake_config_test.go` |
| Agent | `agent/loader_test.go`, `agent/validate_test.go`, `agent/vars_test.go` |
| Config | `config/config_test.go` |
| FeedbackCache | `feedbackcache/cache_test.go` |
| Thread | `thread/state_test.go` |
| Diff/Grant | `diff/diff_test.go`, `grant/grant_test.go` |
