# coragent v1.0 Roadmap

This document outlines the tasks required to reach a stable v1.0 release.
The primary goals are:

1. **Stable interfaces** — Public-facing CLI flags, YAML schema, and Go package APIs must not break between patch releases.
2. **Regression coverage** — Every command and key code path must have automated tests.
3. **Human-readable code** — The codebase must be easy to audit, debug, and contribute to.

---

## Phase 1: Refactoring

### 1-1. Split `internal/api/client.go`

`client.go` is ~45 KB and handles agent CRUD, grants, SQL execution, and HTTP plumbing in one file.
Split it into focused files so each area can be reviewed and changed independently.

- [x] `internal/api/agent.go` — CreateAgent, UpdateAgent, DeleteAgent, GetAgent, ListAgents, DescribeAgent
- [x] `internal/api/grant.go` — ShowGrants, ExecGrant, ExecRevoke
- [x] `internal/api/query.go` — ExecSQL, query helpers used by eval and feedback
- [x] `internal/api/http.go` — doRequest, retry logic, debug logging, header building
- [x] Keep `client.go` as the `Client` struct definition and constructor only

### 1-2. Define explicit interfaces for each subsystem

Unstable concrete types scattered across packages make it hard to test and hard to evolve.
Introduce interface types at package boundaries:

- [x] `internal/api.AgentService` — methods for agent lifecycle operations
- [x] `internal/api.RunService` — streaming execution
- [x] `internal/api.ThreadService` — thread CRUD
- [x] `internal/auth.Authenticator` — `BearerToken(ctx) (string, string, error)`
- [x] Wire interfaces into CLI commands via dependency injection so commands do not construct API clients themselves

### 1-3. Eliminate duplication in `internal/cli/`

Each command file re-implements the same patterns: resolve targets, build a client, call the API, print results.

- [x] Extract `func buildClient(cmd *cobra.Command) (*api.Client, error)` into a shared helper in `root.go` or a new `internal/cli/context.go`
- [x] Extract `func resolveAndBuild(cmd *cobra.Command) (Target, *api.Client, error)` that combines resolve + build
- [x] Standardize confirmation prompt into a reusable `confirm(prompt string, yes bool) bool`

### 1-4. Remove or implement `internal/plan/`

The directory exists but is empty. Either:

- [x] Or delete the directory to avoid confusion

### 1-5. Standardize error handling

- [ ] Replace bare `fmt.Errorf` at call sites with structured sentinel errors or typed error types where callers need to distinguish them
- [x] Ensure all user-facing errors print a clear action hint (`--debug for details` in root; improved database/schema hint)
- [x] Audit for swallowed errors: fixed `enc.Close()` in export.go and loader.go; TOML decode errors now warn to stderr

### 1-6. Freeze the YAML agent spec schema

The `AgentSpec` struct is the primary user-facing contract. Before v1.0:

- [x] Document every field in `internal/agent/agent.go` with a Go doc comment and its Snowflake API counterpart
- [x] Write a JSON Schema or at least a Go validation function that returns descriptive errors for invalid specs
- [x] Mark any fields intended to be experimental / unstable with a comment

---

## Phase 2: Regression Tests

### 2-1. Unit tests — `internal/api/`

Current coverage exists but is incomplete.

- [x] Table-driven tests for `DescribeAgent` response mapping (cover all columns)
- [x] Tests for grant diff computation round-trips (apply + show → no diff)
- [x] Tests for SQL result parsing used by eval and feedback
- [x] Mock-based tests for `RunAgent` streaming event parsing (all event types)

### 2-2. Unit tests — `internal/cli/`

CLI commands currently lack fine-grained unit tests independent of a real Snowflake account.

