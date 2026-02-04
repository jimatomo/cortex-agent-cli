package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultOAuthRedirectURI is the default redirect URI for local callback server.
	// Must use 127.0.0.1 (not localhost) per Snowflake requirements.
	DefaultOAuthRedirectURI = "http://127.0.0.1:8080"

	// DefaultOAuthClientID is the default client ID for Snowflake's built-in
	// LOCAL_APPLICATION security integration.
	DefaultOAuthClientID = "LOCAL_APPLICATION"

	// DefaultOAuthClientSecret is the default client secret for LOCAL_APPLICATION.
	DefaultOAuthClientSecret = "LOCAL_APPLICATION"
)

// OAuthConfig holds configuration for OAuth authentication.
type OAuthConfig struct {
	Account      string
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

// PKCEChallenge holds PKCE (Proof Key for Code Exchange) parameters.
type PKCEChallenge struct {
	CodeVerifier  string
	CodeChallenge string
}

// oauthTokenResponse represents the response from Snowflake's token endpoint.
type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"` // seconds
	Scope        string `json:"scope"`
}

// GeneratePKCE generates a PKCE code verifier and challenge.
// Uses SHA256 method as required by Snowflake.
func GeneratePKCE() (*PKCEChallenge, error) {
	// Generate 32 bytes of random data for code verifier
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return nil, fmt.Errorf("generate code verifier: %w", err)
	}

	// Base64 URL encode without padding
	codeVerifier := base64.RawURLEncoding.EncodeToString(verifierBytes)

	// Create SHA256 hash of the verifier
	hash := sha256.Sum256([]byte(codeVerifier))

	// Base64 URL encode the hash without padding
	codeChallenge := base64.RawURLEncoding.EncodeToString(hash[:])

	return &PKCEChallenge{
		CodeVerifier:  codeVerifier,
		CodeChallenge: codeChallenge,
	}, nil
}

// BuildAuthorizationURL constructs the Snowflake OAuth authorization URL.
// For local applications, PKCE is required.
func BuildAuthorizationURL(cfg OAuthConfig, state string, pkce *PKCEChallenge) (string, error) {
	if cfg.Account == "" {
		return "", fmt.Errorf("account is required")
	}

	clientID := cfg.ClientID
	if clientID == "" {
		clientID = DefaultOAuthClientID
	}

	redirectURI := cfg.RedirectURI
	if redirectURI == "" {
		redirectURI = DefaultOAuthRedirectURI
	}

	baseURL := fmt.Sprintf("https://%s.snowflakecomputing.com/oauth/authorize", cfg.Account)
	params := url.Values{
		"response_type": {"code"},
		"client_id":     {clientID},
		"redirect_uri":  {redirectURI},
	}

	if state != "" {
		params.Set("state", state)
	}

	// PKCE is required for local applications
	if pkce != nil {
		params.Set("code_challenge", pkce.CodeChallenge)
		params.Set("code_challenge_method", "S256")
	}

	return baseURL + "?" + params.Encode(), nil
}

// GenerateState generates a random state string for CSRF protection.
func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random state: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// ExchangeCodeForTokens exchanges an authorization code for OAuth tokens.
// For local applications using PKCE, the code_verifier must be provided.
func ExchangeCodeForTokens(ctx context.Context, cfg OAuthConfig, code string, codeVerifier string) (*OAuthTokens, error) {
	if cfg.Account == "" {
		return nil, fmt.Errorf("account is required")
	}
	if code == "" {
		return nil, fmt.Errorf("authorization code is required")
	}

	clientID := cfg.ClientID
	if clientID == "" {
		clientID = DefaultOAuthClientID
	}

	clientSecret := cfg.ClientSecret
	if clientSecret == "" {
		clientSecret = DefaultOAuthClientSecret
	}

	redirectURI := cfg.RedirectURI
	if redirectURI == "" {
		redirectURI = DefaultOAuthRedirectURI
	}

	tokenURL := fmt.Sprintf("https://%s.snowflakecomputing.com/oauth/token-request", cfg.Account)

	data := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {redirectURI},
	}

	// Include code_verifier for PKCE
	if codeVerifier != "" {
		data.Set("code_verifier", codeVerifier)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, clientSecret)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	var tokenResp oauthTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}

	tokens := &OAuthTokens{
		Account:      strings.ToUpper(cfg.Account),
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Scope:        tokenResp.Scope,
	}

	return tokens, nil
}

// RefreshAccessToken uses a refresh token to obtain a new access token.
func RefreshAccessToken(ctx context.Context, cfg OAuthConfig, refreshToken string) (*OAuthTokens, error) {
	if cfg.Account == "" {
		return nil, fmt.Errorf("account is required")
	}
	if refreshToken == "" {
		return nil, fmt.Errorf("refresh token is required")
	}

	clientID := cfg.ClientID
	if clientID == "" {
		clientID = DefaultOAuthClientID
	}

	clientSecret := cfg.ClientSecret
	if clientSecret == "" {
		clientSecret = DefaultOAuthClientSecret
	}

	redirectURI := cfg.RedirectURI
	if redirectURI == "" {
		redirectURI = DefaultOAuthRedirectURI
	}

	tokenURL := fmt.Sprintf("https://%s.snowflakecomputing.com/oauth/token-request", cfg.Account)

	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"redirect_uri":  {redirectURI},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create refresh request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, clientSecret)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh request failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	var tokenResp oauthTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("decode refresh response: %w", err)
	}

	tokens := &OAuthTokens{
		Account:      strings.ToUpper(cfg.Account),
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Scope:        tokenResp.Scope,
	}

	// If the refresh response doesn't include a new refresh token, keep the old one
	if tokens.RefreshToken == "" {
		tokens.RefreshToken = refreshToken
	}

	return tokens, nil
}

// GetValidAccessToken returns a valid access token, refreshing if necessary.
// It loads tokens from store, checks expiry, refreshes if needed, and saves updated tokens.
func GetValidAccessToken(ctx context.Context, cfg Config) (string, error) {
	store, err := LoadTokenStore()
	if err != nil {
		return "", fmt.Errorf("load token store: %w", err)
	}

	tokens := store.GetTokens(cfg.Account)
	if tokens == nil {
		return "", fmt.Errorf("no OAuth tokens found for account %s; run 'coragent login' first", cfg.Account)
	}

	// If token is still valid, return it
	if !tokens.IsExpired() {
		return tokens.AccessToken, nil
	}

	// Token expired, try to refresh
	if tokens.RefreshToken == "" {
		return "", fmt.Errorf("access token expired and no refresh token available; run 'coragent login' again")
	}

	// Use LOCAL_APPLICATION defaults for refresh
	oauthCfg := OAuthConfig{
		Account:     cfg.Account,
		RedirectURI: cfg.OAuthRedirectURI,
	}

	newTokens, err := RefreshAccessToken(ctx, oauthCfg, tokens.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("refresh access token: %w", err)
	}

	// Save refreshed tokens
	store.SetTokens(*newTokens)
	if err := store.Save(); err != nil {
		return "", fmt.Errorf("save refreshed tokens: %w", err)
	}

	return newTokens.AccessToken, nil
}
