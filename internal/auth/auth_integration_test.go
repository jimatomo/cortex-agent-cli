//go:build integration

package auth

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

// TestIntegration_KeyPairAuth tests JWT generation with key pair authentication.
// Requires: SNOWFLAKE_ACCOUNT, SNOWFLAKE_USER, and SNOWFLAKE_PRIVATE_KEY.
func TestIntegration_KeyPairAuth(t *testing.T) {
	cfg := Config{
		Account:              os.Getenv("SNOWFLAKE_ACCOUNT"),
		User:                 os.Getenv("SNOWFLAKE_USER"),
		PrivateKey:           os.Getenv("SNOWFLAKE_PRIVATE_KEY"),
		PrivateKeyPassphrase: os.Getenv("SNOWFLAKE_PRIVATE_KEY_PASSPHRASE"),
		Authenticator:        AuthenticatorKeyPair,
	}

	// Skip if key pair auth is not configured
	if cfg.Account == "" || cfg.User == "" || cfg.PrivateKey == "" {
		t.Skip("Key pair authentication not configured; skipping test")
	}

	ctx := context.Background()
	token, err := BearerToken(ctx, cfg)
	if err != nil {
		t.Fatalf("BearerToken: %v", err)
	}

	if token == "" {
		t.Fatal("BearerToken returned empty token")
	}

	// Verify the token is a valid JWT format
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Errorf("Token does not appear to be a valid JWT (expected 3 parts, got %d)", len(parts))
	}

	// Parse and validate JWT claims (without verifying signature, just structure)
	parser := jwt.NewParser()
	parsedToken, _, err := parser.ParseUnverified(token, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("Failed to parse JWT: %v", err)
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("Failed to extract claims from JWT")
	}

	// Verify issuer format: ACCOUNT.USER.SHA256:fingerprint
	issuer, ok := claims["iss"].(string)
	if !ok || issuer == "" {
		t.Error("JWT missing 'iss' claim")
	} else {
		if !strings.Contains(issuer, "SHA256:") {
			t.Errorf("JWT issuer should contain SHA256 fingerprint: %s", issuer)
		}
		expectedAccountUser := strings.ToUpper(cfg.Account) + "." + strings.ToUpper(cfg.User)
		if !strings.HasPrefix(issuer, expectedAccountUser) {
			t.Errorf("JWT issuer should start with ACCOUNT.USER: got %s, want prefix %s", issuer, expectedAccountUser)
		}
	}

	// Verify subject format: ACCOUNT.USER
	subject, ok := claims["sub"].(string)
	if !ok || subject == "" {
		t.Error("JWT missing 'sub' claim")
	} else {
		expectedSubject := strings.ToUpper(cfg.Account) + "." + strings.ToUpper(cfg.User)
		if subject != expectedSubject {
			t.Errorf("JWT subject mismatch: got %s, want %s", subject, expectedSubject)
		}
	}

	// Verify expiration is set
	if _, ok := claims["exp"]; !ok {
		t.Error("JWT missing 'exp' claim")
	}

	// Verify issued-at is set
	if _, ok := claims["iat"]; !ok {
		t.Error("JWT missing 'iat' claim")
	}

	t.Logf("Successfully generated JWT for %s", subject)
}

// TestIntegration_AuthHeader tests the full auth header generation.
func TestIntegration_AuthHeader(t *testing.T) {
	cfg := FromEnv()

	if cfg.Account == "" {
		t.Skip("SNOWFLAKE_ACCOUNT not set; skipping test")
	}

	// Key pair authentication requires user and private key
	if cfg.User == "" || cfg.PrivateKey == "" {
		t.Skip("Key pair authentication not fully configured; skipping test")
	}

	ctx := context.Background()
	header, err := AuthHeader(ctx, cfg)
	if err != nil {
		t.Fatalf("AuthHeader: %v", err)
	}

	if header == "" {
		t.Fatal("AuthHeader returned empty string")
	}

	if !strings.HasPrefix(header, "Bearer ") {
		t.Errorf("AuthHeader should start with 'Bearer ': got %s", header[:min(20, len(header))])
	}

	tokenPart := strings.TrimPrefix(header, "Bearer ")
	if tokenPart == "" {
		t.Error("AuthHeader token part is empty")
	}

	t.Logf("Successfully generated auth header (token length: %d)", len(tokenPart))
}

