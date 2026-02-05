package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfigFile(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadSnowflakeConnection_KeyPair(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, `
default_connection_name = "dev"

[connections.dev]
account = "myaccount"
user = "myuser"
role = "myrole"
warehouse = "mywh"
database = "mydb"
schema = "myschema"
authenticator = "SNOWFLAKE_JWT"
private_key_raw = "-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----"
`)
	t.Setenv("SNOWFLAKE_HOME", dir)

	conn, err := LoadSnowflakeConnection("")
	if err != nil {
		t.Fatal(err)
	}
	if conn == nil {
		t.Fatal("expected connection, got nil")
	}
	if conn.Account != "myaccount" {
		t.Errorf("account = %q, want %q", conn.Account, "myaccount")
	}
	if conn.User != "myuser" {
		t.Errorf("user = %q, want %q", conn.User, "myuser")
	}
	if conn.Role != "myrole" {
		t.Errorf("role = %q, want %q", conn.Role, "myrole")
	}
	if conn.Database != "mydb" {
		t.Errorf("database = %q, want %q", conn.Database, "mydb")
	}
	if conn.Schema != "myschema" {
		t.Errorf("schema = %q, want %q", conn.Schema, "myschema")
	}
	if conn.Authenticator != "SNOWFLAKE_JWT" {
		t.Errorf("authenticator = %q, want %q", conn.Authenticator, "SNOWFLAKE_JWT")
	}
}

func TestLoadSnowflakeConnection_OAuth(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, `
default_connection_name = "oauth"

[connections.oauth]
account = "oauthaccount"
user = "oauthuser"
authenticator = "OAUTH_AUTHORIZATION_CODE"
oauth_client_id = "myclient"
oauth_client_secret = "mysecret"
oauth_redirect_uri = "http://localhost:9090/callback"
`)
	t.Setenv("SNOWFLAKE_HOME", dir)

	conn, err := LoadSnowflakeConnection("")
	if err != nil {
		t.Fatal(err)
	}
	if conn == nil {
		t.Fatal("expected connection, got nil")
	}
	if conn.Authenticator != "OAUTH_AUTHORIZATION_CODE" {
		t.Errorf("authenticator = %q, want %q", conn.Authenticator, "OAUTH_AUTHORIZATION_CODE")
	}
	if conn.OAuthClientID != "myclient" {
		t.Errorf("oauth_client_id = %q, want %q", conn.OAuthClientID, "myclient")
	}
}

func TestLoadSnowflakeConnection_NamedConnection(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, `
default_connection_name = "default"

[connections.default]
account = "defaultaccount"

[connections.staging]
account = "stagingaccount"
`)
	t.Setenv("SNOWFLAKE_HOME", dir)

	conn, err := LoadSnowflakeConnection("staging")
	if err != nil {
		t.Fatal(err)
	}
	if conn == nil {
		t.Fatal("expected connection, got nil")
	}
	if conn.Account != "stagingaccount" {
		t.Errorf("account = %q, want %q", conn.Account, "stagingaccount")
	}
}

func TestLoadSnowflakeConnection_DefaultConnection(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, `
default_connection_name = "prod"

[connections.prod]
account = "prodaccount"
`)
	t.Setenv("SNOWFLAKE_HOME", dir)

	conn, err := LoadSnowflakeConnection("")
	if err != nil {
		t.Fatal(err)
	}
	if conn == nil {
		t.Fatal("expected connection, got nil")
	}
	if conn.Account != "prodaccount" {
		t.Errorf("account = %q, want %q", conn.Account, "prodaccount")
	}
}

func TestLoadSnowflakeConnection_MissingFile(t *testing.T) {
	t.Setenv("SNOWFLAKE_HOME", t.TempDir())
	// Clear HOME to prevent finding ~/.snowflake/config.toml
	t.Setenv("HOME", t.TempDir())

	conn, err := LoadSnowflakeConnection("")
	if err != nil {
		t.Fatal(err)
	}
	if conn != nil {
		t.Errorf("expected nil, got %+v", conn)
	}
}

func TestLoadSnowflakeConnection_ConnectionNotFound(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, `
[connections.dev]
account = "devaccount"
`)
	t.Setenv("SNOWFLAKE_HOME", dir)

	_, err := LoadSnowflakeConnection("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing connection")
	}
}

