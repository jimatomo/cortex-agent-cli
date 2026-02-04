package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OAuthTokens holds OAuth tokens for a specific Snowflake account.
type OAuthTokens struct {
	Account      string    `json:"account"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope,omitempty"`
}

// IsExpired returns true if the access token has expired.
func (t *OAuthTokens) IsExpired() bool {
	// Consider expired if less than 60 seconds remaining
	return time.Now().Add(60 * time.Second).After(t.ExpiresAt)
}

// TokenStore manages OAuth tokens for multiple accounts.
type TokenStore struct {
	Tokens map[string]OAuthTokens `json:"tokens"` // Key: uppercase ACCOUNT name
}

// LoadTokenStore loads the OAuth token store from disk.
func LoadTokenStore() (*TokenStore, error) {
	path := oauthFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &TokenStore{Tokens: make(map[string]OAuthTokens)}, nil
		}
		return nil, err
	}

	var store TokenStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, err
	}
	if store.Tokens == nil {
		store.Tokens = make(map[string]OAuthTokens)
	}
	return &store, nil
}

// Save persists the token store to disk.
func (s *TokenStore) Save() error {
	path := oauthFilePath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// GetTokens returns the OAuth tokens for a specific account.
func (s *TokenStore) GetTokens(account string) *OAuthTokens {
	key := normalizeAccountKey(account)
	if tokens, ok := s.Tokens[key]; ok {
		return &tokens
	}
	return nil
}

// SetTokens stores OAuth tokens for a specific account.
func (s *TokenStore) SetTokens(tokens OAuthTokens) {
	key := normalizeAccountKey(tokens.Account)
	tokens.Account = key // Normalize the stored account name
	s.Tokens[key] = tokens
}

// DeleteTokens removes OAuth tokens for a specific account.
func (s *TokenStore) DeleteTokens(account string) {
	key := normalizeAccountKey(account)
	delete(s.Tokens, key)
}

// ListAccounts returns all accounts with stored tokens.
func (s *TokenStore) ListAccounts() []string {
	accounts := make([]string, 0, len(s.Tokens))
	for account := range s.Tokens {
		accounts = append(accounts, account)
	}
	return accounts
}

// Clear removes all stored tokens.
func (s *TokenStore) Clear() {
	s.Tokens = make(map[string]OAuthTokens)
}

// normalizeAccountKey normalizes the account name for use as a map key.
func normalizeAccountKey(account string) string {
	return strings.ToUpper(strings.TrimSpace(account))
}

// oauthFilePath returns the path to the OAuth token file.
func oauthFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".coragent", "oauth.json")
}
