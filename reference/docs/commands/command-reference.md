# Command Reference

Canonical inventory of all coragent commands and subcommands, with implementation entrypoints and primary dependencies.

## Root Commands

### plan [path]
- **Use:** `plan [path]`
- **Entry:** `newPlanCmd` → RunE closure
- **Dependencies:** `agent.LoadAgents`, `buildClientAndCfg`, `buildPlanItems`, `diff.DiffForCreate`, `diff.HasChanges`, `grant.GrantDiff`
- **Side effects:** API read (GetAgent, ShowGrants); stdout only; SQL query tag defaults to `coragent:plan`
- **Flags:** `-R`/`--recursive`

### apply [path]
- **Use:** `apply [path]`
- **Entry:** `newApplyCmd` → RunE closure
- **Dependencies:** `agent.LoadAgents`, `buildClientAndCfg`, `buildPlanItems`, `executeApply`, `diff`, `grant`, `config.LoadCoragentConfig`
- **Side effects:** API write (CreateAgent, UpdateAgent, ExecuteGrant, ExecuteRevoke); optional eval run; confirmation prompt; SQL query tag defaults to `coragent:apply`
- **Flags:** `-y`/`--yes`, `-R`/`--recursive`, `--eval`

### delete [path]
- **Use:** `delete [path]`
- **Entry:** `newDeleteCmd` → RunE closure
- **Dependencies:** `agent.LoadAgents`, `buildClientAndCfg`, `ResolveTarget`, `client.GetAgent`, `client.DeleteAgent`
- **Side effects:** API read + delete; confirmation prompt
- **Flags:** `-y`/`--yes`, `-R`/`--recursive`

### validate [path]
- **Use:** `validate [path]`
- **Entry:** `newValidateCmd` → RunE closure
- **Dependencies:** `agent.LoadAgents`
- **Side effects:** None (no API); stdout only
- **Flags:** `-R`/`--recursive`

### export <agent-name>
- **Use:** `export <agent-name>`
- **Entry:** `newExportCmd` → RunE closure
- **Dependencies:** `buildClientAndCfg`, `ResolveTargetForExport`, `client.DescribeAgent`
- **Side effects:** API read; stdout or file write (`-o`); SQL query tag defaults to `coragent:export`
- **Flags:** `-o`/`--out`

### new
- **Use:** `new`
- **Entry:** `newNewCmd` → `runNew`
- **Dependencies:** `agent.AgentSpec`, `readLine`, `promptWithDefault`, `reorderExportKeys`
- **Side effects:** File I/O (write YAML); interactive prompts
- **Flags:** None

### run [agent-name]
- **Use:** `run [agent-name]`
- **Entry:** `newRunCmd` → RunE closure
- **Dependencies:** `buildClientAndCfg`, `ResolveTargetForExport`, `api.RunAgent`, `thread.LoadState`, `thread.Save`
- **Side effects:** API (RunAgent, CreateThread); thread state read/write; streaming stdout/stderr. When agent-name is omitted, the pre-run agent lookup uses the `run` SQL query tag context.
- **Flags:** `-m`/`--message`, `--show-thinking`, `--new`, `--thread`, `--without-thread`

### threads
- **Use:** `threads`
- **Entry:** `newThreadsCmd` → RunE closure
- **Dependencies:** `thread.LoadState`, `buildClient`, `client.DeleteThread`
- **Side effects:** API (DeleteThread); thread state read/write; interactive UI
- **Flags:** `--list`, `--delete`

### eval [path]
- **Use:** `eval [path]`
- **Entry:** `newEvalCmd` → RunE closure
- **Dependencies:** `agent.LoadAgents`, `buildClientAndCfg`, `config.LoadCoragentConfig`, `api.RunAgent`, `eval_judge`
- **Side effects:** API (RunAgent); file I/O (JSON/MD reports)
- **Flags:** `-o`/`--output-dir`, `-R`/`--recursive`

