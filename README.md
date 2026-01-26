# coragent

CLI tool for managing Snowflake Cortex Agent deployments via the REST API.

## Features

- Deploy Cortex Agents from YAML files
- Plan/Apply workflow with diff detection (PUT update only when changed)
- Validate YAML schema (unknown fields rejected)
- Export existing agents to YAML
- Supports Key Pair authentication and Workload Identity Federation (AWS IAM roles)

## Requirements

- Go 1.22+ (for building from source)
- Snowflake account with Cortex Agents enabled
- Role: `CORTEX_USER` or `CORTEX_AGENT_USER`

## Install

### From GitHub Releases

Download the appropriate binary from GitHub Releases and place it in your `$PATH`.

### From Source

```bash
git clone <your-repo-url>
cd coragent
go build -o coragent ./cmd/coragent
```

## Quick Start

### 1) Configure credentials

**Key Pair Authentication**

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

**Workload Identity Federation (AWS IAM Roles)**

> **Note:** Currently, only AWS IAM roles are supported for Workload Identity Federation.

```bash
export SNOWFLAKE_ACCOUNT=your_account
export SNOWFLAKE_ROLE=CORTEX_USER
export SNOWFLAKE_AUTHENTICATOR=WORKLOAD_IDENTITY
export SNOWFLAKE_WORKLOAD_IDENTITY_PROVIDER=AWS
# AWS credentials are automatically retrieved from the environment
# (IAM role, environment variables, EC2 instance profile, ECS task role, etc.)
```

For GitHub Actions with OIDC:

```yaml
- uses: aws-actions/configure-aws-credentials@v4
  with:
    role-to-assume: ${{ secrets.AWS_ROLE_ARN }}
    aws-region: your-region

- name: Deploy agents
  env:
    SNOWFLAKE_ACCOUNT: ${{ secrets.SNOWFLAKE_ACCOUNT }}
    SNOWFLAKE_ROLE: CORTEX_USER
    SNOWFLAKE_AUTHENTICATOR: WORKLOAD_IDENTITY
    SNOWFLAKE_WORKLOAD_IDENTITY_PROVIDER: AWS
  run: coragent apply
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
- Integration tests run when WIF secrets are configured
- Release workflow builds binaries and publishes to GitHub Releases

## Development

```bash
go test ./...
```
