package auth

import (
	"testing"
	"time"
)

func TestIsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{"1 hour from now", time.Now().Add(1 * time.Hour), false},
		{"61 seconds from now", time.Now().Add(61 * time.Second), false},
		{"30 seconds from now (within 60s buffer)", time.Now().Add(30 * time.Second), true},
		{"already expired", time.Now().Add(-1 * time.Minute), true},
		{"exactly 60 seconds from now (edge)", time.Now().Add(60 * time.Second), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &OAuthTokens{ExpiresAt: tt.expiresAt}
			got := token.IsExpired()
			if got != tt.want {
				t.Errorf("IsExpired() = %v, want %v (expiresAt=%v, now=%v)",
					got, tt.want, tt.expiresAt, time.Now())
			}
		})
	}
}

func TestNormalizeAccountKey(t *testing.T) {
	tests := []struct {
		name    string
		account string
		want    string
	}{
		{"uppercase", "MY_ACCOUNT", "MY_ACCOUNT"},
		{"lowercase", "my_account", "MY_ACCOUNT"},
		{"with whitespace", "  my_account  ", "MY_ACCOUNT"},
		{"empty", "", ""},
		{"mixed", "MyAccount", "MYACCOUNT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeAccountKey(tt.account)
			if got != tt.want {
				t.Errorf("normalizeAccountKey(%q) = %q, want %q", tt.account, got, tt.want)
			}
		})
	}
}

func TestTokenStore_SetAndGet(t *testing.T) {
	store := &TokenStore{Tokens: make(map[string]OAuthTokens)}

	tokens := OAuthTokens{
		Account:     "my_account",
		AccessToken: "access123",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	store.SetTokens(tokens)

	got := store.GetTokens("MY_ACCOUNT")
	if got == nil {
		t.Fatal("expected non-nil tokens")
	}
	if got.AccessToken != "access123" {
		t.Errorf("AccessToken = %q, want %q", got.AccessToken, "access123")
	}
	// Account should be normalized
	if got.Account != "MY_ACCOUNT" {
		t.Errorf("Account = %q, want %q", got.Account, "MY_ACCOUNT")
	}
}

func TestTokenStore_GetTokens_CaseInsensitive(t *testing.T) {
	store := &TokenStore{Tokens: make(map[string]OAuthTokens)}
	store.SetTokens(OAuthTokens{Account: "MY_ACCT", AccessToken: "tok1"})

	got := store.GetTokens("my_acct")
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got.AccessToken != "tok1" {
		t.Errorf("AccessToken = %q, want %q", got.AccessToken, "tok1")
	}
}

func TestTokenStore_GetTokens_NotFound(t *testing.T) {
	store := &TokenStore{Tokens: make(map[string]OAuthTokens)}
	got := store.GetTokens("NONEXISTENT")
	if got != nil {
		t.Errorf("expected nil for nonexistent account, got %+v", got)
	}
}

func TestTokenStore_DeleteTokens(t *testing.T) {
	store := &TokenStore{Tokens: make(map[string]OAuthTokens)}
	store.SetTokens(OAuthTokens{Account: "ACCT1", AccessToken: "tok1"})
	store.SetTokens(OAuthTokens{Account: "ACCT2", AccessToken: "tok2"})

	store.DeleteTokens("acct1")

	if got := store.GetTokens("ACCT1"); got != nil {
		t.Errorf("expected nil after delete, got %+v", got)
	}
	if got := store.GetTokens("ACCT2"); got == nil {
		t.Error("ACCT2 should not be affected")
	}
}

func TestTokenStore_ListAccounts(t *testing.T) {
	store := &TokenStore{Tokens: make(map[string]OAuthTokens)}
	store.SetTokens(OAuthTokens{Account: "ACCT1"})
	store.SetTokens(OAuthTokens{Account: "ACCT2"})

	accounts := store.ListAccounts()
	if len(accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(accounts))
	}

	found := map[string]bool{}
	for _, a := range accounts {
		found[a] = true
	}
	if !found["ACCT1"] || !found["ACCT2"] {
		t.Errorf("expected ACCT1 and ACCT2, got %v", accounts)
	}
}

func TestTokenStore_Clear(t *testing.T) {
	store := &TokenStore{Tokens: make(map[string]OAuthTokens)}
	store.SetTokens(OAuthTokens{Account: "ACCT1"})
	store.SetTokens(OAuthTokens{Account: "ACCT2"})

	store.Clear()

	if len(store.Tokens) != 0 {
		t.Errorf("expected 0 tokens after clear, got %d", len(store.Tokens))
	}
}

func TestTokenStore_SaveAndLoad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	store := &TokenStore{Tokens: make(map[string]OAuthTokens)}
	store.SetTokens(OAuthTokens{
		Account:      "ACCT1",
		AccessToken:  "access123",
		RefreshToken: "refresh456",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour).Truncate(time.Second),
		Scope:        "session:role:ANALYST",
	})

	if err := store.Save(); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	loaded, err := LoadTokenStore()
	if err != nil {
		t.Fatalf("LoadTokenStore error: %v", err)
	}

	got := loaded.GetTokens("ACCT1")
	if got == nil {
		t.Fatal("expected non-nil tokens after load")
	}
	if got.AccessToken != "access123" {
		t.Errorf("AccessToken = %q, want %q", got.AccessToken, "access123")
	}
	if got.RefreshToken != "refresh456" {
		t.Errorf("RefreshToken = %q, want %q", got.RefreshToken, "refresh456")
	}
	if got.Scope != "session:role:ANALYST" {
		t.Errorf("Scope = %q, want %q", got.Scope, "session:role:ANALYST")
	}
}

func TestLoadTokenStore_NoFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	store, err := LoadTokenStore()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil store")
	}
	if len(store.Tokens) != 0 {
		t.Errorf("expected empty tokens, got %d", len(store.Tokens))
	}
}
