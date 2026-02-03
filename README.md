# coragent (Cortex Agent CLI)

CLI tool for managing Snowflake Cortex Agent deployments via the REST API.

⚠️ This is an unofficial tool developed by a Snowflake user.

## Features

- Deploy Cortex Agents from YAML files
- Plan/Apply workflow with diff detection (PUT update only when changed)
- Validate YAML schema (unknown fields rejected)
- Export existing agents to YAML
- Run agents with streaming response and multi-turn conversation support
- Key Pair (RSA JWT) authentication

## Install

### Quick Install (Recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/jimatomo/cortex-agent-cli/main/install.sh | sh
```

Options:

```bash
# Install specific version
curl -fsSL https://raw.githubusercontent.com/jimatomo/cortex-agent-cli/main/install.sh | sh -s -- -v v0.1.0

# Install to custom directory (e.g., /usr/local/bin)
curl -fsSL https://raw.githubusercontent.com/jimatomo/cortex-agent-cli/main/install.sh | sh -s -- -d /usr/local/bin

# Force reinstall (even if same version exists)
curl -fsSL https://raw.githubusercontent.com/jimatomo/cortex-agent-cli/main/install.sh | sh -s -- --force
```

The script automatically:
- Detects your OS (macOS/Linux) and architecture (amd64/arm64)
- Downloads the latest release from GitHub
- Verifies checksum for security
- Installs to `~/.local/bin` by default

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

1. CLI flags: `--database`, `--schema`
2. YAML `deploy` section
3. Environment variables: `SNOWFLAKE_DATABASE`, `SNOWFLAKE_SCHEMA`

## Commands

| Command | Description |
|---------|-------------|
| `coragent plan [path]` | Show execution plan without applying (default: `.`) |
| `coragent apply [path]` | Apply changes to agents (default: `.`) |
| `coragent validate [path]` | Validate YAML files only (default: `.`) |
| `coragent export <agent-name>` | Export existing agent to YAML |
| `coragent run <agent-name>` | Run an agent with streaming response |
| `coragent threads` | Manage conversation threads |

## Global Flags

- `--account` / `-a`: Snowflake account identifier
- `--database` / `-d`: Target database
- `--schema` / `-s`: Target schema
- `--role` / `-r`: Snowflake role to use
- `--debug`: Enable debug logging with stack trace

## Plan/Apply Behavior

- **CREATE**: Agent does not exist → `POST`
- **UPDATE**: Agent exists with changes → `PUT` with changed top-level fields
- **NO_CHANGE**: Agent exists, no diff → Skip API call

## Export

```bash
# Output to stdout
coragent export my-agent

# Output to file
coragent export my-agent --out ./my-agent.yaml
```

## Run

Run an agent interactively with streaming response and conversation thread support.

### Basic Usage

```bash
# Run with interactive thread selection
coragent run my-agent -m "What are the top sales by region?"

# Start a new conversation thread
coragent run my-agent --new -m "Starting fresh topic"

# Continue a specific thread
coragent run my-agent --thread 12345 -m "Follow-up question"

# Single-turn mode (no thread tracking)
coragent run my-agent --without-thread -m "One-off question"
```

### Thread Support

Threads enable multi-turn conversations with context preservation. The CLI transparently manages threads via the Snowflake Cortex Threads API.

| Flag | Behavior |
|------|----------|
| (none) | Interactive selection from existing threads, or create new |
| `--new` | Create a new thread immediately |
| `--thread <id>` | Continue a specific thread by ID |
| `--without-thread` | Single-turn request without thread tracking |

Thread state is stored locally in `~/.coragent/threads.json` to track:
- Thread ID and last message ID (for `parent_message_id`)
- Last used timestamp
- Conversation summary

### Display Options

```bash
# Show agent's reasoning/thinking process
coragent run my-agent -m "Complex query" --show-thinking

# Show tool usage (SQL queries, search calls, etc.)
coragent run my-agent -m "Query data" --show-tools

# Combine with debug for full details
coragent run my-agent -m "Query" --show-tools --debug
```

### Run Flags

| Flag | Description |
|------|-------------|
| `-m, --message` | Message to send to the agent (required) |
| `--new` | Start a new conversation thread |
| `--thread <id>` | Continue a specific thread by ID |
| `--without-thread` | Run without thread support (single-turn) |
| `--show-thinking` | Display reasoning tokens on stderr |
| `--show-tools` | Display tool usage on stderr |

## Threads

Manage conversation threads across all agents.

### Usage

```bash
# Interactive mode - list threads and delete interactively
coragent threads

# List all threads (non-interactive)
coragent threads --list

# Delete a specific thread by ID
coragent threads --delete 29864464
```

### Interactive Mode

In interactive mode, you can view all threads and select which ones to delete:

```
Threads:
  [1] Thread 29864468 (2 hours ago) - "What are the top sales..."
      Agent: ACCOUNT/TEST_DB/PUBLIC/MY_AGENT
  [2] Thread 29864464 (1 day ago) - "Tell me about inventory..."
      Agent: ACCOUNT/TEST_DB/PUBLIC/OTHER_AGENT

  [d] Delete threads  [q] Quit
  Select: d
  Select threads to delete (space-separated, or 'all'): 1 2
  Delete 2 threads? [y/N]: y
  Deleted thread 29864468
  Deleted thread 29864464
```

### Threads Flags

| Flag | Description |
|------|-------------|
| `--list` | List all threads and exit (no API credentials required) |
| `--delete <id>` | Delete a specific thread by ID |

## CI/CD

- CI runs `go vet` and `go test`
- Integration tests require Key Pair authentication credentials
- Release workflow builds binaries and publishes to GitHub Releases

## Development

```bash
go test ./...
```
