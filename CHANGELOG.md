# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
`coragent` uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased] — v1.0.0

### Added
- `internal/api` interfaces (`AgentService`, `RunService`, `ThreadService`, `GrantService`, `QueryService`) for dependency injection and testing.
- `auth.Authenticator` interface with `ConfigAuthenticator` implementation.
- `AgentSpec.Validate()` with descriptive errors for invalid YAML specs.
- Shared CLI helpers in `internal/cli/context.go`: `buildClient`, `buildClientAndCfg`, `confirm`, `convertGrantRows`.
- Unit tests: `plan` command via interface-driven `buildPlanItems`; feedback cache round-trip; `DescribeAgent` SQL response mapping; OAuth token refresh with mock HTTP server.
- `.golangci.yml` baseline linter configuration (`errcheck`, `govet`, `staticcheck`, `godot`).
- CI: golangci-lint job; `-race` flag on unit tests; per-package coverage report.

### Changed
- `internal/api/client.go` split into focused files: `agent.go`, `grant.go`, `query.go`, `http.go`, keeping `client.go` as struct/constructor only.
- `internal/auth/oauth.go`: extracted `refreshAccessTokenInternal` and `exchangeCodeForTokensInternal` helpers for testability.

### Removed
- Empty `internal/plan/` directory.

---

## [0.3.x] — Feedback & Eval enhancements

### Added
- `feedback` command: fetch `CORTEX_AGENT_FEEDBACK` observability events, LEFT JOIN with REQUEST events to show user question, agent response, tool invocations, and SQL; incremental cache at `~/.coragent/feedback/<agent>.json`; `--clear` flag to reset cache.
- LLM-as-a-Judge response scoring via `SNOWFLAKE.CORTEX.COMPLETE`; `response_score_threshold` at spec and test-case level.
- `ignore_tools` in eval config and `.coragent.toml` to exclude utility tools (e.g., `data_to_chart`) from eval scoring.
- `--no-tools` flag on `feedback` to hide tool invocation details.
- Variable substitution: `vars` blocks with `--env` flag for environment-specific configuration; `${env.VAR}` for OS environment variables.

### Fixed
- Expand `ALL` privilege to individual privileges in grant diff comparison.

---

## [0.2.x] — Run, Grants & Eval

### Added
- `run` command: execute a Cortex Agent with SSE streaming output; multi-turn conversation support via `--thread` flag and `internal/thread` state store.
- `eval` command: run YAML-defined test cases against a deployed agent; tool-match verification; JSON and Markdown output; timestamp suffix option.
- Grant management: `GRANT`/`REVOKE` diff applied during `apply`; `SHOW GRANTS` round-trip; DATABASE_ROLE support.
- `export` command: export a live agent back to YAML with unmapped-field warnings.
- Profile fields `avatar` and `color` in agent spec.
- `--recursive` / `-R` flag on `plan`, `apply`, `delete` for multi-agent directories.

### Fixed
- Correct `Before`/`After` order in diff output.
- Improved `semantic_view` and `tool_resources` handling in describe response.

---

## [0.1.x] — Initial Release

### Added
- `plan` command: compare local YAML spec against live Snowflake agent, show diff.
- `apply` command: create or update agents from YAML with confirmation prompt; `-y` flag for automation.
- `delete` command: delete agents defined in YAML files.
- `validate` command: validate YAML spec files against the schema.
- Key Pair (RSA JWT) authentication via `SNOWFLAKE_PRIVATE_KEY` env var or `~/.snowflake/config.toml`.
- OAuth authentication (experimental) with PKCE; `login`/`logout` commands.
- `--version` flag.
- Installation script (`install.sh`).
- GitHub Actions CI with integration tests.
