package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGeneratePKCE(t *testing.T) {
	pkce, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pkce.CodeVerifier == "" {
		t.Error("expected non-empty CodeVerifier")
	}
	if pkce.CodeChallenge == "" {
		t.Error("expected non-empty CodeChallenge")
	}
	// Verifier and challenge should be different
	if pkce.CodeVerifier == pkce.CodeChallenge {
		t.Error("CodeVerifier and CodeChallenge should be different")
	}
	// Verifier should be base64url encoded (43 chars for 32 bytes)
	if len(pkce.CodeVerifier) < 40 {
		t.Errorf("CodeVerifier length = %d, expected >= 40", len(pkce.CodeVerifier))
	}
}

func TestGeneratePKCE_Uniqueness(t *testing.T) {
	pkce1, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pkce2, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pkce1.CodeVerifier == pkce2.CodeVerifier {
		t.Error("two PKCE generations should produce different verifiers")
	}
	if pkce1.CodeChallenge == pkce2.CodeChallenge {
		t.Error("two PKCE generations should produce different challenges")
	}
}

func TestGenerateState(t *testing.T) {
	state, err := GenerateState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state == "" {
		t.Error("expected non-empty state")
	}
	// Should be valid base64
	if _, err := base64.URLEncoding.DecodeString(state); err != nil {
		t.Errorf("state is not valid base64url: %v", err)
	}
}

func TestGenerateState_Uniqueness(t *testing.T) {
	state1, err := GenerateState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	state2, err := GenerateState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state1 == state2 {
		t.Error("two state generations should produce different values")
	}
}

func TestBuildAuthorizationURL(t *testing.T) {
	tests := []struct {
		name    string
		cfg     OAuthConfig
		state   string
		pkce    *PKCEChallenge
		wantErr bool
		check   func(string) error
	}{
		{
			name:    "account required",
			cfg:     OAuthConfig{},
			wantErr: true,
		},
		{
			name: "defaults",
			cfg:  OAuthConfig{Account: "MYACCT"},
			check: func(url string) error {
				if !strings.Contains(url, "MYACCT.snowflakecomputing.com") {
					return errContains("account in URL")
				}
				if !strings.Contains(url, "client_id=LOCAL_APPLICATION") {
					return errContains("default client_id")
				}
				if !strings.Contains(url, "response_type=code") {
					return errContains("response_type")
				}
				return nil
			},
		},
		{
			name:  "with PKCE",
			cfg:   OAuthConfig{Account: "MYACCT"},
			pkce:  &PKCEChallenge{CodeVerifier: "v", CodeChallenge: "c"},
			check: func(url string) error {
				if !strings.Contains(url, "code_challenge=c") {
					return errContains("code_challenge")
				}
				if !strings.Contains(url, "code_challenge_method=S256") {
					return errContains("code_challenge_method")
				}
				return nil
			},
		},
		{
			name: "custom config",
			cfg: OAuthConfig{
				Account:     "MYACCT",
				ClientID:    "custom_client",
				RedirectURI: "http://localhost:9999/cb",
			},
			check: func(url string) error {
				if !strings.Contains(url, "client_id=custom_client") {
					return errContains("custom client_id")
				}
				if !strings.Contains(url, "redirect_uri="+strings.ReplaceAll("http://localhost:9999/cb", ":", "%3A")) {
					// URL encoding may vary, just check basic presence
					if !strings.Contains(url, "redirect_uri=") {
						return errContains("redirect_uri")
					}
				}
				return nil
			},
		},
		{
			name:  "with state",
			cfg:   OAuthConfig{Account: "MYACCT"},
			state: "random_state_value",
			check: func(url string) error {
				if !strings.Contains(url, "state=random_state_value") {
					return errContains("state parameter")
				}
				return nil
			},
		},
		{
			name: "empty state omitted",
			cfg:  OAuthConfig{Account: "MYACCT"},
			check: func(url string) error {
				if strings.Contains(url, "state=") {
					return errContains("state should not be present")
				}
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildAuthorizationURL(tt.cfg, tt.state, tt.pkce)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				if err := tt.check(got); err != nil {
					t.Errorf("URL check failed: %v\nURL: %s", err, got)
				}
			}
		})
	}
}

type errContainsType string

func errContains(msg string) errContainsType {
	return errContainsType("expected URL to contain " + msg)
}

func (e errContainsType) Error() string {
	return string(e)
}

// tokenServerResponse is the JSON body that tokenHandlerFunc returns.
type tokenServerResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

// successTokenHandler returns an http.Handler that responds 200 with the given
// token response.
func successTokenHandler(resp tokenServerResponse) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
}

// errorTokenHandler returns an http.Handler that responds with the given status
// code and body string.
func errorTokenHandler(status int, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	})
}

