# Auth Component

The auth package handles configuration loading, JWT generation, and OAuth token management.

## Key Files

- `internal/auth/auth.go` — `Config`, `BearerToken`, `AuthHeader`, JWT/OAuth dispatch
- `internal/auth/snowflake_config.go` — `LoadConfig`, `LoadSnowflakeConnection`, `WriteConnection`, `DiagnoseConfig`
- `internal/auth/authenticator.go` — JWT signing, key loading
- `internal/auth/oauth.go` — OAuth token exchange, refresh, `GetValidAccessToken`
- `internal/auth/oauth_store.go` — Token persistence
- `internal/auth/oauth_server.go` — Local callback server for login flow
- `internal/auth/login.go` — Login URL construction (used by CLI)

## Config Structure

```go
type Config struct {
    Account              string
    User                 string
    Role                 string
    Warehouse            string
    Database             string
    Schema               string
    PrivateKey           string
    PrivateKeyPassphrase string
    Authenticator        string  // KEYPAIR or OAUTH
    OAuthRedirectURI     string
}
```

## Config Loading

- **LoadConfig(connectionName)** — Loads from config.toml; connection name selects section; empty → default connection
- **Search order:** `$SNOWFLAKE_HOME/config.toml`, `~/.snowflake/config.toml`, `~/.config/snowflake/config.toml` (Linux)
- CLI calls `applyAuthOverrides` to overlay `--account`, `--role`, `--database`, `--schema`

## Authenticators

Config-facing values (from `config.toml` or env) are mapped to internal constants by `mapAuthenticator` in `snowflake_config.go`:

| Config Value | Internal Constant | Token Type | Behavior |
|-------------|-------------------|------------|----------|
| `SNOWFLAKE_JWT` | `KEYPAIR` | KEYPAIR_JWT | Sign JWT with private key |
| `OAUTH_AUTHORIZATION_CODE` | `OAUTH` | OAUTH | Get/refresh access token from store |

When `Authenticator` is empty, it defaults to `KEYPAIR`.

## Token Store (OAuth)

- **Path:** `~/.coragent/tokens.json` (or platform-specific)
- **Operations:** `LoadTokenStore`, `GetTokens`, `Save`, `DeleteTokens`, `Clear`
- **Refresh:** `GetValidAccessToken` checks expiry; refreshes if needed before returning

## Related Docs

- [flows/auth-flow.md](../flows/auth-flow.md) — Login, logout, runtime flow
- [reference/config-priority.md](../../config-priority.md) — Resolution order
