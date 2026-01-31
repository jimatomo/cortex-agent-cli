# coragent (Cortex Agent CLI)

CLI tool for managing Snowflake Cortex Agent deployments via the REST API.

⚠️ This is an unofficial tool developed by a Snowflake user.

## Features

- Deploy Cortex Agents from YAML files
- Plan/Apply workflow with diff detection (PUT update only when changed)
- Validate YAML schema (unknown fields rejected)
- Export existing agents to YAML
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

## CI/CD

- CI runs `go vet` and `go test`
- Integration tests require Key Pair authentication credentials
- Release workflow builds binaries and publishes to GitHub Releases

## Development

```bash
go test ./...
```