func TestToAuthConfig_AuthenticatorMapping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"SNOWFLAKE_JWT", AuthenticatorKeyPair},
		{"snowflake_jwt", AuthenticatorKeyPair},
		{"OAUTH_AUTHORIZATION_CODE", AuthenticatorOAuth},
		{"", AuthenticatorKeyPair},
	}

	for _, tc := range tests {
		conn := SnowflakeConnection{Authenticator: tc.input}
		cfg, err := conn.ToAuthConfig()
		if err != nil {
			t.Errorf("authenticator %q: unexpected error: %v", tc.input, err)
			continue
		}
		if cfg.Authenticator != tc.expected {
			t.Errorf("authenticator %q → %q, want %q", tc.input, cfg.Authenticator, tc.expected)
		}
	}
}

func TestToAuthConfig_PrivateKeyFile(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "rsa_key.pem")
	keyContent := "-----BEGIN PRIVATE KEY-----\ntestkey\n-----END PRIVATE KEY-----"
	if err := os.WriteFile(keyPath, []byte(keyContent), 0o600); err != nil {
		t.Fatal(err)
	}

	conn := SnowflakeConnection{
		Account:        "testaccount",
		PrivateKeyFile: keyPath,
	}
	cfg, err := conn.ToAuthConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PrivateKey != keyContent {
		t.Errorf("private key = %q, want %q", cfg.PrivateKey, keyContent)
	}
}

func TestToAuthConfig_PrivateKeyPathFallback(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "rsa_key.pem")
	keyContent := "-----BEGIN PRIVATE KEY-----\npathkey\n-----END PRIVATE KEY-----"
	if err := os.WriteFile(keyPath, []byte(keyContent), 0o600); err != nil {
		t.Fatal(err)
	}

	conn := SnowflakeConnection{
		Account:        "testaccount",
		PrivateKeyPath: keyPath,
	}
	cfg, err := conn.ToAuthConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PrivateKey != keyContent {
		t.Errorf("private key = %q, want %q", cfg.PrivateKey, keyContent)
	}
}

func TestToAuthConfig_PrivateKeyRaw(t *testing.T) {
	rawKey := "-----BEGIN PRIVATE KEY-----\nrawkey\n-----END PRIVATE KEY-----"
	conn := SnowflakeConnection{
		Account:       "testaccount",
		PrivateKeyRaw: rawKey,
	}
	cfg, err := conn.ToAuthConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PrivateKey != rawKey {
		t.Errorf("private key = %q, want %q", cfg.PrivateKey, rawKey)
	}
}

func TestToAuthConfig_PrivateKeyFilePriority(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "rsa_key.pem")
	fileContent := "file-key"
	if err := os.WriteFile(keyPath, []byte(fileContent), 0o600); err != nil {
		t.Fatal(err)
	}

	conn := SnowflakeConnection{
		Account:        "testaccount",
		PrivateKeyFile: keyPath,
		PrivateKeyRaw:  "raw-key",
	}
	cfg, err := conn.ToAuthConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PrivateKey != fileContent {
		t.Errorf("private key = %q, want %q (file should take priority)", cfg.PrivateKey, fileContent)
	}
}

func TestLoadConfig_EnvOverridesConfigToml(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, `
default_connection_name = "dev"

[connections.dev]
account = "toml-account"
user = "toml-user"
database = "toml-db"
schema = "toml-schema"
`)
	t.Setenv("SNOWFLAKE_HOME", dir)
	t.Setenv("SNOWFLAKE_ACCOUNT", "env-account")
	t.Setenv("SNOWFLAKE_USER", "")
	t.Setenv("SNOWFLAKE_DATABASE", "")
	t.Setenv("SNOWFLAKE_SCHEMA", "")
	t.Setenv("SNOWFLAKE_ROLE", "")
	t.Setenv("SNOWFLAKE_WAREHOUSE", "")
	t.Setenv("SNOWFLAKE_PRIVATE_KEY", "")
	t.Setenv("SNOWFLAKE_PRIVATE_KEY_PASSPHRASE", "")
	t.Setenv("SNOWFLAKE_AUTHENTICATOR", "")
	t.Setenv("SNOWFLAKE_OAUTH_REDIRECT_URI", "")

	cfg := LoadConfig("")
	if cfg.Account != "env-account" {
		t.Errorf("account = %q, want %q (env should override)", cfg.Account, "env-account")
	}
	if cfg.User != "toml-user" {
		t.Errorf("user = %q, want %q (toml should be used as fallback)", cfg.User, "toml-user")
	}
	if cfg.Database != "toml-db" {
		t.Errorf("database = %q, want %q", cfg.Database, "toml-db")
	}
	if cfg.Schema != "toml-schema" {
		t.Errorf("schema = %q, want %q", cfg.Schema, "toml-schema")
	}
}

