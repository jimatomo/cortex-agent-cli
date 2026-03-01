# Auth Flow

Authentication is resolved at runtime when building the API client. Config is loaded from multiple sources and overridden by CLI flags.

## Config Resolution Order

`LoadConfig` (in `snowflake_config.go`) builds the base config, then `applyAuthOverrides` (in `plan.go`) overlays CLI flags:

1. **config.toml** — Base config from `LoadSnowflakeConnection(connectionName)` (lowest priority)
2. **Environment variables** — `SNOWFLAKE_ACCOUNT`, `SNOWFLAKE_USER`, `SNOWFLAKE_ROLE`, etc. (overlaid by `overlayEnv`)
3. **CLI flags** — `--account`, `--role`, `--database`, `--schema` (overlaid by `applyAuthOverrides`, highest priority)

The `--connection` flag selects which named connection to load from `~/.snowflake/config.toml`.

See [reference/config-priority.md](../../config-priority.md) for the full user-facing resolution order.

### LoadConfig Internal Implementation

```go
// LoadConfig processing order (snowflake_config.go)
base := Config{Authenticator: AuthenticatorKeyPair}
if conn, err := LoadSnowflakeConnection(connectionName); err == nil && conn != nil {
    base, _ = conn.ToAuthConfig()
}
overlayEnv(&base)  // overlay with environment variables
return base
```

### applyAuthOverrides Internal Implementation

Defined in `internal/cli/plan.go`. Each field is overwritten only when non-empty:

- `opts.Account` → `cfg.Account` (uppercase normalized)
- `opts.Role` → `cfg.Role` (uppercase normalized)
- `opts.Database` → `cfg.Database`
- `opts.Schema` → `cfg.Schema`

## Authenticator Types

| Type | Value | Token Source |
|------|-------|--------------|
| Key-pair (default) | `SNOWFLAKE_JWT` / `KEYPAIR` | JWT signed with private key |
| OAuth | `OAUTH_AUTHORIZATION_CODE` / `OAUTH` | Access token from token store |

## Key-Pair Flow

### BearerToken Dispatch (auth.go)

`BearerToken(ctx, cfg)` branches on `cfg.Authenticator`:

1. `KEYPAIR` (or empty) → calls `keyPairJWT(cfg)`, returns `"KEYPAIR_JWT"`
2. `OAUTH` → calls `GetValidAccessToken(ctx, cfg)`, returns `"OAUTH"`
3. Otherwise → `fmt.Errorf("unsupported authenticator: %s", cfg.Authenticator)`

### JWT Generation Details (auth.go keyPairJWT)

1. **Required checks:** `Account`, `User`, `PrivateKey` must be non-empty
2. **Key loading:** `loadKeyPair(cfg.PrivateKey, cfg.PrivateKeyPassphrase)` loads RSA key pair
   - Supports inline PEM (`\n` escape), YAML indentation normalization, Base64-encoded PEM/DER
   - Supported formats: PKCS#8 (`PRIVATE KEY`), PKCS#1 (`RSA PRIVATE KEY`), encrypted PKCS#8 (`ENCRYPTED PRIVATE KEY`)
   - Passphrase: `SNOWFLAKE_PRIVATE_KEY_PASSPHRASE` or `PRIVATE_KEY_PASSPHRASE`
3. **Fingerprint:** `publicKeyFingerprint(publicKey)` — SHA256 Base64-encoded
4. **JWT claims:** `jwt.RegisteredClaims` with `Issuer`, `Subject`, `IssuedAt`, `ExpiresAt` (1 hour validity)
5. **Signing:** RS256 signature, returned as string

### API Request Usage

In `internal/api/http.go` `doJSON`, each request calls `auth.BearerToken(ctx, c.authCfg)` and sets:

- `Authorization: Bearer <token>`
- `X-Snowflake-Authorization-Token-Type: KEYPAIR_JWT` or `OAUTH`

## OAuth Flow

### Login (`coragent login`) — Implementation Flow (internal/cli/login.go)

1. **Account resolution:** `--account` → `rootOpts.Account` → `SNOWFLAKE_ACCOUNT` → `LoadConfig().Account` (in order)
2. **Callback server start:** `auth.NewCallbackServer(8080)` listens on `127.0.0.1:8080`
   - Path: mounted at `/` (handles all paths)
   - `GetRedirectURI()` returns `http://127.0.0.1:8080` (`DefaultOAuthRedirectURI`)
