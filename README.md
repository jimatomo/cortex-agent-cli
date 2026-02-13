# coragent (Cortex Agent CLI)

CLI tool for managing Snowflake Cortex Agent deployments via the REST API.

⚠️ This is an unofficial tool developed by a Snowflake user.

## Features

- Deploy Cortex Agents from YAML files
- Plan/Apply/Delete workflow with diff detection (PUT update only when changed)
- Grant management with diff-based GRANT/REVOKE on agents
- Validate YAML schema (unknown fields rejected)
- Export existing agents to YAML
- Run agents with streaming response and multi-turn conversation support
- Evaluate agent accuracy with test cases defined in YAML
- Recursive directory scanning for multi-agent projects
- Key Pair (RSA JWT) authentication
- OAuth authentication (experimental)

## Install

### Quick Install (Recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/jimatomo/cortex-agent-cli/main/install.sh | sh
```

The script detects OS/architecture, verifies checksum, and installs to `~/.local/bin` by default.

```bash
# Install specific version
curl -fsSL https://raw.githubusercontent.com/jimatomo/cortex-agent-cli/main/install.sh | sh -s -- -v v0.1.0

# Install to custom directory
curl -fsSL https://raw.githubusercontent.com/jimatomo/cortex-agent-cli/main/install.sh | sh -s -- -d ~/bin

# Force reinstall
curl -fsSL https://raw.githubusercontent.com/jimatomo/cortex-agent-cli/main/install.sh | sh -s -- --force
```

### Manual Download

Download the appropriate binary from [GitHub Releases](https://github.com/jimatomo/cortex-agent-cli/releases) and place it in your `$PATH`.

### From Source

```bash
git clone <your-repo-url>
cd coragent
go build -o coragent ./cmd/coragent
```

## Quick Start

### 1) Configure credentials

Choose one of the following authentication methods:

**Option A: Key Pair via config.toml (Recommended)**

Use `~/.snowflake/config.toml` — the same file used by Snowflake CLI and the Python connector. See [config.toml details](#snowflake-cli-configtoml) for search paths and all supported fields.

```toml
default_connection_name = "myconn"

[connections.myconn]
account = "your_account"
user = "your_user"
role = "CORTEX_USER"
database = "MY_DATABASE"
schema = "MY_SCHEMA"
authenticator = "SNOWFLAKE_JWT"
private_key_file = "~/.snowflake/rsa_key.p8"
```

```bash
coragent plan                    # uses default connection
coragent plan --connection myconn  # uses named connection
```

**Option B: OAuth via config.toml (Recommended for interactive use)**

```toml
# ~/.snowflake/config.toml
default_connection_name = "myconn"

[connections.myconn]
account = "your_account"
role = "CORTEX_USER"
database = "MY_DATABASE"
schema = "MY_SCHEMA"
authenticator = "OAUTH_AUTHORIZATION_CODE"
```

```bash
coragent login   # opens browser for authentication
coragent plan    # OAuth is selected automatically via authenticator setting
```

See [OAuth Authentication](#oauth-authentication-experimental) for details on environment variables, token lifecycle, and advanced options.

**Option C: Key Pair via environment variables (Recommended for CI/CD)**

```bash
export SNOWFLAKE_ACCOUNT=your_account
export SNOWFLAKE_USER=your_user
export SNOWFLAKE_ROLE=CORTEX_USER
# Provide the private key contents directly (PEM or base64-encoded PEM)
export SNOWFLAKE_PRIVATE_KEY="-----BEGIN PRIVATE KEY-----
...
-----END PRIVATE KEY-----"
# If the private key is encrypted, also set the passphrase
# export SNOWFLAKE_PRIVATE_KEY_PASSPHRASE=your_passphrase
# (PRIVATE_KEY_PASSPHRASE is also supported as a fallback)
```

### 2) Define an agent in YAML

```yaml
deploy:
  database: MY_DATABASE
  schema: MY_SCHEMA

name: my-support-agent
comment: Customer support agent
profile:
  display_name: Support Bot
models:
  orchestration: claude-4-sonnet
