# Query, Feedback, and Threads Flow

These flows cover agent execution (`run`), feedback retrieval (`feedback`), and thread management (`threads`).

## Run Flow

### Entry

- **Command:** `coragent run [agent-name]`
- **Entry:** `newRunCmd` in `internal/cli/run.go`

### Steps

1. **Resolve target** — `ResolveTargetForExport(opts, cfg)` → database, schema
2. **Agent selection** — If agent-name omitted, prompt user to select from `ListAgents`
3. **Thread selection** — Unless `--new`, `--thread`, or `--without-thread`:
   - Load `thread.LoadState()` from `~/.coragent/threads.json`
   - Prompt to select existing thread or create new
4. **Run** — `client.RunAgent` with message; stream response events
5. **State update** — On completion, update thread state (summary, last used) and save

### Dependencies

- `internal/api` — `RunAgent`, `CreateThread`, `ListAgents`
- `internal/thread` — `LoadState`, `Save`, thread CRUD

## Feedback Flow

### Entry

- **Command:** `coragent feedback [agent-name]`
- **Entry:** `newFeedbackCmd` in `internal/cli/feedback.go`

### Modes

1. **Remote table mode** — When `feedback.remote.enabled` in `.coragent.toml` and remote DB/schema/table are configured: sync events into the remote table, then read/update records from that table
2. **API + local cache mode** — Uses `GetFeedback` REST API and local cache when remote mode is not configured
3. **No-refresh read-only mode** — With `--no-refresh`, skip new-event fetch/sync and read only the existing local cache or existing remote table rows before any optional checked updates
4. **Init** — `--init` creates the remote feedback table when using remote mode

### Steps

1. Load `config.LoadCoragentConfig()` for feedback settings
2. Resolve remote DB/schema/table if enabled
3. If remote mode: ensure table exists, optionally sync new events (`SyncFeedbackFromEventsToTable`) unless `--no-refresh`, then fetch rows (`GetFeedbackFromTable`)
4. If local mode: load local cache (`~/.coragent/feedback/<agent>.json`), optionally fetch incremental feedback via API (`GetFeedback`) unless `--no-refresh`, then merge and save
5. Display records (default negative only; `--all` for all). Response bodies are printed in full without truncation in the interactive text output
6. Prompt to mark as checked; update remote table or local cache depending on mode

### Dependencies

- `internal/api` — `GetFeedback`, `FeedbackTableExists`, `SyncFeedbackFromEventsToTable`, `GetFeedbackFromTable`, `UpdateFeedbackChecked`, `ClearFeedbackForAgent`
- `internal/feedbackcache` — Load, Save, Merge
- `internal/config` — Feedback remote config

## Threads Flow

### Entry

- **Command:** `coragent threads`
- **Entry:** `newThreadsCmd` in `internal/cli/threads.go`

### Modes

1. **List** — `--list`: display threads from local state only (no API)
2. **Delete** — `--delete <id>`: delete specific thread via API, update local state
3. **Interactive** — Default: show threads, prompt to delete; uses API for delete

### Steps

1. Load `thread.LoadState()`
2. If list-only: display and exit
3. If delete-by-id: find thread in state, call `client.DeleteThread`, remove from state, save
4. If interactive: loop display → prompt (d/delete, q/quit) → delete selected → save

### Dependencies

- `internal/api` — `DeleteThread` (via `buildClient`, no config needed for list-only mode)
- `internal/thread` — `LoadState`, `GetAllThreads`, `DeleteThread`, `Save`

## Related Docs

- [components/config-feedbackcache-thread.md](../components/config-feedbackcache-thread.md)
- [components/api.md](../components/api.md) — RunService, ThreadService, QueryService
