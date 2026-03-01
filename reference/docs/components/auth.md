# Auth Component

The auth package handles configuration loading, JWT generation, and OAuth token management.

## Key Files

| File | Responsibility |
|------|----------------|
| `auth.go` | `Config`, `BearerToken`, `AuthHeader`, `keyPairJWT`, `loadKeyPair`, `parsePrivateKey`, `publicKeyFingerprint` |
| `snowflake_config.go` | `LoadConfig`, `LoadSnowflakeConnection`, `WriteConnection`, `DiagnoseConfig`, `overlayEnv`, `findConfigPath`, `ToAuthConfig`, `mapAuthenticator` |
| `authenticator.go` | `Authenticator` interface, `ConfigAuthenticator`, `NewAuthenticator` |
| `oauth.go` | `ExchangeCodeForTokens`, `RefreshAccessToken`, `GetValidAccessToken`, `BuildAuthorizationURL`, `GeneratePKCE`, `GenerateState` |
| `oauth_store.go` | `TokenStore`, `OAuthTokens`, `LoadTokenStore`, `Save`, `GetTokens`, `SetTokens`, `DeleteTokens`, `Clear` |
| `oauth_server.go` | `CallbackServer`, callback HTTP server, success/error HTML rendering |
| `login.go` | `Login`, `doLogin` — KEYPAIR session login (separate from OAuth) |

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

### findConfigPath Search Order

1. `$SNOWFLAKE_HOME/config.toml`
2. `~/.snowflake/config.toml`
3. Linux only: `~/.config/snowflake/config.toml`

Returns the first path that exists. Empty string if none exist.

### LoadSnowflakeConnection Connection Name Resolution

1. `connectionName` empty → `default_connection_name` (from config.toml)
2. Still empty → `SNOWFLAKE_DEFAULT_CONNECTION_NAME` env var
3. Both empty → returns `nil` (not an error)

### SnowflakeConnection → Config Conversion (ToAuthConfig)

- **Private key resolution order:** `private_key_file` → `private_key_path` → `private_key_raw`
- `~` expanded via `expandHome()` to home directory
- Empty `oauth_redirect_uri` → `DefaultOAuthRedirectURI` (`http://127.0.0.1:8080`)
- Empty `authenticator` → `AuthenticatorKeyPair`

### overlayEnv Environment Variables

Overwrites: `SNOWFLAKE_ACCOUNT`, `SNOWFLAKE_USER`, `SNOWFLAKE_ROLE`, `SNOWFLAKE_WAREHOUSE`, `SNOWFLAKE_DATABASE`, `SNOWFLAKE_SCHEMA`, `SNOWFLAKE_PRIVATE_KEY`, `SNOWFLAKE_PRIVATE_KEY_PASSPHRASE` (or `PRIVATE_KEY_PASSPHRASE`), `SNOWFLAKE_AUTHENTICATOR`, `SNOWFLAKE_OAUTH_REDIRECT_URI`. Only overwrites when non-empty.

## Authenticators

Mapping via `mapAuthenticator` (snowflake_config.go):

| Config Value | Internal Constant | Token Type | Behavior |
|-------------|-------------------|------------|----------|
| `SNOWFLAKE_JWT` | `KEYPAIR` | KEYPAIR_JWT | Sign JWT with private key |
| `OAUTH_AUTHORIZATION_CODE` | `OAUTH` | OAUTH | Get/refresh access token from store |
| Empty | `KEYPAIR` | — | Default |
| Other | Uppercased as-is | — | BearerToken returns error if unsupported |

## Private Key Loading (loadKeyPair)

Supported input formats:

1. **Inline PEM:** `-----BEGIN PRIVATE KEY-----` form; `\n` replaced with newline
2. **YAML indentation:** `normalizePEM()` strips leading common indent
3. **Base64-encoded PEM:** Decoded then parsed as PEM
4. **Base64-encoded DER:** PKCS#8 or PKCS#1 DER format

Supported PEM block types:

- `PRIVATE KEY` — PKCS#8
- `RSA PRIVATE KEY` — PKCS#1
- `ENCRYPTED PRIVATE KEY` — PKCS#8 encrypted (decrypted via `pkcs8.ParsePKCS8PrivateKey`)
- Legacy `x509.IsEncryptedPEMBlock` — decrypted via `x509.DecryptPEMBlock`

## Token Store (OAuth)

- **Path:** `~/.coragent/oauth.json` (`oauthFilePath()`)
- **Format:** JSON. `{"tokens": {"ACCOUNT_NAME": {...}}}`
- **Account key:** `normalizeAccountKey()` — uppercase and trim
- **Expiry check:** `IsExpired()` — expired when less than 60 seconds remaining
- **Rationale:** 60 seconds is an intentionally short safety buffer for access-token rollover. It is not the refresh-token lifetime and exists to reduce near-expiry request failures.
- **Operations:** `LoadTokenStore`, `GetTokens`, `SetTokens`, `DeleteTokens`, `Clear`, `Save`
- **File permissions:** Directory `0700`, file `0600`

## OAuth Constants (oauth.go)

| Constant | Value |
|----------|-------|
| `DefaultOAuthRedirectURI` | `http://127.0.0.1:8080` |
| `DefaultOAuthClientID` | `LOCAL_APPLICATION` |
| `DefaultOAuthClientSecret` | `LOCAL_APPLICATION` |

## CallbackServer (oauth_server.go)

- **Default port:** 8080
- **Bind address:** `127.0.0.1` (localhost not allowed — Snowflake requirement)
- **Handler:** Mounted at `/`. Parses `?code=...&state=...` or `?error=...&error_description=...`
- **CSRF:** Caller verifies state
- **Response:** HTML success page on success, HTML error page on failure

## Authenticator Interface (authenticator.go)

```go
type Authenticator interface {
    BearerToken(ctx context.Context) (token string, tokenType string, err error)
}
```

`ConfigAuthenticator` calls `auth.BearerToken(ctx, a.cfg)` internally. JWT is re-signed on each call, so short-lived tokens always return a valid token.

## Related Docs

- [flows/auth-flow.md](../flows/auth-flow.md) — Login, logout, runtime flow
- [reference/config-priority.md](../../config-priority.md) — Resolution order