instructions:
  response: |
    You are a helpful customer support agent.
orchestration:
  budget:
    seconds: 60
    tokens: 16000
tools:
  - tool_spec:
      type: cortex_search
      name: knowledge_search
tool_resources:
  cortex_search:
    - name: knowledge_search
      service: KNOWLEDGE_SERVICE
```

### 3) Plan and apply

```bash
# Plan (defaults to current directory)
coragent plan

# Apply
coragent apply
```

## Configuration Priority

Settings are resolved in the following order (highest priority first):

1. CLI flags: `--database`, `--schema`, `--role`, `--account`, `--connection`
2. YAML `deploy` section (database/schema only)
3. Environment variables: `SNOWFLAKE_DATABASE`, `SNOWFLAKE_SCHEMA`, etc.
4. Snowflake CLI config.toml (`~/.snowflake/config.toml`)

### Snowflake CLI config.toml

The CLI reads `~/.snowflake/config.toml` — the same file used by Snowflake CLI (`snow`) and the Python connector. The file is searched in the following order:

1. `$SNOWFLAKE_HOME/config.toml`
2. `~/.snowflake/config.toml`
3. `~/.config/snowflake/config.toml` (Linux only)

Use `--connection` / `-c` to select a named connection. If omitted, `default_connection_name` (or the `SNOWFLAKE_DEFAULT_CONNECTION_NAME` env var) is used.

#### Supported config.toml fields

| Field | Description |
|-------|-------------|
| `account` | Snowflake account identifier |
| `user` | Snowflake user name |
| `role` | Snowflake role |
| `warehouse` | Snowflake warehouse |
| `database` | Default database |
| `schema` | Default schema |
| `authenticator` | Auth method: `SNOWFLAKE_JWT` (key pair) or `OAUTH_AUTHORIZATION_CODE` |
| `private_key_file` | Path to private key file (supports `~` expansion) |
| `private_key_path` | Alias for `private_key_file` |
| `private_key_raw` | Private key content inline |
| `oauth_redirect_uri` | OAuth redirect URI (default: `http://127.0.0.1:8080`) |
| `oauth_client_id` | OAuth client ID (default: `LOCAL_APPLICATION`) |
| `oauth_client_secret` | OAuth client secret (default: `LOCAL_APPLICATION`) |

## OAuth Authentication (Experimental)

> **Warning**: OAuth authentication is an experimental feature. The API and behavior may change in future versions.