3. **CSRF protection:** `auth.GenerateState()` — 32-byte random, Base64 URL-encoded
4. **PKCE:** `auth.GeneratePKCE()` — code_verifier / code_challenge (S256)
5. **Authorization URL:** `auth.BuildAuthorizationURL(oauthCfg, state, pkce)`
   - Endpoint: `https://{account}.snowflakecomputing.com/oauth/authorize`
   - Params: `response_type=code`, `client_id`, `redirect_uri`, `state`, `code_challenge`, `code_challenge_method=S256`
   - Default client_id: `LOCAL_APPLICATION`
6. **Browser launch:** `openBrowser(authURL)` — macOS: `open`, Linux: `xdg-open`, Windows: `cmd /c start`
7. **Callback wait:** `server.WaitForCode(ctx)` receives `?code=...&state=...` or `?error=...`
8. **State verification:** mismatch treated as CSRF attack
9. **Token exchange:** `auth.ExchangeCodeForTokens(ctx, oauthCfg, code, pkce.CodeVerifier)`
   - Endpoint: `https://{account}.snowflakecomputing.com/oauth/token-request`
   - Basic Auth: `client_id:client_secret` (LOCAL_APPLICATION)
   - Body: `grant_type=authorization_code`, `code`, `redirect_uri`, `code_verifier`
10. **Token persistence:** `LoadTokenStore()` → `SetTokens()` → `Save()` to `~/.coragent/oauth.json`

### Runtime (GetValidAccessToken)

1. `LoadTokenStore()` reads `~/.coragent/oauth.json`
2. `store.GetTokens(cfg.Account)` fetches tokens for account (uppercase normalized)
3. If `tokens.IsExpired()` is false, return `tokens.AccessToken` as-is
   - Expiry: considered expired when less than 60 seconds remaining
   - Note: this 60-second threshold is an intentional safety margin for access tokens. It is separate from refresh-token validity (which is much longer).
4. If expired: `RefreshAccessToken(ctx, oauthCfg, tokens.RefreshToken)` to refresh
   - If refresh fails (e.g., invalid/revoked refresh token), the command returns an error that explicitly advises running `coragent login` again.
5. After refresh: `store.SetTokens(*newTokens)` → `store.Save()`, return new access token

### Logout (`coragent logout`)

1. `auth.LoadTokenStore()` loads store
2. `--all` → `store.Clear()`, otherwise `store.DeleteTokens(account)`
3. `store.Save()`

## Key Files

- `internal/auth/auth.go` — `Config`, `BearerToken`, `AuthHeader`, JWT/OAuth dispatch, `keyPairJWT`, `loadKeyPair`, `parsePrivateKey`
- `internal/auth/snowflake_config.go` — `LoadConfig`, `LoadSnowflakeConnection`, `overlayEnv`, `findConfigPath`, `ToAuthConfig`, `mapAuthenticator`
- `internal/auth/authenticator.go` — `Authenticator` interface, `ConfigAuthenticator`
- `internal/auth/oauth.go` — `ExchangeCodeForTokens`, `RefreshAccessToken`, `GetValidAccessToken`, `BuildAuthorizationURL`, `GeneratePKCE`, `GenerateState`
- `internal/auth/oauth_store.go` — `TokenStore`, `LoadTokenStore`, `Save`, `GetTokens`, `SetTokens`, `DeleteTokens`, `oauthFilePath` (`~/.coragent/oauth.json`)
- `internal/auth/oauth_server.go` — `CallbackServer`, `NewCallbackServer`, `Start`, `WaitForCode`, `Stop`, `handleCallback`
- `internal/auth/login.go` — `Login`, `doLogin` (KEYPAIR session login; separate from OAuth)
- `internal/cli/context.go` — `buildClient`, `buildClientAndCfg`
- `internal/cli/plan.go` — `applyAuthOverrides`
- `internal/cli/login.go` — `runLogin`, full OAuth login flow

## Related Docs

- [components/auth.md](../components/auth.md) — Config chain and authenticators
- [reference/config-priority.md](../../config-priority.md) — User-facing config reference
