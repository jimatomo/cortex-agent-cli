---
description: Analyze feedback to propose improvements to an agent, and apply changes to the YAML upon approval
allowed-tools:
  - Bash
  - Read
  - Edit
  - Write
  - Glob
  - AskUserQuestion
---

Analyze user feedback for a Cortex Agent, propose improvements to the agent spec, and apply them upon approval.

Usage: `/improve <agent-name> [-d DATABASE] [-s SCHEMA] [--limit N]`

Parse the arguments from: `$ARGUMENTS`

## Steps

### Step 1: Collect Feedback

Extract `<agent-name>`, `-d`/`--database`, `-s`/`--schema`, and `--limit` from the arguments.
If `-d` / `-s` are not specified, omit those options (the default connection settings will be used).

> **Note**: The `feedback` command uses a local cache (`~/.coragent/feedback/<agent>.json`).
> Checked records are not shown by default.
> **Always include `--json`** when fetching data to skip interactive prompts and get structured JSON.

First, check the total count of all feedback (including checked, all sentiments):

```bash
coragent feedback <agent-name> [--database DB] [--schema SCHEMA] --all --include-checked --json [--limit N]
```

Then fetch unchecked negative feedback (used for improvement analysis):

```bash
coragent feedback <agent-name> [--database DB] [--schema SCHEMA] --json [--limit N]
```

Parse the returned JSON to read `sentiment`, `comment`, `question`, and `response`.
If there are 0 unchecked negative feedback entries, inform the user that there is no feedback available for improvement proposals at this time, and stop.

### Step 2: Retrieve the Current Agent Spec

Use Glob to search for `**/*.yml` and `**/*.yaml` and check whether a local YAML file matching `<agent-name>` exists.

- **If a local file exists**: Load it with `Read`. Record the file path.
- **If no local file exists**: Retrieve the spec from the remote using `coragent export <agent-name> [--database DB] [--schema SCHEMA]`. Do not create a file at this point.

### Step 3: Analyze and Propose Improvements

Cross-reference the collected feedback with the agent spec and analyze/propose the following:

1. **Feedback pattern classification**
   - Answer accuracy issues (incorrect information, incomplete responses)
   - Handling of out-of-scope questions
   - Response style issues (too long / too short, unclear)
   - Other categories

2. **Improvement points in the agent spec** (especially the `instructions` section)
   - Issues with the current `instructions`
   - Content that should be added or revised

3. **Concrete change proposals**
   - Clearly show the changes to `instructions` in before/after format

Even if feedback exists, issues that cannot be resolved by spec changes (e.g., missing data sources) should be excluded from the change proposals and communicated separately as comments.

### Step 4: Request Permission to Modify the YAML

If spec changes are deemed effective, use `AskUserQuestion` to request the user's approval.

Information to present:
- What is being changed (e.g., around which lines of `instructions`)
- Before (original)
- After (proposed)

Always include a "Do not modify" option in the choices.

### Step 5: Modify the YAML File (Only Upon Approval)

If the user approves the changes:

1. **If a local file exists**: Modify it directly using the `Edit` tool.
2. **If no local file exists**: First export the file:
   ```bash
   coragent export <agent-name> [--database DB] [--schema SCHEMA] --out <agent-name>.yml
   ```
   Then modify it using the `Edit` tool.

After editing, display the changes again.

### Step 6: Request Permission to Run Apply

After modifying the YAML, use `AskUserQuestion` to confirm whether to run apply.

If approved, run:

```bash
coragent apply <path-to-yaml> [--database DB] [--schema SCHEMA]
```

Notify the user when apply succeeds.

### Step 7: Mark Feedback as Checked

Use `AskUserQuestion` to confirm whether to mark the analyzed feedback as checked.

If approved, run the following (`-y` auto-skips the confirmation prompt):

```bash
coragent feedback <agent-name> [--database DB] [--schema SCHEMA] -y
```

This marks all unchecked negative feedback reviewed in this session as checked, so it will not appear in future `feedback` / `improve` runs.
