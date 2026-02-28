# Auth Flow

Authentication is resolved at runtime when building the API client. Config is loaded from multiple sources and overridden by CLI flags.

## Config Resolution Order

`LoadConfig` (in `snowflake_config.go`) builds the base config, then `applyAuthOverrides` (in `plan.go`) overlays CLI flags:

1. **config.toml** — Base config from `LoadSnowflakeConnection(connectionName)` (lowest priority)
2. **Environment variables** — `SNOWFLAKE_ACCOUNT`, `SNOWFLAKE_USER`, `SNOWFLAKE_ROLE`, etc. (overlaid by `overlayEnv`)
3. **CLI flags** — `--account`, `--role`, `--database`, `--schema` (overlaid by `applyAuthOverrides`, highest priority)

The `--connection` flag selects which named connection to load from `~/.snowflake/config.toml`.

See [reference/config-priority.md](../../config-priority.md) for the full user-facing resolution order.

## Authenticator Types

| Type | Value | Token Source |
|------|-------|--------------|
| Key-pair (default) | `SNOWFLAKE_JWT` / `KEYPAIR` | JWT signed with private key |
| OAuth | `OAUTH_AUTHORIZATION_CODE` / `OAUTH` | Access token from token store |

## Key-Pair Flow

1. Load private key from env (`SNOWFLAKE_PRIVATE_KEY`) or file (config)
2. Build JWT with account, user, validity window
3. Sign with RSA; return as `KEYPAIR_JWT` bearer token

## OAuth Flow

### Login (`coragent login`)

1. Start local HTTP server on redirect URI (default `http://127.0.0.1:8080/callback`)
2. Open browser to Snowflake auth URL (or print URL if `--no-browser`)
3. User authenticates; Snowflake redirects with auth code
4. Exchange code for access + refresh tokens
5. Store tokens in `~/.coragent/tokens.json` (or platform-specific path)

### Runtime

1. `auth.BearerToken(ctx, cfg)` → `GetValidAccessToken`
2. If token expired and refresh token exists: refresh automatically
3. Return access token as `OAUTH` bearer

### Logout (`coragent logout`)

1. Load token store
2. Delete tokens for account (or `--all`)
3. Save store

## Key Files

- `internal/auth/auth.go` — `Config`, `BearerToken`, `AuthHeader`, JWT/OAuth dispatch
- `internal/auth/snowflake_config.go` — `LoadConfig`, `LoadSnowflakeConnection`, config.toml search
- `internal/auth/oauth.go` — OAuth token exchange, refresh
- `internal/auth/oauth_store.go` — Token persistence
- `internal/auth/oauth_server.go` — Local callback server for login
- `internal/cli/context.go` — `buildClient`, `buildClientAndCfg`
- `internal/cli/plan.go` — `applyAuthOverrides` (overlays CLI flags onto auth config)

## Related Docs

- [components/auth.md](../components/auth.md) — Config chain and authenticators
- [reference/config-priority.md](../../config-priority.md) — User-facing config reference
