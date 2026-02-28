# Update Checklist

When making code changes, update the corresponding docs in the same PR/commit.

## When X Changes, Update Y

| Code Change | Docs to Update |
|-------------|----------------|
| New command or subcommand | [command-reference.md](../commands/command-reference.md), [cli-command-map.md](../architecture/cli-command-map.md), [command-coverage-checklist.md](../commands/command-coverage-checklist.md) |
| Command flags, behavior, or dependencies | [command-reference.md](../commands/command-reference.md) |
| Plan/apply flow (load, resolve, diff, apply) | [plan-apply-flow.md](../flows/plan-apply-flow.md), [components/diff-and-grant.md](../components/diff-and-grant.md) |
| Auth config, OAuth, login/logout | [auth-flow.md](../flows/auth-flow.md), [components/auth.md](../components/auth.md) |
| Run, feedback, threads flow | [query-feedback-threads-flow.md](../flows/query-feedback-threads-flow.md), [components/config-feedbackcache-thread.md](../components/config-feedbackcache-thread.md) |
| API client, interfaces, error handling | [components/api.md](../components/api.md) |
| Agent loading, vars, validation | [components/agent.md](../components/agent.md) |
| CLI root, options, resolve, errors | [components/cli.md](../components/cli.md) |
| Config, feedback cache, thread state | [components/config-feedbackcache-thread.md](../components/config-feedbackcache-thread.md) |
| New package or major refactor | [overview.md](../architecture/overview.md), relevant component/flow docs |
| Test layout or mock strategy | [testing-strategy.md](../testing/testing-strategy.md) |

## Verification

After doc updates:

1. Check internal links: `rg '\]\(' reference/docs/` for broken refs
2. Ensure no duplicate content with `reference/` (yaml-spec, config-priority) unless intentional
3. Run `command-coverage-checklist` steps when command tree changes