func TestLoadConfig_NoConfigFile(t *testing.T) {
	t.Setenv("SNOWFLAKE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SNOWFLAKE_ACCOUNT", "env-only")
	t.Setenv("SNOWFLAKE_USER", "")
	t.Setenv("SNOWFLAKE_DATABASE", "")
	t.Setenv("SNOWFLAKE_SCHEMA", "")
	t.Setenv("SNOWFLAKE_ROLE", "")
	t.Setenv("SNOWFLAKE_WAREHOUSE", "")
	t.Setenv("SNOWFLAKE_PRIVATE_KEY", "")
	t.Setenv("SNOWFLAKE_PRIVATE_KEY_PASSPHRASE", "")
	t.Setenv("SNOWFLAKE_AUTHENTICATOR", "")
	t.Setenv("SNOWFLAKE_OAUTH_REDIRECT_URI", "")

	cfg := LoadConfig("")
	if cfg.Account != "env-only" {
		t.Errorf("account = %q, want %q", cfg.Account, "env-only")
	}
	if cfg.Authenticator != AuthenticatorKeyPair {
		t.Errorf("authenticator = %q, want %q", cfg.Authenticator, AuthenticatorKeyPair)
	}
}

func TestLoadConfig_DefaultConnectionFromEnv(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, `
[connections.envconn]
account = "envconn-account"
`)
	t.Setenv("SNOWFLAKE_HOME", dir)
	t.Setenv("SNOWFLAKE_DEFAULT_CONNECTION_NAME", "envconn")
	t.Setenv("SNOWFLAKE_ACCOUNT", "")
	t.Setenv("SNOWFLAKE_USER", "")
	t.Setenv("SNOWFLAKE_DATABASE", "")
	t.Setenv("SNOWFLAKE_SCHEMA", "")
	t.Setenv("SNOWFLAKE_ROLE", "")
	t.Setenv("SNOWFLAKE_WAREHOUSE", "")
	t.Setenv("SNOWFLAKE_PRIVATE_KEY", "")
	t.Setenv("SNOWFLAKE_PRIVATE_KEY_PASSPHRASE", "")
	t.Setenv("SNOWFLAKE_AUTHENTICATOR", "")
	t.Setenv("SNOWFLAKE_OAUTH_REDIRECT_URI", "")

	cfg := LoadConfig("")
	if cfg.Account != "envconn-account" {
		t.Errorf("account = %q, want %q", cfg.Account, "envconn-account")
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"~/some/path", filepath.Join(home, "some/path")},
	}

	for _, tc := range tests {
		got := expandHome(tc.input)
		if got != tc.expected {
			t.Errorf("expandHome(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestMapAuthenticator(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"SNOWFLAKE_JWT", AuthenticatorKeyPair},
		{"snowflake_jwt", AuthenticatorKeyPair},
		{"Snowflake_JWT", AuthenticatorKeyPair},
		{"OAUTH_AUTHORIZATION_CODE", AuthenticatorOAuth},
		{"oauth_authorization_code", AuthenticatorOAuth},
		{"", AuthenticatorKeyPair},
		{"  ", AuthenticatorKeyPair},
		{"UNKNOWN", "UNKNOWN"},
	}

	for _, tc := range tests {
		got := mapAuthenticator(tc.input)
		if got != tc.expected {
			t.Errorf("mapAuthenticator(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestToAuthConfig_OAuthFields(t *testing.T) {
	conn := SnowflakeConnection{
		Account:           "oauthaccount",
		Authenticator:     "OAUTH_AUTHORIZATION_CODE",
		OAuthClientID:     "myclient",
		OAuthClientSecret: "mysecret",
		OAuthRedirectURI:  "http://localhost:9090/callback",
	}
	cfg, err := conn.ToAuthConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Authenticator != AuthenticatorOAuth {
		t.Errorf("authenticator = %q, want %q", cfg.Authenticator, AuthenticatorOAuth)
	}
	if cfg.OAuthRedirectURI != "http://localhost:9090/callback" {
		t.Errorf("oauth redirect uri = %q, want %q", cfg.OAuthRedirectURI, "http://localhost:9090/callback")
	}
}

func TestToAuthConfig_DefaultOAuthRedirectURI(t *testing.T) {
	conn := SnowflakeConnection{
		Account:       "testaccount",
		Authenticator: "OAUTH_AUTHORIZATION_CODE",
	}
	cfg, err := conn.ToAuthConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OAuthRedirectURI != DefaultOAuthRedirectURI {
		t.Errorf("oauth redirect uri = %q, want default %q", cfg.OAuthRedirectURI, DefaultOAuthRedirectURI)
	}
}
