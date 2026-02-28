# CLI Command Map

## Command Tree

```
coragent
├── plan [path]
├── apply [path]
├── delete [path]
├── validate [path]
├── export <agent-name>
├── new
├── run [agent-name]
├── threads
├── eval [path]
├── feedback [agent-name]
├── login
├── logout
└── auth
    ├── status
    └── init
```

## Entrypoints by Command

| Command | Entry Function | Source File |
|---------|----------------|-------------|
| `plan` | `newPlanCmd` | `internal/cli/plan.go` |
| `apply` | `newApplyCmd` | `internal/cli/apply.go` |
| `delete` | `newDeleteCmd` | `internal/cli/delete.go` |
| `validate` | `newValidateCmd` | `internal/cli/validate.go` |
| `export` | `newExportCmd` | `internal/cli/export.go` |
| `new` | `newNewCmd` | `internal/cli/new.go` |
| `run` | `newRunCmd` | `internal/cli/run.go` |
| `threads` | `newThreadsCmd` | `internal/cli/threads.go` |
| `eval` | `newEvalCmd` | `internal/cli/eval.go` |
| `feedback` | `newFeedbackCmd` | `internal/cli/feedback.go` |
| `login` | `newLoginCmd` | `internal/cli/login.go` |
| `logout` | `newLogoutCmd` | `internal/cli/logout.go` |
| `auth` | `newAuthCmd` | `internal/cli/auth.go` |
| `auth status` | `newAuthStatusCmd` | `internal/cli/auth.go` |
| `auth init` | `newAuthInitCmd` | `internal/cli/auth_init.go` |

## Root Registration

All root-level commands are registered in `internal/cli/root.go` via `cmd.AddCommand()`. The `auth` command adds its subcommands in `internal/cli/auth.go`.

## Shared Infrastructure

- **RootOptions** — Persistent flags: `--account`, `--database`, `--schema`, `--role`, `--connection`, `--env`, `--quote-identifiers`, `--debug`
- **buildClient** / **buildClientAndCfg** — Construct API client from auth config; defined in `internal/cli/context.go`
- **ResolveTarget** / **ResolveTargetForExport** — Resolve database/schema from spec, opts, and config; in `internal/cli/resolve.go`
