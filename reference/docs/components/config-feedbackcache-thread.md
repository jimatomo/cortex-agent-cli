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

- `feedback.remote.enabled` — Enable remote feedback table mode
- `feedback.remote.database`, `schema`, `table` — Remote feedback table location

## FeedbackCache (`internal/feedbackcache`)

### Purpose

Cache feedback records locally to support incremental review and "checked" filtering.

### Key Files

- `internal/feedbackcache/cache.go` — `Load`, `Save`, `CachePath`, `Cache.Merge`, `Cache.LatestTimestamp`

### Storage

- **Path:** `~/.coragent/feedback/<agent>.json`
- **Format:** JSON array of feedback records
- **Merge:** New records from `GetFeedback` merged with cache; duplicates avoided
- **Checked:** Records can be marked checked; `--include-checked` shows them; default hides

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