### feedback [agent-name]
- **Use:** `feedback [agent-name]`
- **Entry:** `newFeedbackCmd` → RunE closure
- **Dependencies:** `config.LoadCoragentConfig`, `buildClientAndCfg`, `api.GetFeedback`, `api.FeedbackTableExists`, `api.SyncFeedbackFromEventsToTable`, `api.GetFeedbackFromTable`, `feedbackcache`
- **Side effects:** API read/write (feedback fetch, remote table sync/update/clear); feedback cache read/write in local mode; optional remote table init. With `--no-refresh`, skip API fetch in local mode and skip remote table sync in remote mode, reading only saved state before any optional checked updates. With `--infer-negative`, request-only interactions are selected from observability with a separate SQL query and individually scored via `SELECT SNOWFLAKE.CORTEX.AI_COMPLETE(...) AS response` using a single-string prompt plus structured output; the model can be overridden with `feedback.judge_model`, and both negative and positive inferred classifications are persisted so previously judged rows are not rescored on later runs. Remote mode requires a table initialized via `feedback --init`, then uses transient stage-table `INSERT` and final `MERGE` statements. When `feedback --init` finds an existing remote table, it can rename that table to a timestamped backup before recreating the configured table.
- **Side effects:** API read/write (feedback fetch, remote table sync/update/clear); feedback cache read/write in local mode; optional remote table init. With `--no-refresh`, skip API fetch in local mode and skip remote table sync in remote mode, reading only saved state before any optional checked updates. With `--infer-negative`, request-only interactions are selected from observability with a separate SQL query and individually scored via `SELECT SNOWFLAKE.CORTEX.AI_COMPLETE(...) AS response` using a single-string prompt plus structured output; the model can be overridden with `feedback.judge_model`, and both negative and positive inferred classifications are persisted so previously judged rows are not rescored on later runs. Remote mode requires a table initialized via `feedback --init`, then uses transient stage-table `INSERT` and final `MERGE` statements. When `feedback --init` finds an existing remote table, it can rename that table to a timestamped backup before recreating the configured table. SQL query tag defaults to `coragent:feedback`.
- **Flags:** `--all`, `--limit`, `--json` (returns `[]` when no records), `-y`/`--yes`, `--include-checked`, `--no-tools`, `--no-refresh`, `--infer-negative`, `--clear`, `--init`

### login
- **Use:** `login`
- **Entry:** `newLoginCmd` → `runLogin`
- **Dependencies:** `auth.NewCallbackServer`, `auth.GenerateState`, `auth.GeneratePKCE`, `auth.BuildAuthorizationURL`, `auth.ExchangeCodeForTokens`, `auth.LoadTokenStore`
- **Side effects:** OAuth flow (browser or manual URL); token store write
- **Flags:** `-a`/`--account`, `--redirect-uri`, `--no-browser`, `--timeout`

### logout
- **Use:** `logout`
- **Entry:** `newLogoutCmd` → `runLogout`
- **Dependencies:** `auth.LoadTokenStore`, `store.DeleteTokens`, `store.Clear`
- **Side effects:** Token store write (delete tokens)
- **Flags:** `-a`/`--account`, `--all`

## Auth Subcommands

### auth status
- **Use:** `auth status`
- **Entry:** `newAuthStatusCmd` → `runAuthStatus`
- **Dependencies:** `auth.LoadConfig`, `auth.DiagnoseConfig`, `auth.LoadTokenStore`
- **Side effects:** None (read-only)
- **Flags:** `-a`/`--account`

### auth init
- **Use:** `auth init`
- **Entry:** `newAuthInitCmd` → `runAuthInit`
- **Dependencies:** `auth.LoadSnowflakeConnection`, `auth.WriteConnection`, `promptWithDefault`
- **Side effects:** File I/O (create or update `~/.snowflake/config.toml`); interactive prompts
- **Flags:** `--force`