Snowflake OAuth (Authorization Code Flow) authenticates via browser using the built-in `SNOWFLAKE$LOCAL_APPLICATION` security integration. No Snowflake-side setup is required in most cases. For details, see [Snowflake OAuth for Local Applications](https://docs.snowflake.com/en/user-guide/oauth-local-applications).

### Configuration

Set `authenticator` to `OAUTH_AUTHORIZATION_CODE` in config.toml (see [Quick Start](#1-configure-credentials) for a full example), or use environment variables:

```bash
export SNOWFLAKE_ACCOUNT=your_account
export SNOWFLAKE_AUTHENTICATOR=OAUTH
# Optional: custom redirect URI (default: http://127.0.0.1:8080)
# export SNOWFLAKE_OAUTH_REDIRECT_URI=http://127.0.0.1:9090
```

To customize the redirect URI in config.toml:

```toml
[connections.myconn]
account = "your_account"
authenticator = "OAUTH_AUTHORIZATION_CODE"
oauth_redirect_uri = "http://127.0.0.1:9090"
```

### Login

```bash
coragent login                       # account from config.toml or env
coragent login --account your_account  # explicit account
coragent login --connection myconn     # named connection
coragent login --no-browser            # print URL instead of opening browser
coragent login --timeout 10m           # custom timeout (default: 5m)
```

Tokens are stored in `~/.coragent/oauth.json` and automatically refreshed when a refresh token is available.

| Flag | Description |
|------|-------------|
| `-a, --account` | Snowflake account identifier (overrides config.toml / env) |
| `--redirect-uri` | OAuth redirect URI (default: `http://127.0.0.1:8080`) |
| `--no-browser` | Print the authorization URL instead of opening a browser |
| `--timeout` | Timeout waiting for authentication (default: `5m`) |

### Token Lifecycle

- **Access tokens** expire after approximately 10 minutes (set by Snowflake).
- **Refresh tokens** are used automatically to renew expired access tokens without re-authentication.
- If the refresh token itself expires or is revoked, run `coragent login` again.

### Status and Logout

```bash
coragent auth status                   # show token status
coragent logout --account your_account   # logout from specific account
coragent logout --all                  # logout from all accounts
```

### OAuth Environment Variables

| Variable | Description |
|----------|-------------|
| `SNOWFLAKE_AUTHENTICATOR` | Set to `OAUTH` to enable OAuth authentication |
| `SNOWFLAKE_OAUTH_REDIRECT_URI` | Redirect URI (default: `http://127.0.0.1:8080`) |

## Commands

| Command | Description |
|---------|-------------|
| `coragent plan [path]` | Show execution plan without applying (default: `.`) |
| `coragent apply [path]` | Apply changes to agents (default: `.`) |
| `coragent delete [path]` | Delete agents defined in YAML files (default: `.`) |
| `coragent validate [path]` | Validate YAML files only (default: `.`) |
| `coragent export <agent-name>` | Export existing agent to YAML |
| `coragent run [agent-name]` | Run an agent with streaming response (interactive selection if omitted) |
| `coragent eval [path]` | Evaluate agent accuracy using test cases (default: `.`) |
| `coragent threads` | Manage conversation threads |
| `coragent login` | Authenticate with Snowflake using OAuth |
| `coragent logout` | Remove stored OAuth tokens |
| `coragent auth status` | Show authentication status |

## Global Flags

- `--account` / `-a`: Snowflake account identifier
- `--database` / `-d`: Target database
- `--schema` / `-s`: Target schema
- `--role` / `-r`: Snowflake role to use
- `--connection` / `-c`: Snowflake CLI connection name (from `~/.snowflake/config.toml`)
- `--quote-identifiers`: Double-quote database/schema names for case-sensitive identifiers
- `--debug`: Enable debug logging with stack trace

## Plan/Apply Behavior

- **CREATE**: Agent does not exist → `POST`
- **UPDATE**: Agent exists with changes → `PUT` with changed top-level fields
- **NO_CHANGE**: Agent exists, no diff → Skip API call

| Flag | Commands | Description |
|------|----------|-------------|
| `-R, --recursive` | plan, apply, delete, validate | Recursively load agents from subdirectories |
| `-y, --yes` | apply, delete | Skip confirmation prompt |
| `--eval` | apply | Run eval tests for changed agents after apply |

## Delete

Delete agents defined in YAML files. Shows a plan and asks for confirmation before deleting.

```bash
coragent delete                # current directory
coragent delete agent.yaml     # specific file
coragent delete ./agents -R    # recursive
coragent delete -y             # skip confirmation
```

## Export

```bash
# Output to stdout
coragent export my-agent

# Output to file
coragent export my-agent --out ./my-agent.yaml
```

## Run

Run an agent with streaming response. If agent-name or `-m` is omitted, interactive prompts are shown.

```bash
coragent run                                           # fully interactive
coragent run my-agent -m "What are the top sales?"     # specify both
coragent run my-agent --new -m "Starting fresh topic"  # new thread
coragent run my-agent --thread 12345 -m "Follow-up"   # continue thread
coragent run my-agent --without-thread -m "One-off"    # single-turn (no thread)
coragent run my-agent -m "Query" --show-thinking       # show reasoning
```

### Thread Support

Threads enable multi-turn conversations via the Snowflake Cortex Threads API. Thread state is stored locally in `~/.coragent/threads.json`. Tool usage is always displayed on stderr.

### Run Flags

| Flag | Description |
|------|-------------|
| `-m, --message` | Message to send (interactive prompt if omitted) |
| `--new` | Start a new conversation thread |
| `--thread <id>` | Continue a specific thread by ID |
| `--without-thread` | Single-turn mode (no thread tracking) |
| `--show-thinking` | Display reasoning tokens on stderr |

## Project Configuration (`.coragent.toml`)

Project-level settings are loaded from `.coragent.toml` (current directory) or `~/.coragent/config.toml`. CLI flags override these values.

```toml
[eval]
output_dir = "./eval-results"
timestamp_suffix = true   # append UTC timestamp to output filenames
```

## Eval

Evaluate agent accuracy by running test cases defined in the YAML spec file's `eval` section. Each test can verify expected tool usage, run a custom command for validation, or both.

### YAML Definition

Add an `eval` section to your agent spec:

```yaml
eval:
  tests:
    # Tool matching only
    - question: "Show me the sales data"
      expected_tools:
        - sample_semantic_view

    # Tool matching + custom command
    - question: "Search the Snowflake docs"
      expected_tools:
        - snowflake_docs_service
      command: "python eval_check.py"

    # Custom command only (no agent call when question is omitted)
    - command: "python eval_standalone.py"
```

### Test Case Fields

| Field | Required | Description |
|-------|----------|-------------|
| `question` | No | Question to send to the agent. If omitted, the agent call is skipped. |
| `expected_tools` | No* | List of tool names that must appear in the agent's response |
| `command` | No* | Shell command to run after the agent responds (or standalone if no question) |

\* At least one of `expected_tools` or `command` is required.

### Custom Command

When `command` is specified, it is executed via `sh -c` with the working directory set to the YAML file's directory. The command receives a JSON payload on stdin:

```json
{
  "question": "the question sent to the agent",
  "response": "the agent's text response",
  "actual_tools": ["tool_a", "tool_b"],
  "expected_tools": ["tool_a"],
  "thread_id": "12345"
}
```

- **Exit code 0** = pass, **non-zero** = fail
- stdout/stderr are captured and included in the report
- If both `expected_tools` and `command` are specified, both must pass for the test to pass

### Usage

```bash
coragent eval                          # current directory
coragent eval agent.yaml               # specific file
coragent eval ./agents/ -R             # recursive
coragent eval agent.yaml -o ./results  # custom output directory
```

### Output

Two report files are generated per agent: `{agent_name}_eval.json` (machine-readable) and `{agent_name}_eval.md` (markdown report). With `timestamp_suffix = true` in `.coragent.toml`, filenames include a UTC timestamp (e.g., `{agent_name}_eval_20260212_103000.json`).

Output directory priority: `-o` flag > `eval.output_dir` in `.coragent.toml` > `.` (current directory).

| Icon | Meaning |
|------|---------|
| ✅ | Test passed |
| ⚠️ | Passed with extra/duplicate tool calls |
| ❌ | Test failed |

## Threads

Manage conversation threads across all agents.

```bash
coragent threads                   # interactive mode (list, select, delete)
coragent threads --list            # list all threads (non-interactive)
coragent threads --delete 29864464   # delete a specific thread by ID
```

## YAML Spec Reference

A complete example showing all supported fields:

```yaml
deploy:
  database: MY_DATABASE
  schema: MY_SCHEMA
  quote_identifiers: true  # Double-quote database/schema for case-sensitive identifiers
  grant:
    account_roles:
      - role: ANALYST
        privileges:
          - ALL            # Expands to USAGE, MODIFY, MONITOR
    database_roles:
      - role: MY_DATABASE.CORTEX_MONITOR
        privileges:
          - MONITOR

eval:
  tests:
    - question: "Show me the sales data"
      expected_tools:
        - sample_semantic_view
    - question: "Search the Snowflake docs"
      expected_tools:
        - snowflake_docs_service
      command: "python eval_check.py"
    - command: "python eval_standalone.py"

name: my-support-agent
comment: Customer support agent
profile:
  display_name: Support Bot
models:
  orchestration: claude-4-sonnet
instructions:
  response: |
    You are a helpful customer support agent.
  orchestration: |
    Think step by step before answering.
  system: |
    You are a helpful assistant.
  sample_questions:
    - question: "How do I analyze sales data?"
    - question: "What is a pivot table?"
orchestration:
  budget:
    seconds: 60
    tokens: 16000
tools:
  - tool_spec:
      type: cortex_analyst_text_to_sql
      name: sample_semantic_view
      description: A semantic view for sales data
  - tool_spec:
      type: cortex_search
      name: snowflake_docs_service
      description: Snowflake documentation search
tool_resources:
  sample_semantic_view:
    semantic_view: MY_DATABASE.MY_SCHEMA.SAMPLE_SM
    execution_environment:
      type: warehouse
      warehouse: COMPUTE_WH
      query_timeout: 60
  snowflake_docs_service:
    search_service: MY_DATABASE.MY_SCHEMA.DOCS_SERVICE
    max_results: 4
    id_column: SOURCE_URL
    title_column: DOCUMENT_TITLE
```

### Top-level Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Agent name |
| `comment` | No | Agent description |
| `deploy` | No | Deployment settings (database, schema, quote_identifiers, grants) |
| `eval` | No | Evaluation test cases with tool matching and/or custom commands (not sent to Snowflake API) |
| `profile` | No | Agent profile (`display_name`) |
| `models` | No | Model configuration (`orchestration`: model name) |
| `instructions` | No | Agent instructions |
| `orchestration` | No | Orchestration settings (`budget`) |
| `tools` | No | Tool definitions |
| `tool_resources` | No | Per-tool resource configuration |

### Instructions Fields

| Field | Description |
|-------|-------------|
| `response` | Instructions for how the agent should respond |
| `orchestration` | Instructions for the orchestration layer |
| `system` | System-level instructions |
| `sample_questions` | List of sample questions (each with a `question` field) |

### Tool Resources

`tool_resources` is a map keyed by tool name (matching `tool_spec.name`). Supported sub-fields depend on the tool type:

**`cortex_analyst_text_to_sql`:**

| Field | Description |
|-------|-------------|
| `semantic_view` | Fully qualified semantic view name (e.g., `DB.SCHEMA.VIEW`) |
| `execution_environment.type` | Environment type (e.g., `warehouse`) |
| `execution_environment.warehouse` | Warehouse name |
| `execution_environment.query_timeout` | Query timeout in seconds |

**`cortex_search`:**

| Field | Description |
|-------|-------------|
| `search_service` | Fully qualified search service name (e.g., `DB.SCHEMA.SERVICE`) |
| `max_results` | Maximum number of results to return |
| `id_column` | ID column name |
| `title_column` | Title column name |

## Grant Management

Grants can be managed declaratively via the `deploy.grant` section. The CLI computes the diff between the desired state (YAML) and the current state (Snowflake) and executes only the necessary `GRANT`/`REVOKE` statements.

### YAML Definition

```yaml
deploy:
  database: MY_DATABASE
  schema: MY_SCHEMA
  grant:
    account_roles:
      - role: ANALYST
        privileges:
          - ALL
      - role: DATA_VIEWER
        privileges:
          - USAGE
    database_roles:
      - role: MY_DATABASE.CORTEX_MONITOR
        privileges:
          - MONITOR
```

### Privileges

| Privilege | Description |
|-----------|-------------|
| `USAGE` | Allows using the agent |
| `MODIFY` | Allows modifying the agent |
| `MONITOR` | Allows monitoring the agent |
| `ALL` | Expands to `USAGE`, `MODIFY`, and `MONITOR` |

- `OWNERSHIP` is managed automatically by Snowflake and is ignored.
- Database roles must be fully qualified (e.g., `MY_DATABASE.ROLE_NAME`).

### Behavior

- On `plan`: Grant changes are shown as `+` (grant) and `-` (revoke) under the `grants:` section.
- On `apply`: `REVOKE` statements are executed first, then `GRANT` statements.
- If no `deploy.grant` section is defined, existing grants on the agent are not modified.

## CI/CD

- CI runs `go vet` and `go test`
- Integration tests require Key Pair authentication credentials
- Release workflow builds binaries and publishes to GitHub Releases

## Development

```bash
go test ./...
```