// TestIntegration_InvalidKeyPairAuth tests error handling for invalid key pair configuration.
func TestIntegration_InvalidKeyPairAuth(t *testing.T) {
	tests := []struct {
		name        string
		cfg         Config
		wantErr     bool
		errContains string
	}{
		{
			name: "missing account",
			cfg: Config{
				Account:       "",
				User:          "testuser",
				PrivateKey:    "-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----",
				Authenticator: AuthenticatorKeyPair,
			},
			wantErr:     true,
			errContains: "SNOWFLAKE_ACCOUNT",
		},
		{
			name: "missing user",
			cfg: Config{
				Account:       "testaccount",
				User:          "",
				PrivateKey:    "-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----",
				Authenticator: AuthenticatorKeyPair,
			},
			wantErr:     true,
			errContains: "SNOWFLAKE_USER",
		},
		{
			name: "missing private key",
			cfg: Config{
				Account:       "testaccount",
				User:          "testuser",
				PrivateKey:    "",
				Authenticator: AuthenticatorKeyPair,
			},
			wantErr:     true,
			errContains: "SNOWFLAKE_PRIVATE_KEY",
		},
		{
			name: "invalid private key format",
			cfg: Config{
				Account:       "testaccount",
				User:          "testuser",
				PrivateKey:    "not-a-valid-pem-key",
				Authenticator: AuthenticatorKeyPair,
			},
			wantErr:     true,
			errContains: "invalid SNOWFLAKE_PRIVATE_KEY",
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BearerToken(ctx, tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Error should contain %q: got %v", tt.errContains, err)
				}
			} else if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestIntegration_UnsupportedAuthenticator tests error for unknown authenticator type.
func TestIntegration_UnsupportedAuthenticator(t *testing.T) {
	cfg := Config{
		Account:       "testaccount",
		Authenticator: "UNSUPPORTED_AUTH",
	}

	ctx := context.Background()
	_, err := BearerToken(ctx, cfg)
	if err == nil {
		t.Error("Expected error for unsupported authenticator")
	}
	if !strings.Contains(err.Error(), "unsupported authenticator") {
		t.Errorf("Error should mention unsupported authenticator: %v", err)
	}
}

// TestIntegration_FromEnv tests configuration loading from environment.
func TestIntegration_FromEnv(t *testing.T) {
	// Save original env values
	origAccount := os.Getenv("SNOWFLAKE_ACCOUNT")
	origUser := os.Getenv("SNOWFLAKE_USER")
	origRole := os.Getenv("SNOWFLAKE_ROLE")

	// Set test values
	testAccount := "TEST_ACCOUNT_123"
	testUser := "TEST_USER_456"
	testRole := "TEST_ROLE"

	os.Setenv("SNOWFLAKE_ACCOUNT", testAccount)
	os.Setenv("SNOWFLAKE_USER", testUser)
	os.Setenv("SNOWFLAKE_ROLE", testRole)

	// Restore original values after test
	t.Cleanup(func() {
		if origAccount != "" {
			os.Setenv("SNOWFLAKE_ACCOUNT", origAccount)
		} else {
			os.Unsetenv("SNOWFLAKE_ACCOUNT")
		}
		if origUser != "" {
			os.Setenv("SNOWFLAKE_USER", origUser)
		} else {
			os.Unsetenv("SNOWFLAKE_USER")
		}
		if origRole != "" {
			os.Setenv("SNOWFLAKE_ROLE", origRole)
		} else {
			os.Unsetenv("SNOWFLAKE_ROLE")
		}
	})

	cfg := FromEnv()

	if cfg.Account != testAccount {
		t.Errorf("Account mismatch: got %q, want %q", cfg.Account, testAccount)
	}
	if cfg.User != testUser {
		t.Errorf("User mismatch: got %q, want %q", cfg.User, testUser)
	}
	if cfg.Role != testRole {
		t.Errorf("Role mismatch: got %q, want %q", cfg.Role, testRole)
	}
}
