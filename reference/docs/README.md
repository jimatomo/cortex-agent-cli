# Internal Implementation Docs

Internal documentation for the coragent codebase. For user-facing docs, see [README.md](../../README.md) and the [reference](../) directory.

## Navigation

### Architecture
- [Overview](architecture/overview.md) — End-to-end CLI execution and package relationships
- [CLI Command Map](architecture/cli-command-map.md) — Command tree and internal entrypoints

### Commands
- [Command Reference](commands/command-reference.md) — Canonical inventory of all commands/subcommands
- [Command Coverage Checklist](commands/command-coverage-checklist.md) — Ensure no command docs are missed when the tree changes

### Flows
- [Plan/Apply Flow](flows/plan-apply-flow.md) — Load → resolve → diff → grant diff → apply lifecycle
- [Auth Flow](flows/auth-flow.md) — Auth resolution, OAuth/login/logout, credential loading
- [Query, Feedback, Threads Flow](flows/query-feedback-threads-flow.md) — Query execution and feedback/thread flows

### Components
- [Agent](components/agent.md) — YAML parsing, vars/env substitution, validation
- [API](components/api.md) — Client abstraction, service interfaces, error handling
- [CLI](components/cli.md) — Cobra root/options, command wiring, error classification
- [Auth](components/auth.md) — Config chain, authenticators, token/key handling
- [Diff and Grant](components/diff-and-grant.md) — Spec diff model and GRANT/REVOKE convergence
- [Config, FeedbackCache, Thread](components/config-feedbackcache-thread.md) — Project config, feedback cache, thread state

### Testing
- [Testing Strategy](testing/testing-strategy.md) — Unit/integration/regression layout and mock strategy

### Maintenance
- [Update Checklist](maintenance/update-checklist.md) — "When X changes, update Y docs" mapping

## Maintenance Policy

These docs must be kept in sync with the codebase. See [AGENTS.md](../../AGENTS.md) for the mandatory maintenance rules.