- [x] Introduce a `fakeAgentService` (implementing the interface from 1-2) to use in command tests
- [x] Test `plan` command output for create / update / unchanged / delete cases
- [x] Test `apply` command: `executeApply` (create/update/no-change/grants), `applyGrantDiff`, `confirm` with io.Reader injection
- [x] Test `export` command: field mapping, unmapped-field warnings
- [x] Test `eval` command: pass/fail scoring, tool-invocation matching
- [x] Test `feedback` command: cache write, incremental update, `--clear` flag

### 2-3. Unit tests — `internal/diff/`

- [x] Verify diff output for all change types (Added, Removed, Modified) with nested paths
- [x] Edge cases: empty spec vs full spec, no-op diff returns empty diff

### 2-4. Unit tests — `internal/auth/`

- [x] Test JWT signing with a test RSA key
- [x] Test `snowflake_config.go` parsing for all authenticator types
- [x] Test OAuth token refresh logic with a mock HTTP server

### 2-5. Regression test suite

Create `internal/regression/` with end-to-end scenarios that run against mock HTTP servers (no real Snowflake required).

- [x] Scenario: full lifecycle — validate → plan → apply → export → delete
- [x] Scenario: multi-environment variable substitution
- [x] Scenario: grant add/remove across apply cycles
- [x] Scenario: eval with passing and failing test cases
- [x] Scenario: thread continuity across `run` invocations

### 2-6. CI integration

- [x] Add `go test ./...` with `-race` flag to CI pipeline
- [x] Separate unit tests (`-short`) from integration tests (require credentials)
- [x] Set minimum coverage threshold for non-integration packages (baseline: 45%, target: 80%)

---

## Phase 3: Readability & Maintainability

### 3-1. Reduce file size in `internal/cli/`

Large files are hard to read and review:

- [x] `run.go` — separate streaming I/O handling into `internal/cli/run_io.go`
- [x] `eval.go` — separate judge logic into `internal/cli/eval_judge.go`
- [x] `feedback.go` + `feedback_cache.go` — move cache to `internal/feedbackcache/` package for testability

### 3-2. Structured logging

- [x] Replace ad-hoc `fmt.Fprintf(os.Stderr, "[debug] ...")` with a `slog`-based logger
- [x] Pass the logger via context or dependency injection rather than global state
- [x] `--debug` sets log level to DEBUG; default is WARN (errors only)

### 3-3. Add CHANGELOG

- [x] Create `CHANGELOG.md` using [Keep a Changelog](https://keepachangelog.com/) format
- [x] Back-fill notable changes since the initial commit
- [x] Add CHANGELOG update to the release checklist

### 3-4. Improve public-facing documentation

- [x] `README.md`: add a quick-start section showing the minimal config to run `coragent apply`
- [x] Each command: ensure `--help` output fully describes all flags and shows at least one example
- [x] Document `.coragent.toml` schema with all supported keys

### 3-5. Linter and formatting enforcement

- [x] Add `.golangci.yml` with a baseline ruleset (`errcheck`, `govet`, `staticcheck`, `godot` for comments)
- [x] Add linter step to CI
- [x] Fix existing lint warnings before v1.0

---

## Milestone Summary

| Milestone | Contents |
|-----------|----------|
| Phase 1 complete | Refactoring done, interfaces defined |
| Phase 2 complete | Unit tests + regression suite green |
| Phase 3 complete | Readability improvements, linter clean, CHANGELOG |
| **v1.0.0** | Stable release — no planned breaking changes to CLI flags or YAML schema |

---

## Stability Guarantees for v1.0+

Once v1.0 is released, the following are considered **stable** and will not change without a major version bump:

- All CLI command names and flags documented in `--help`
- The `AgentSpec` YAML schema (field names and types)
- The `DeployConfig`, `EvalConfig`, and `GrantConfig` YAML schema
- Exit codes: 0 = success, 1 = user/config error, 2 = unexpected error

The following are **not** guaranteed stable in v1.x:

- Internal Go package APIs under `internal/`
- Output format of `--debug` logs
- Local cache file formats (`~/.coragent/`)
