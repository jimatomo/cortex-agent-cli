# AGENTS.md

This file provides guidance for AI agents (Claude Code, Codex, etc.) working on this codebase.
For user-facing documentation, see [README.md](./README.md).

## Project Overview

**coragent** is a CLI tool for managing Snowflake Cortex Agent deployments via REST API, following a plan/apply workflow similar to Terraform.

- **Module:** `coragent` (Go 1.24.0)
- **Philosophy:** Minimal dependencies, no external logging framework — uses `fmt` and `fatih/color`. Maintainability is prioritized; breaking changes are a valid option when they improve the codebase. Security is non-negotiable — always implement safely and avoid introducing vulnerabilities.

## Development Commands

```bash
# Build
go build -o coragent ./cmd/coragent

# Unit tests (race detector + short mode)
go test -race -short ./...

# Integration tests (requires Snowflake credentials)
go test -race -tags=integration ./...

# Code quality
go vet ./...
go mod tidy
golangci-lint run
```

## Testing Patterns

- **Framework:** Standard `testing.T` — no external test libraries
- **File I/O:** `t.TempDir()` for temporary directories
- **Mocking:** `internal/regression/mock.go` implements all service interfaces
- **Integration tests:** Build tag `integration`, require Snowflake credentials
- **Coverage target:** 40% minimum (enforced in CI)

## Internal Implementation Docs

Internal implementation documentation lives under [reference/docs](./reference/docs). These docs must be maintained in sync with the codebase.

- **Location:** [reference/docs](./reference/docs) — architecture, flows, components, commands, testing, maintenance
- **Maintenance rule:** Any functional change in `cmd/` or `internal/` requires reviewing and updating affected markdown files in `reference/docs` in the same PR/commit.
- **New packages/flows:** Add a corresponding docs entry and update the index in `reference/docs/README.md`.
- **Command tree changes:** Any change to `new*Cmd`, `Use:` strings, or `AddCommand` wiring requires updating [reference/docs/commands/](./reference/docs/commands/).
