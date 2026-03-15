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
4. **Inference mode** — With `--infer-negative`, include request-only interactions that `SNOWFLAKE.CORTEX.AI_COMPLETE` classifies as implicit negative feedback
5. **Init** — `--init` creates the remote feedback table when using remote mode; if the table already exists, the CLI can first rename it to a backup table before recreating the primary table

### Steps

1. Load `config.LoadCoragentConfig()` for feedback settings
2. Resolve remote DB/schema/table if enabled
3. If remote mode: ensure table exists, optionally sync new events (`SyncFeedbackFromEventsToTable`) unless `--no-refresh`, then fetch rows (`GetFeedbackFromTable`)
4. If local mode: load local cache (`~/.coragent/feedback/<agent>.json`), optionally fetch feedback via API (`GetFeedback`) unless `--no-refresh`, then merge and save
5. If `--infer-negative` is enabled: explicit feedback stays unchanged, and request-only interactions may be added after judge classification; inferred rows keep provenance metadata in JSON/output and remote persistence, including positive classifications that are cached to avoid re-judging on later runs
6. Display records (default negative only; `--all` for all). Response bodies are printed in full without truncation in the interactive text output
7. Prompt to mark as checked; update remote table or local cache depending on mode
8. In non-JSON mode, the command prints short progress lines to stdout while loading cache/remote state, refreshing, and preparing the final record list

### `--infer-negative` Detailed Flow

1. Start from the normal feedback flow; explicit `CORTEX_AGENT_FEEDBACK` rows are always loaded first.
2. When the flag is enabled, query `CORTEX_AGENT_REQUEST` rows that do not have a matching feedback event for the same `record_id`.
3. Extract `question`, `response`, `tool_uses`, and `response_time_ms` from the request payload.
4. Build a single-string structured-output `SNOWFLAKE.CORTEX.AI_COMPLETE` prompt asking whether the interaction should be treated as implicit negative feedback because the user's goal was substantially unmet. The model defaults to `llama4-scout` and can be overridden with `feedback.judge_model`.
5. Convert the model result into inferred sentiment:
   - `negative = true` becomes `sentiment = negative`
   - `negative = false` becomes `sentiment = positive`
   - attach provenance fields such as `sentiment_source = inferred` and the returned reasoning in either case
6. Merge explicit and inferred rows by `record_id`:
   - explicit feedback wins over inferred feedback for the same record
   - local cache keeps checked state while refreshing mutable fields
   - remote persistence updates mutable fields in-place during upsert and normalizes inferred-row timestamps so `event_ts` remains populated
7. If `--no-refresh` is also set, none of the above inference work runs; the command only reads previously saved rows.
8. During refresh, inference mode keeps request-only candidates incremental via the request timestamp cursor, but explicit feedback is always reloaded so late-arriving feedback cannot be skipped by a newer inferred-request timestamp from another record.

### SQL Statements

#### Explicit Feedback Fetch

The normal fetch path issues a `SELECT` over two `GET_AI_OBSERVABILITY_EVENTS` table-function calls:

- source alias `f` for `CORTEX_AGENT_FEEDBACK`
- source alias `r` for `CORTEX_AGENT_REQUEST`
- join key: `ai.observability.record_id`
- optional incremental predicate:
  - explicit-only mode: `f.TIMESTAMP >= TO_TIMESTAMP_TZ('<since>', 'YYYY-MM-DD HH24:MI:SS.FF3 TZHTZM')`
  - `--infer-negative` mode: no explicit-feedback timestamp filter, so explicit rows always refresh

This returns feedback payload plus the associated request payload so Go can extract `question`, `response`, `tool_uses`, and `response_time_ms`.

#### Request-Only Candidate Fetch

Inference mode adds a second `SELECT`:

- source alias `r` for `CORTEX_AGENT_REQUEST`
- `LEFT JOIN` source alias `f` for `CORTEX_AGENT_FEEDBACK`
- filter:
  - `r.RECORD:name = 'CORTEX_AGENT_REQUEST'`
  - `f.RECORD:name IS NULL`
  - when diff refresh is active: `r.TIMESTAMP >= TO_TIMESTAMP_TZ('<request-since>', 'YYYY-MM-DD HH24:MI:SS.FF3 TZHTZM')`

This yields only interactions that have request telemetry but no explicit feedback row yet.

#### COMPLETE-Based Negative Inference

For each request-only candidate, the command executes:

- `SELECT SNOWFLAKE.CORTEX.AI_COMPLETE(...) AS response`

with these characteristics:

- model: `llama4-scout`
- `temperature = 0`
- structured output schema:
  - `negative: boolean`
  - `reasoning: string`
- prompt inputs:
  - extracted user question
  - extracted agent response
  - summarized tool chain

Only `negative = true` results are retained.

### Remote Persistence Notes

- Default remote sync remains SQL-only for explicit feedback.
- With `--infer-negative`, remote sync switches to Go-side materialization so explicit and inferred rows can be merged through the same `UpsertFeedbackRecords` path.
- `feedback --init` creates the remote table with `sentiment_source` and `sentiment_reason`; if an older table is missing those columns, `--infer-negative` stops and tells the user to rerun `feedback --init`.
- When `feedback --init` finds an existing remote table, it first prompts whether to rename that table to a timestamped backup name before recreating the configured table. If the user declines the rename, the CLI falls back to a destructive recreate confirmation.

#### Remote Sync Without `--infer-negative`

The default remote sync path stays in SQL and runs a single large `MERGE`:

- `MERGE INTO <remote_table> AS tgt USING (...) AS s`
- CTE pipeline:
  - `all_events`
  - `transformed_events`
  - `joined_feedback_and_request`
  - `question_agg`
  - `response_agg`
  - `tool_use_rows`
  - `tool_result_rows`
  - `tool_result_agg`
  - `tool_agg`
  - `enriched_feedback_and_request`
  - `final`

Within that SQL:

- `LISTAGG` reconstructs question and response text
- `LATERAL FLATTEN` walks message content and tool content arrays
- `ARRAY_AGG(OBJECT_CONSTRUCT(...))` rebuilds `tool_uses`
- `CASE WHEN TRY_TO_BOOLEAN(VALUE:positive::STRING) THEN 'positive' ELSE 'negative' END` derives explicit sentiment

#### Remote Sync With `--infer-negative`

Inference mode switches to a Go-orchestrated SQL sequence:

1. verify the remote table already has `sentiment_source` and `sentiment_reason`
2. if either column is missing, stop and instruct the user to run `coragent feedback --init` so the table is recreated with the full schema
3. explicit/request-only rows are fetched and classified in Go
4. `CREATE TRANSIENT TABLE TMP_FEEDBACK_STAGE_<timestamp> (...)`
5. batched `INSERT INTO <stage> ... UNION ALL ...`
6. final upsert:
   - `MERGE INTO <target> AS t USING (<stage_select>) AS s ON t.record_id = s.record_id`
   - `WHEN MATCHED THEN UPDATE SET ...`
   - `WHEN NOT MATCHED THEN INSERT (...)`

The `WHEN MATCHED` branch updates mutable fields such as `sentiment`, `sentiment_source`, `sentiment_reason`, `feedback_message`, `question`, `response`, `tool_uses`, and `request_value`, but does not overwrite checked-state columns.

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