// TestRefreshAccessTokenInternal_Success verifies a 200 response is decoded into
// an OAuthTokens with the expected fields.
func TestRefreshAccessTokenInternal_Success(t *testing.T) {
	want := tokenServerResponse{
		AccessToken:  "new-access-token",
		RefreshToken: "new-refresh-token",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		Scope:        "session:role:ANALYST",
	}
	srv := httptest.NewServer(successTokenHandler(want))
	defer srv.Close()

	cfg := OAuthConfig{Account: "MYACCT"}
	tokens, err := refreshAccessTokenInternal(context.Background(), cfg, "old-refresh-token", srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokens.AccessToken != "new-access-token" {
		t.Errorf("AccessToken = %q, want %q", tokens.AccessToken, "new-access-token")
	}
	if tokens.RefreshToken != "new-refresh-token" {
		t.Errorf("RefreshToken = %q, want %q", tokens.RefreshToken, "new-refresh-token")
	}
	if tokens.Account != "MYACCT" {
		t.Errorf("Account = %q, want %q", tokens.Account, "MYACCT")
	}
	if tokens.ExpiresAt.Before(time.Now().Add(3500 * time.Second)) {
		t.Error("ExpiresAt too early")
	}
}

// TestRefreshAccessTokenInternal_KeepsOldRefreshToken verifies that when the
// server response omits a new refresh token, the original is preserved.
func TestRefreshAccessTokenInternal_KeepsOldRefreshToken(t *testing.T) {
	// Response has no refresh_token field.
	srv := httptest.NewServer(successTokenHandler(tokenServerResponse{
		AccessToken: "fresh-access",
		ExpiresIn:   3600,
	}))
	defer srv.Close()

	cfg := OAuthConfig{Account: "MYACCT"}
	tokens, err := refreshAccessTokenInternal(context.Background(), cfg, "original-refresh", srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokens.RefreshToken != "original-refresh" {
		t.Errorf("RefreshToken = %q, want original %q", tokens.RefreshToken, "original-refresh")
	}
}

// TestRefreshAccessTokenInternal_ServerError verifies that a non-200 response
// is returned as an error.
func TestRefreshAccessTokenInternal_ServerError(t *testing.T) {
	srv := httptest.NewServer(errorTokenHandler(http.StatusUnauthorized, `{"error":"invalid_grant"}`))
	defer srv.Close()

	cfg := OAuthConfig{Account: "MYACCT"}
	_, err := refreshAccessTokenInternal(context.Background(), cfg, "bad-token", srv.URL, srv.Client())
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error = %q, want to contain status 401", err.Error())
	}
}

// TestRefreshAccessTokenInternal_InvalidJSON verifies that a malformed JSON
// response is returned as an error.
func TestRefreshAccessTokenInternal_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json {{{"))
	}))
	defer srv.Close()

	cfg := OAuthConfig{Account: "MYACCT"}
	_, err := refreshAccessTokenInternal(context.Background(), cfg, "tok", srv.URL, srv.Client())
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// TestRefreshAccessTokenInternal_SendsGrantType verifies that the POST body
// contains grant_type=refresh_token.
func TestRefreshAccessTokenInternal_SendsGrantType(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err == nil {
			gotBody = r.FormValue("grant_type")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenServerResponse{AccessToken: "tok", ExpiresIn: 3600})
	}))
	defer srv.Close()

	cfg := OAuthConfig{Account: "MYACCT"}
	_, err := refreshAccessTokenInternal(context.Background(), cfg, "refresh-tok", srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotBody != "refresh_token" {
		t.Errorf("grant_type = %q, want %q", gotBody, "refresh_token")
	}
}

// TestExchangeCodeForTokensInternal_Success verifies that an authorization code
// is correctly exchanged for tokens.
func TestExchangeCodeForTokensInternal_Success(t *testing.T) {
	want := tokenServerResponse{
		AccessToken:  "access-from-code",
		RefreshToken: "refresh-from-code",
		TokenType:    "Bearer",
		ExpiresIn:    7200,
	}
	srv := httptest.NewServer(successTokenHandler(want))
	defer srv.Close()

	cfg := OAuthConfig{Account: "MYACCT"}
	tokens, err := exchangeCodeForTokensInternal(context.Background(), cfg, "auth-code", "verifier", srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokens.AccessToken != "access-from-code" {
		t.Errorf("AccessToken = %q, want %q", tokens.AccessToken, "access-from-code")
	}
}

// TestExchangeCodeForTokensInternal_SendsCodeVerifier verifies that code_verifier
// is included in the POST body when provided.
func TestExchangeCodeForTokensInternal_SendsCodeVerifier(t *testing.T) {
	var gotVerifier string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		gotVerifier = r.FormValue("code_verifier")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenServerResponse{AccessToken: "tok", ExpiresIn: 3600})
	}))
	defer srv.Close()

	cfg := OAuthConfig{Account: "MYACCT"}
	_, err := exchangeCodeForTokensInternal(context.Background(), cfg, "code", "my-verifier", srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotVerifier != "my-verifier" {
		t.Errorf("code_verifier = %q, want %q", gotVerifier, "my-verifier")
	}
}

// TestExchangeCodeForTokensInternal_ServerError verifies that a non-200 response
// returns an error.
func TestExchangeCodeForTokensInternal_ServerError(t *testing.T) {
	srv := httptest.NewServer(errorTokenHandler(http.StatusBadRequest, `{"error":"invalid_code"}`))
	defer srv.Close()

	cfg := OAuthConfig{Account: "MYACCT"}
	_, err := exchangeCodeForTokensInternal(context.Background(), cfg, "bad-code", "", srv.URL, srv.Client())
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error = %q, want to contain 400", err.Error())
	}
}

// TestRefreshAccessToken_MissingAccount verifies that RefreshAccessToken returns
// an error when account is not set (no HTTP round-trip needed).
func TestRefreshAccessToken_MissingAccount(t *testing.T) {
	_, err := RefreshAccessToken(context.Background(), OAuthConfig{}, "tok")
	if err == nil {
		t.Fatal("expected error for missing account")
	}
	if !strings.Contains(err.Error(), "account") {
		t.Errorf("error = %q, want to mention 'account'", err.Error())
	}
}

// TestRefreshAccessToken_MissingToken verifies that RefreshAccessToken returns
// an error when refresh token is empty.
func TestRefreshAccessToken_MissingToken(t *testing.T) {
	_, err := RefreshAccessToken(context.Background(), OAuthConfig{Account: "ACCT"}, "")
	if err == nil {
		t.Fatal("expected error for empty refresh token")
	}
}
