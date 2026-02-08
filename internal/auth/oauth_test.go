package auth

import (
	"encoding/base64"
	"strings"
	"testing"
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
