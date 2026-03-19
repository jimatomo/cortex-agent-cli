# Config, FeedbackCache, and Thread Components

## Config (`internal/config`)

### Purpose

Load project and user settings from `.coragent.toml`.

### Key File

- `internal/config/config.go` — `LoadCoragentConfig`, `CoragentConfig` struct

### Search Order

1. `.coragent.toml` in current directory
2. `~/.coragent/config.toml`

### Settings (Eval)

- `eval.output_dir` — Output directory for eval reports
- `eval.timestamp_suffix` — Append timestamp to output filenames
- `eval.judge_model` — Model for LLM-as-a-Judge (default: `llama4-scout`)
- `eval.response_score_threshold` — Score threshold (0 to disable)
- `eval.ignore_tools` — Tool names excluded from eval tool-match checks (default includes `data_to_chart`)

### Settings (Feedback)

- `feedback.judge_model` — Model used by `feedback --infer-negative` (default: `llama4-scout`)
- `feedback.remote.enabled` — Enable remote feedback table mode
- `feedback.remote.database`, `schema`, `table` — Remote feedback table location

### Settings (Query Tag)

- `query_tag.base` — Base query tag value for supported Snowflake requests; defaults to `coragent`
- Effective tag format — `<base>:<command>` for command-scoped requests such as `plan`, `apply`, `run`, `eval`, and `feedback`

## FeedbackCache (`internal/feedbackcache`)

### Purpose

Cache feedback records locally to support incremental review and "checked" filtering.

### Key Files

- `internal/feedbackcache/cache.go` — `Load`, `Save`, `CachePath`, `Cache.Merge`, `Cache.LatestTimestamp`

### Storage

- **Path:** `~/.coragent/feedback/<agent>.json`
- **Format:** JSON object with a top-level `records` array
- **Merge:** New records from `GetFeedback` merged with cache by `record_id`; checked state is preserved while refreshed records can replace older inferred data
- **Checked:** Records can be marked checked; `--include-checked` shows them; default hides
- **No refresh:** `feedback --no-refresh` reads the existing cache as-is and skips `GetFeedback` plus cache rewrite
- **Inference metadata:** When `feedback --infer-negative` is used, cached records can include inferred sentiment provenance/reason fields

### Inference Semantics

- **Default mode:** Cache contains only explicit feedback-derived rows.
- **`--infer-negative` mode:** Cache may additionally contain request-only rows that were classified by `SNOWFLAKE.CORTEX.AI_COMPLETE`; negative results surface by default, while positive inferred rows are still cached so they are not re-judged on later runs.
- **Refresh scope:** In inference mode the command keeps request-only inference incremental with `LatestTimestamp()`, but explicit feedback is reloaded on each refresh so newer explicit feedback cannot be hidden behind a later request timestamp from another record.
- **Conflict rule:** If the same `record_id` later receives explicit feedback, the explicit row replaces the inferred row's mutable payload while preserving `Checked`.

### SQL Interaction Summary

- **Explicit-only refresh:** one observability `SELECT` over `GET_AI_OBSERVABILITY_EVENTS` with feedback/request `LEFT JOIN`
- **Inference refresh:** the explicit `SELECT` above plus:
  - one request-only candidate `SELECT`
  - one `SELECT SNOWFLAKE.CORTEX.AI_COMPLETE(...)` per candidate row
- **Remote inferred persistence:** may additionally issue:
  - one schema check to confirm `sentiment_source` / `sentiment_reason` already exist
  - `CREATE TRANSIENT TABLE ...`
  - batched `INSERT INTO ... UNION ALL ...`
  - final `MERGE INTO ...`

## Thread (`internal/thread`)

### Purpose

Persist conversation thread state for `run` and `threads` commands.

### Key Files

- `internal/thread/state.go` — `StateStore`, `LoadState`, `Save`, `GetThreads`, `FindThread`, `AddOrUpdateThread`, `DeleteThread`, `GetAllThreads`

### Storage

- **Path:** `~/.coragent/threads.json`
- **Structure:** Map of agent key (e.g., `db/schema/agent`) → list of `ThreadState` (ThreadID, Summary, LastUsed)
- **Agent key:** `account/database/schema/agentName` format

### Usage

- **run** — Load state; select or create thread; update summary/last-used on completion; save
- **threads** — Load state; display; delete via API; remove from state; save

## Related Docs

- [flows/query-feedback-threads-flow.md](../flows/query-feedback-threads-flow.md) — How these are used in run, feedback, threads
