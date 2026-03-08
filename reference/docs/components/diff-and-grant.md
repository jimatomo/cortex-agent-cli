# Diff and Grant Components

## Diff Package (`internal/diff`)

### Purpose

Compute the difference between desired spec (from YAML) and remote spec (from API) for update planning.

### Key Types

- **Change** — `Path`, `Type` (Added/Removed/Modified), `Before`, `After`
- **ChangeType** — `Added`, `Removed`, `Modified`

### Key Functions

- **Diff(local, remote)** — Returns `[]Change` comparing local spec against remote; used when agent exists
- **DiffForCreate(spec)** — Returns changes representing "what will be created"; used for plan create output and delete "what will be removed"
- **HasChanges(changes)** — True if any non-empty change list

### Behavior

- Compares top-level and nested fields; produces dot-notation paths (e.g., `instructions.response`)
- Empty vs nil handling aligned with Snowflake API expectations
- Used by plan/apply to build update payloads; `updatePayload` in apply maps changes to top-level keys for PATCH
- CLI previews render diff string values in full without truncation, preserving UTF-8 text such as Japanese in `plan`, `apply`, and `delete`
- In `plan` and the `apply` preview, unchanged agents are omitted from the detailed body; only create/update targets are shown, while unchanged counts remain in the summary

## Grant Package (`internal/grant`)

### Purpose

Compute and execute GRANT/REVOKE diffs to converge agent privileges to the desired state.

### Key Types

- **GrantConfig** — From `spec.Deploy.Grant`; `AccountRoles`, `DatabaseRoles`
- **GrantState** — Current state from `ShowGrants` rows
- **GrantDiff** — `ToGrant`, `ToRevoke`; `HasChanges()`

### Key Functions

- **FromGrantConfig(cfg)** — Convert YAML grant config to internal grant set
- **FromShowGrantsRows(rows)** — Convert API rows to current state
- **ComputeDiff(desired, current)** — Returns `GrantDiff` with ToGrant and ToRevoke
- **applyGrantDiff** (in cli) — Executes REVOKE first, then GRANT

### Privileges

Valid privileges: `USAGE`, `MODIFY`, `MONITOR`, `ALL` (expands to USAGE+MODIFY+MONITOR).

### API Integration

- **ShowGrants** — Returns rows with `Privilege`, `GrantedTo` (ACCOUNT_ROLE/DATABASE_ROLE), `GranteeName`
- **ExecuteGrant** / **ExecuteRevoke** — Run SQL via API

### Grant Unspecified

When `deploy.grant` is not specified in the YAML spec, grant logic is skipped entirely:
- `ShowGrants` is not called (avoids unnecessary API call)
- No GRANT/REVOKE statements are executed
- Existing grants on the agent are left unchanged

## Related Docs

- [flows/plan-apply-flow.md](../flows/plan-apply-flow.md) — How diff and grant are used
- [components/api.md](api.md) — GrantService interface
