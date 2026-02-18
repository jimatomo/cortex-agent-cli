# Configuration Priority Reference

## Resolution Order

coragent resolves configuration in the following priority order (lower number = higher priority):

1. **CLI Flags**
   `--database`, `--schema`, `--account`, `--role`, `--connection`, `--env`

2. **YAML `deploy` Section**
   Only `deploy.database` and `deploy.schema` (other auth-related settings cannot be specified in YAML).
   These values may themselves contain `${ vars.* }` or `${ env.* }` references that are resolved first.

3. **Environment Variables**
   - `SNOWFLAKE_ACCOUNT`
   - `SNOWFLAKE_USER`
   - `SNOWFLAKE_ROLE`
   - `SNOWFLAKE_DATABASE`
   - `SNOWFLAKE_SCHEMA`
   - `SNOWFLAKE_PRIVATE_KEY`
   - `SNOWFLAKE_PRIVATE_KEY_PASSPHRASE`
   - `SNOWFLAKE_AUTHENTICATOR`
   - etc.

4. **config.toml**
   `~/.snowflake/config.toml` (or `$SNOWFLAKE_HOME/config.toml`, etc.)

## Variable Substitution

Two syntaxes are supported and can be mixed freely:

| Syntax | Source |
|--------|--------|
| `${ vars.KEY }` | `vars:` section in the YAML file, selected via `--env` |
| `${ env.KEY }` | OS environment variable |

Both are resolved **before** any other YAML processing, so `deploy.database`, `tool_resources`, and all other fields can use either syntax.

### `vars` + `--env`

The `vars` section defines per-environment variable groups. The `--env` flag (or `-e`) selects which group to use.

Resolution order for `${ vars.KEY }`:
1. Selected environment (`vars.<env_name>`)
2. Fallback to `vars.default`
3. Error if variable is not found in either

```bash
coragent apply agent.yml --env prod    # vars.prod → vars.default fallback
coragent apply agent.yml               # vars.default only
```

### `env`

`${ env.KEY }` reads the value from the OS environment at the time the command runs. An error is raised if the variable is not set. No `vars:` section is needed.

```bash
export MY_DATABASE=PROD_DB
coragent apply agent.yml               # ${ env.MY_DATABASE } → PROD_DB
```

## config.toml Search Order

1. `$SNOWFLAKE_HOME/config.toml`
2. `~/.snowflake/config.toml`
3. `~/.config/snowflake/config.toml` (Linux only)

## Specifying a Connection

Use `--connection` (or `-c`) to select a named connection. When omitted, `default_connection_name` (or the `SNOWFLAKE_DEFAULT_CONNECTION_NAME` environment variable) is used.

## Project Settings (.coragent.toml)

Eval-related settings are configured in `.coragent.toml` (current directory) or `~/.coragent/config.toml`. CLI flags (e.g., `-o`) override these values.

| Setting | Description |
|---------|-------------|
| `eval.output_dir` | Output directory for eval reports |
| `eval.timestamp_suffix` | Append timestamp to output filenames |
| `eval.judge_model` | Model used for LLM-as-a-Judge |
| `eval.response_score_threshold` | Score threshold (0 to disable) |
