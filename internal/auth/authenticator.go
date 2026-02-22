package auth

import "context"

// Authenticator provides authentication tokens for Snowflake API calls.
// Implementations must return a bearer token and its type for use in the
// Authorization header.
type Authenticator interface {
	// BearerToken returns a fresh bearer token and its type string.
	// Token types: "KEYPAIR_JWT" for key pair auth, "OAUTH" for OAuth.
	BearerToken(ctx context.Context) (token string, tokenType string, err error)
}

// ConfigAuthenticator is an Authenticator backed by a static auth.Config.
// It calls the package-level BearerToken function on each invocation so that
// short-lived JWT tokens are always freshly signed.
type ConfigAuthenticator struct {
	cfg Config
}

// NewAuthenticator returns an Authenticator for the given Config.
func NewAuthenticator(cfg Config) Authenticator {
	return &ConfigAuthenticator{cfg: cfg}
}

// BearerToken implements Authenticator.
func (a *ConfigAuthenticator) BearerToken(ctx context.Context) (string, string, error) {
	return BearerToken(ctx, a.cfg)
}
