package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strings"
	"testing"
)

// generateTestPEM generates a PEM-encoded PKCS8 private key for testing.
func generateTestPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: der}
	return string(pem.EncodeToMemory(block))
}

// generateTestPKCS1PEM generates a PEM-encoded PKCS1 (RSA PRIVATE KEY) for testing.
func generateTestPKCS1PEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der := x509.MarshalPKCS1PrivateKey(key)
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}
	return string(pem.EncodeToMemory(block))
}

func TestNormalizePEM(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			"no indent",
			"-----BEGIN PRIVATE KEY-----\ndata\n-----END PRIVATE KEY-----",
			"-----BEGIN PRIVATE KEY-----\ndata\n-----END PRIVATE KEY-----",
		},
		{
			"space indent",
			"  -----BEGIN PRIVATE KEY-----\n  data\n  -----END PRIVATE KEY-----",
			"-----BEGIN PRIVATE KEY-----\ndata\n-----END PRIVATE KEY-----",
		},
		{
			"tab indent",
			"\t-----BEGIN PRIVATE KEY-----\n\tdata\n\t-----END PRIVATE KEY-----",
			"-----BEGIN PRIVATE KEY-----\ndata\n-----END PRIVATE KEY-----",
		},
		{
			"single line",
			"just one line",
			"just one line",
		},
		{
			"mixed indent uses minimum",
			"    -----BEGIN PRIVATE KEY-----\n      data\n    -----END PRIVATE KEY-----",
			"-----BEGIN PRIVATE KEY-----\n  data\n-----END PRIVATE KEY-----",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizePEM(tt.in)
			if got != tt.want {
				t.Errorf("normalizePEM() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadKeyPair_PKCS8(t *testing.T) {
	pemStr := generateTestPEM(t)
	priv, pub, err := loadKeyPair(pemStr, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if priv == nil {
		t.Error("expected non-nil private key")
	}
	if pub == nil {
		t.Error("expected non-nil public key")
	}
}

func TestLoadKeyPair_PKCS1(t *testing.T) {
	pemStr := generateTestPKCS1PEM(t)
	priv, pub, err := loadKeyPair(pemStr, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if priv == nil {
		t.Error("expected non-nil private key")
	}
	if pub == nil {
		t.Error("expected non-nil public key")
	}
}

func TestLoadKeyPair_Base64Encoded(t *testing.T) {
	pemStr := generateTestPEM(t)
	encoded := base64.StdEncoding.EncodeToString([]byte(pemStr))
	priv, pub, err := loadKeyPair(encoded, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if priv == nil || pub == nil {
		t.Error("expected non-nil keys")
	}
}

func TestLoadKeyPair_EscapedNewlines(t *testing.T) {
	pemStr := generateTestPEM(t)
	// Replace actual newlines with literal \n (common in env vars)
	escaped := strings.ReplaceAll(strings.TrimRight(pemStr, "\n"), "\n", "\\n")
	priv, pub, err := loadKeyPair(escaped, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if priv == nil || pub == nil {
		t.Error("expected non-nil keys")
	}
}

func TestNormalizePEM_IndentedPEM(t *testing.T) {
	// Test normalizePEM specifically with indented PEM (as from YAML)
	pemStr := "    -----BEGIN PRIVATE KEY-----\n    data\n    -----END PRIVATE KEY-----"
	got := normalizePEM(pemStr)
	want := "-----BEGIN PRIVATE KEY-----\ndata\n-----END PRIVATE KEY-----"
	if got != want {
		t.Errorf("normalizePEM(indented) = %q, want %q", got, want)
	}
}

func TestLoadKeyPair_Empty(t *testing.T) {
	_, _, err := loadKeyPair("", "")
	if err == nil {
		t.Error("expected error for empty key")
	}
}

func TestLoadKeyPair_InvalidPEM(t *testing.T) {
	_, _, err := loadKeyPair("not a valid PEM or base64", "")
	if err == nil {
		t.Error("expected error for invalid PEM")
	}
}

func TestLoadKeyPair_WhitespaceOnly(t *testing.T) {
	_, _, err := loadKeyPair("   \n  \t  ", "")
	if err == nil {
		t.Error("expected error for whitespace-only key")
	}
}

func TestBearerToken_UnsupportedAuthenticator(t *testing.T) {
	cfg := Config{
		Account:       "ACCT",
		User:          "USER",
		Authenticator: "UNKNOWN_METHOD",
	}
	_, _, err := BearerToken(context.Background(), cfg)
	if err == nil {
		t.Error("expected error for unsupported authenticator")
	}
	if !strings.Contains(err.Error(), "unsupported authenticator") {
		t.Errorf("error = %q, want to contain 'unsupported authenticator'", err.Error())
	}
}

func TestBearerToken_DefaultIsKeyPair(t *testing.T) {
	pemStr := generateTestPEM(t)
	cfg := Config{
		Account:       "ACCT",
		User:          "USER",
		PrivateKey:    pemStr,
		Authenticator: "", // should default to KEYPAIR
	}
	token, tokenType, err := BearerToken(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokenType != "KEYPAIR_JWT" {
		t.Errorf("tokenType = %q, want %q", tokenType, "KEYPAIR_JWT")
	}
	// JWT has 3 parts separated by dots
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Errorf("expected 3 JWT parts, got %d", len(parts))
	}
}

func TestKeyPairJWT_MissingConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{"missing account", Config{User: "U", PrivateKey: "key"}},
		{"missing user", Config{Account: "A", PrivateKey: "key"}},
		{"missing key", Config{Account: "A", User: "U"}},
		{"empty key", Config{Account: "A", User: "U", PrivateKey: "  "}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := keyPairJWT(tt.cfg)
			if err == nil {
				t.Error("expected error for missing config")
			}
		})
	}
}

func TestKeyPairJWT_Success(t *testing.T) {
	pemStr := generateTestPEM(t)
	cfg := Config{
		Account:    "MYACCOUNT",
		User:       "MYUSER",
		PrivateKey: pemStr,
	}
	token, err := keyPairJWT(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// JWT has 3 parts
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Errorf("expected 3 JWT parts, got %d", len(parts))
	}
}

func TestPublicKeyFingerprint(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	fp, err := publicKeyFingerprint(&key.PublicKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fp == "" {
		t.Error("expected non-empty fingerprint")
	}
	// Should be valid base64
	if _, err := base64.StdEncoding.DecodeString(fp); err != nil {
		t.Errorf("fingerprint is not valid base64: %v", err)
	}
}

func TestEnvOrDefault(t *testing.T) {
	tests := []struct {
		name     string
		envKey   string
		envValue string
		fallback string
		want     string
	}{
		{"env set", "TEST_ENV_VAR_1", "from_env", "default", "from_env"},
		{"env empty", "TEST_ENV_VAR_2", "", "default", "default"},
		{"env whitespace", "TEST_ENV_VAR_3", "  ", "default", "default"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.envKey, tt.envValue)
			got := envOrDefault(tt.envKey, tt.fallback)
			if got != tt.want {
				t.Errorf("envOrDefault(%q, %q) = %q, want %q", tt.envKey, tt.fallback, got, tt.want)
			}
		})
	}

	t.Run("env not set", func(t *testing.T) {
		got := envOrDefault("TOTALLY_UNSET_VAR_XYZ_12345", "fallback_val")
		if got != "fallback_val" {
			t.Errorf("envOrDefault for unset var = %q, want %q", got, "fallback_val")
		}
	})
}
