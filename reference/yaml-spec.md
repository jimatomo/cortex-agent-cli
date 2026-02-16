# YAML Specification Reference

Complete reference for the coragent YAML agent definition format.

## Full Example

```yaml
vars:
  default:
    SNOWFLAKE_DATABASE: DEV_DB
    SNOWFLAKE_SCHEMA: DEV
    WAREHOUSE: DEV_WH
  prod:
    SNOWFLAKE_DATABASE: PROD_DB
    SNOWFLAKE_SCHEMA: PUBLIC
    WAREHOUSE: COMPUTE_WH
  qa:
    SNOWFLAKE_SCHEMA: QA

deploy:
  database: ${ vars.SNOWFLAKE_DATABASE }
  schema: ${ vars.SNOWFLAKE_SCHEMA }
  quote_identifiers: true
  grant:
    account_roles:
      - role: ANALYST
        privileges:
          - ALL
    database_roles:
      - role: ${ vars.SNOWFLAKE_DATABASE }.CORTEX_MONITOR
        privileges:
          - MONITOR

eval:
  judge_model: claude-3-5-sonnet
  tests:
    - question: "Show me the sales data"
      expected_tools:
        - sample_semantic_view
    - question: "What was Q4 revenue?"
      expected_tools:
        - revenue_view
      expected_response: "Q4 revenue was approximately $120M"
    - question: "Search the Snowflake docs"
      expected_tools:
        - snowflake_docs_service
      command: "python eval_check.py"

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
    seconds: 300
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
    semantic_view: ${ vars.SNOWFLAKE_DATABASE }.${ vars.SNOWFLAKE_SCHEMA }.SAMPLE_SM
    execution_environment:
      type: warehouse
      warehouse: ${ vars.WAREHOUSE }
      query_timeout: 60
  snowflake_docs_service:
    search_service: ${ vars.SNOWFLAKE_DATABASE }.${ vars.SNOWFLAKE_SCHEMA }.DOCS_SERVICE
    max_results: 4
    id_column: SOURCE_URL
    title_column: DOCUMENT_TITLE
```

## Top-Level Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Agent name |
| `comment` | No | Human-readable description |
| `vars` | No | Variable substitution groups keyed by environment name |
| `deploy` | No | Deployment settings (database, schema, quote_identifiers, grant) |
| `eval` | No | Evaluation tests (not sent to the API) |
| `profile` | No | Profile settings (display_name) |
| `models` | No | Model configuration (orchestration) |
| `instructions` | No | Agent instructions |
| `orchestration` | No | Orchestration settings (budget) |
| `tools` | No | Tool definitions |
| `tool_resources` | No | Per-tool resource configuration |

## `vars` (Variable Substitution)

The `vars` section defines environment-specific variables that are substituted before the YAML is parsed.

### Structure

```yaml
vars:
  default:          # Fallback values (used when a key is missing in the selected env)
    KEY: value
  dev:              # Environment-specific overrides
    KEY: dev_value
  prod:
    KEY: prod_value
```

### Reference Syntax

Use `${ vars.KEY }` anywhere in a scalar value:

```yaml
deploy:
  database: ${ vars.DATABASE }                          # Full replacement
  schema: ${ vars.SCHEMA }
tool_resources:
  my_tool:
    semantic_view: ${ vars.DATABASE }.${ vars.SCHEMA }.MY_VIEW  # Partial / multiple refs
```

### Resolution Rules

1. If `--env <name>` is specified, values from that environment are used first.
2. Any keys missing in the selected environment fall back to `default`.
3. If `--env` is omitted, only `default` values are used.
4. If a referenced variable has no value in either the selected environment or `default`, an error is raised.
5. If `--env` specifies an unknown environment name, it falls back entirely to `default`.

### CLI Usage

```bash
coragent plan agent.yml                # uses vars.default only
coragent plan agent.yml --env dev      # uses vars.dev, fallback to vars.default
coragent apply agent.yml --env prod    # uses vars.prod, fallback to vars.default
```

## `instructions` Sub-Fields

| Field | Description |
|-------|-------------|
| `response` | Instructions for how the agent should respond |
| `orchestration` | Instructions for the orchestration layer |
| `system` | System-level instructions |
| `sample_questions` | Sample questions (each element has a `question` field) |

## `tool_resources` (by Tool Type)

### cortex_analyst_text_to_sql

| Field | Description |
|-------|-------------|
| `semantic_view` | Fully qualified name (DB.SCHEMA.VIEW) |
| `execution_environment.type` | Environment type (e.g., `warehouse`) |
| `execution_environment.warehouse` | Warehouse name |
| `execution_environment.query_timeout` | Query timeout in seconds |

### cortex_search

| Field | Description |
|-------|-------------|
| `search_service` | Fully qualified service name (DB.SCHEMA.SERVICE) |
| `max_results` | Maximum number of results to return |
| `id_column` | ID column name |
| `title_column` | Title column name |

## `eval.tests` Fields

| Field | Required | Description |
|-------|----------|-------------|
| `question` | No | Question to send to the agent. If omitted, the agent is not invoked |
| `expected_tools` | No | List of tool names expected in the response |
| `expected_response` | No | Expected response content (used by LLM-as-a-Judge) |
| `command` | No | Shell command to run for validation |

At least one of `expected_tools`, `expected_response`, or `command` is required per test.

## `deploy.grant` Privileges

| Privilege | Description |
|-----------|-------------|
| `USAGE` | Use the agent |
| `MODIFY` | Modify the agent |
| `MONITOR` | Monitor the agent |
| `ALL` | Expands to USAGE, MODIFY, and MONITOR |
