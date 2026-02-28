# CLI Component

The CLI package wires Cobra commands, persistent flags, and shared helpers.

## Key Files

- `internal/cli/root.go` — `NewRootCmd`, `Execute`, `RootOptions`
- `internal/cli/context.go` — `buildClient`, `buildClientAndCfg`, `confirm`, `convertGrantRows`
- `internal/cli/plan.go` — `applyAuthOverrides` (overlays CLI flags onto auth config)
- `internal/cli/resolve.go` — `ResolveTarget`, `ResolveTargetForExport`
- `internal/cli/errors.go` — `UserErr`, `IsUserError`

## RootOptions

Persistent flags applied to all commands:

| Flag | Var | Description |
|------|-----|-------------|
| `-a`/`--account` | Account | Snowflake account |
| `-d`/`--database` | Database | Target database |
| `-s`/`--schema` | Schema | Target schema |
| `-r`/`--role` | Role | Snowflake role |
| `-c`/`--connection` | Connection | config.toml connection name |
| `-e`/`--env` | Env | vars environment name |
| `--quote-identifiers` | QuoteIdentifiers | Double-quote DB/schema |
| `--debug` | Debug | Enable debug logging |

## Execute Flow

1. `NewRootCmd()` builds root command with all subcommands (via `cmd.AddCommand`)
2. `PersistentPreRun` sets the package-level `DebugEnabled` flag from `opts.Debug`
3. `root.Execute()` runs the selected command
4. On error:
   - If `DebugEnabled`: print full stack trace via `debug.Stack()`
   - Print `Error: <message>`
   - If `IsUserError(err)`: exit 1 (no --debug hint)
   - Else: print "run with --debug for detailed trace output"; exit 2

## Error Classification

- **User errors** — Config, validation, user cancellation; exit 1
- **System errors** — API failures, unexpected; exit 2

`UserErr(err)` wraps an error as user error. `IsUserError` checks for that wrapper.

## Client Construction

- `buildClient(opts)` — Returns `*api.Client`; used when config not needed
- `buildClientAndCfg(opts)` — Returns `(*api.Client, auth.Config)`; used when `ResolveTarget` needs config (plan, apply, delete, export, run, eval, feedback)

## Target Resolution

- **ResolveTarget(spec, opts, cfg)** — For plan/apply/delete: database/schema from deploy section, opts, or cfg
- **ResolveTargetForExport(opts, cfg)** — For export/run: database/schema from opts or cfg (no spec)

## Related Docs

- [architecture/cli-command-map.md](../architecture/cli-command-map.md) — Command tree
- [commands/command-reference.md](../commands/command-reference.md) — Per-command details
