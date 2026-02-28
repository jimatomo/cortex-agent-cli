# Command Coverage Checklist

Use this checklist when modifying the command tree to ensure docs stay in sync.

## When Adding a New Command

- [ ] Add the command to `internal/cli/root.go` (or `auth.go` for auth subcommands)
- [ ] Update [command-reference.md](command-reference.md) with:
  - Use string
  - Entry function
  - Dependencies
  - Side effects
  - Flags
- [ ] Update [cli-command-map.md](../architecture/cli-command-map.md) command tree and entrypoints table
- [ ] Add a flow or component doc if the command introduces new behavior (e.g., new API usage)
- [ ] Update [update-checklist.md](../maintenance/update-checklist.md) if the new command affects existing docs

## When Changing Command Behavior

- [ ] Update [command-reference.md](command-reference.md) if flags, dependencies, or side effects change
- [ ] Update flow docs if the execution path changes (e.g., plan/apply, auth, run/feedback/threads)
- [ ] Update component docs if new packages or interfaces are used

## When Renaming or Removing a Command

- [ ] Remove or update the entry in [command-reference.md](command-reference.md)
- [ ] Update [cli-command-map.md](../architecture/cli-command-map.md)
- [ ] Remove or update any flow/component references to the old command

## Verification

After changes, run a quick ripgrep check to ensure no stale references:

```bash
rg "old-command-name" reference/docs/
```
