package auth

import (
	"os"
	"path/filepath"
	"strings"
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

// --- DiagnoseConfig tests ---

func clearEnvForDiagnose(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"SNOWFLAKE_HOME", "SNOWFLAKE_DEFAULT_CONNECTION_NAME",
		"SNOWFLAKE_ACCOUNT", "SNOWFLAKE_USER",
	} {
		t.Setenv(key, "")
	}
}

func TestDiagnoseConfig_NoConfigFile(t *testing.T) {
	t.Setenv("SNOWFLAKE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	diag := DiagnoseConfig("")
	if diag.ConfigPath != "" {
		t.Errorf("ConfigPath = %q, want empty", diag.ConfigPath)
	}
	if len(diag.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(diag.Messages))
	}
	if diag.Messages[0].Level != DiagInfo {
		t.Errorf("level = %d, want DiagInfo", diag.Messages[0].Level)
	}
}

func TestDiagnoseConfig_TOMLSyntaxError(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, `this is not valid toml [[[`)
	t.Setenv("SNOWFLAKE_HOME", dir)

	diag := DiagnoseConfig("")
	if diag.ConfigPath == "" {
		t.Fatal("expected ConfigPath to be set")
	}
	if len(diag.Messages) == 0 {
		t.Fatal("expected error messages")
	}
	if diag.Messages[0].Level != DiagError {
		t.Errorf("level = %d, want DiagError", diag.Messages[0].Level)
	}
	if !containsStr(diag.Messages[0].Message, "TOML syntax error") {
		t.Errorf("message = %q, want to contain 'TOML syntax error'", diag.Messages[0].Message)
	}
}

func TestDiagnoseConfig_EmptyConnections(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, `default_connection_name = "dev"`)
	t.Setenv("SNOWFLAKE_HOME", dir)
	clearEnvForDiagnose(t)
	t.Setenv("SNOWFLAKE_HOME", dir)

	diag := DiagnoseConfig("")
	hasWarning := false
	for _, msg := range diag.Messages {
		if msg.Level == DiagWarning && containsStr(msg.Message, "No [connections] entries") {
			hasWarning = true
		}
	}
	if !hasWarning {
		t.Errorf("expected warning about empty connections, got %+v", diag.Messages)
	}
}

func TestDiagnoseConfig_ConnectionNotFound(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, `
[connections.dev]
account = "devaccount"
`)
	t.Setenv("SNOWFLAKE_HOME", dir)
	clearEnvForDiagnose(t)
	t.Setenv("SNOWFLAKE_HOME", dir)

	diag := DiagnoseConfig("production")
	hasError := false
	for _, msg := range diag.Messages {
		if msg.Level == DiagError && containsStr(msg.Message, `"production" not found`) {
			hasError = true
		}
	}
	if !hasError {
		t.Errorf("expected error about missing connection, got %+v", diag.Messages)
	}
	if len(diag.ConnectionNames) != 1 || diag.ConnectionNames[0] != "dev" {
		t.Errorf("ConnectionNames = %v, want [dev]", diag.ConnectionNames)
	}
}

func TestDiagnoseConfig_NoDefaultConnection(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, `
[connections.dev]
account = "devaccount"

[connections.staging]
account = "stagingaccount"
`)
	t.Setenv("SNOWFLAKE_HOME", dir)
	clearEnvForDiagnose(t)
	t.Setenv("SNOWFLAKE_HOME", dir)
	t.Setenv("SNOWFLAKE_DEFAULT_CONNECTION_NAME", "")

	diag := DiagnoseConfig("")
	hasWarning := false
	for _, msg := range diag.Messages {
		if msg.Level == DiagWarning && containsStr(msg.Message, "No default_connection_name") {
			hasWarning = true
		}
	}
	if !hasWarning {
		t.Errorf("expected warning about no default connection, got %+v", diag.Messages)
	}
}

func TestDiagnoseConfig_MissingRequiredFields(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, `
default_connection_name = "dev"

[connections.dev]
role = "myrole"
`)
	t.Setenv("SNOWFLAKE_HOME", dir)
	clearEnvForDiagnose(t)
	t.Setenv("SNOWFLAKE_HOME", dir)

	diag := DiagnoseConfig("")
	accountWarning := false
	userWarning := false
	for _, msg := range diag.Messages {
		if msg.Level == DiagWarning && containsStr(msg.Message, "missing 'account'") {
			accountWarning = true
		}
		if msg.Level == DiagWarning && containsStr(msg.Message, "missing 'user'") {
			userWarning = true
		}
	}
	if !accountWarning {
		t.Errorf("expected warning about missing account")
	}
	if !userWarning {
		t.Errorf("expected warning about missing user")
	}
}

func TestDiagnoseConfig_OAuthNoUserWarning(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, `
default_connection_name = "dev"

[connections.dev]
account = "myaccount"
authenticator = "OAUTH_AUTHORIZATION_CODE"
`)
	t.Setenv("SNOWFLAKE_HOME", dir)
	clearEnvForDiagnose(t)
	t.Setenv("SNOWFLAKE_HOME", dir)

	diag := DiagnoseConfig("")
	for _, msg := range diag.Messages {
		if msg.Level == DiagWarning && containsStr(msg.Message, "missing 'user'") {
			t.Errorf("unexpected user warning for OAuth connection: %s", msg.Message)
		}
	}
}

func TestDiagnoseConfig_UnknownAuthenticator(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, `
default_connection_name = "dev"

[connections.dev]
account = "myaccount"
user = "myuser"
authenticator = "INVALID_AUTH"
`)
	t.Setenv("SNOWFLAKE_HOME", dir)
	clearEnvForDiagnose(t)
	t.Setenv("SNOWFLAKE_HOME", dir)

	diag := DiagnoseConfig("")
	hasWarning := false
	for _, msg := range diag.Messages {
		if msg.Level == DiagWarning && containsStr(msg.Message, "Unknown authenticator") {
			hasWarning = true
		}
	}
	if !hasWarning {
		t.Errorf("expected warning about unknown authenticator, got %+v", diag.Messages)
	}
}

func TestDiagnoseConfig_PrivateKeyFileNotFound(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, `
default_connection_name = "dev"

[connections.dev]
account = "myaccount"
user = "myuser"
private_key_file = "/nonexistent/path/rsa_key.p8"
`)
	t.Setenv("SNOWFLAKE_HOME", dir)
	clearEnvForDiagnose(t)
	t.Setenv("SNOWFLAKE_HOME", dir)

	diag := DiagnoseConfig("")
	hasError := false
	for _, msg := range diag.Messages {
		if msg.Level == DiagError && containsStr(msg.Message, "Private key file not found") {
			hasError = true
		}
	}
	if !hasError {
		t.Errorf("expected error about missing private key file, got %+v", diag.Messages)
	}
}

func TestDiagnoseConfig_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "rsa_key.pem")
	if err := os.WriteFile(keyPath, []byte("key-content"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeConfigFile(t, dir, `
default_connection_name = "dev"

[connections.dev]
account = "myaccount"
user = "myuser"
authenticator = "SNOWFLAKE_JWT"
private_key_file = "`+keyPath+`"
`)
	t.Setenv("SNOWFLAKE_HOME", dir)
	clearEnvForDiagnose(t)
	t.Setenv("SNOWFLAKE_HOME", dir)

	diag := DiagnoseConfig("")
	if diag.ConfigPath == "" {
		t.Error("expected ConfigPath to be set")
	}
	if diag.ConnectionName != "dev" {
		t.Errorf("ConnectionName = %q, want 'dev'", diag.ConnectionName)
	}
	for _, msg := range diag.Messages {
		if msg.Level >= DiagWarning {
			t.Errorf("unexpected issue: [%d] %s", msg.Level, msg.Message)
		}
	}
}

func containsStr(s, substr string) bool {
	return strings.Contains(s, substr)
}

// --- WriteConnection tests ---

func TestWriteConnection_CreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SNOWFLAKE_HOME", dir)

	conn := SnowflakeConnection{
		Account:       "myorg-myaccount",
		User:          "myuser",
		Authenticator: "SNOWFLAKE_JWT",
		Role:          "myrole",
		Database:      "mydb",
		Schema:        "myschema",
	}
	path, err := WriteConnection("dev", conn, true)
	if err != nil {
		t.Fatal(err)
	}
	if path == "" {
		t.Fatal("expected non-empty path")
	}

	// File must be readable back
	loaded, err := LoadSnowflakeConnection("dev")
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil {
		t.Fatal("expected connection, got nil")
	}
	if loaded.Account != "myorg-myaccount" {
		t.Errorf("account = %q, want %q", loaded.Account, "myorg-myaccount")
	}
	if loaded.User != "myuser" {
		t.Errorf("user = %q, want %q", loaded.User, "myuser")
	}
	if loaded.Role != "myrole" {
		t.Errorf("role = %q, want %q", loaded.Role, "myrole")
	}
	if loaded.Database != "mydb" {
		t.Errorf("database = %q, want %q", loaded.Database, "mydb")
	}
	if loaded.Authenticator != "SNOWFLAKE_JWT" {
		t.Errorf("authenticator = %q, want %q", loaded.Authenticator, "SNOWFLAKE_JWT")
	}
}

func TestWriteConnection_SetsDefaultConnectionName(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SNOWFLAKE_HOME", dir)

	conn := SnowflakeConnection{Account: "myaccount", Authenticator: "SNOWFLAKE_JWT"}
	if _, err := WriteConnection("prod", conn, true); err != nil {
		t.Fatal(err)
	}

	// default_connection_name should make the connection loadable with empty name
	loaded, err := LoadSnowflakeConnection("")
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil {
		t.Fatal("expected connection via default name, got nil")
	}
	if loaded.Account != "myaccount" {
		t.Errorf("account = %q, want %q", loaded.Account, "myaccount")
	}
}

func TestWriteConnection_PreservesExistingConnections(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, `
default_connection_name = "dev"

[connections.dev]
account = "devaccount"
authenticator = "SNOWFLAKE_JWT"
`)
	t.Setenv("SNOWFLAKE_HOME", dir)

	// Add a second connection
	conn := SnowflakeConnection{Account: "prodaccount", Authenticator: "OAUTH_AUTHORIZATION_CODE"}
	if _, err := WriteConnection("prod", conn, false); err != nil {
		t.Fatal(err)
	}

	// Original connection must still be loadable
	dev, err := LoadSnowflakeConnection("dev")
	if err != nil {
		t.Fatal(err)
	}
	if dev == nil || dev.Account != "devaccount" {
		t.Errorf("dev connection missing or wrong account: %+v", dev)
	}

	// New connection must also be loadable
	prod, err := LoadSnowflakeConnection("prod")
	if err != nil {
		t.Fatal(err)
	}
	if prod == nil || prod.Account != "prodaccount" {
		t.Errorf("prod connection missing or wrong account: %+v", prod)
	}
}

func TestWriteConnection_UpdatesExistingConnection(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, `
default_connection_name = "dev"

[connections.dev]
account = "oldaccount"
authenticator = "SNOWFLAKE_JWT"
user = "olduser"
`)
	t.Setenv("SNOWFLAKE_HOME", dir)

	updated := SnowflakeConnection{Account: "newaccount", Authenticator: "SNOWFLAKE_JWT", User: "newuser"}
	if _, err := WriteConnection("dev", updated, true); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadSnowflakeConnection("dev")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Account != "newaccount" {
		t.Errorf("account = %q, want %q", loaded.Account, "newaccount")
	}
	if loaded.User != "newuser" {
		t.Errorf("user = %q, want %q", loaded.User, "newuser")
	}
}

func TestWriteConnection_OmitsEmptyFields(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SNOWFLAKE_HOME", dir)

	conn := SnowflakeConnection{
		Account:       "myaccount",
		Authenticator: "OAUTH_AUTHORIZATION_CODE",
		// User, Role, Database, Schema, PrivateKeyFile all empty
	}
	path, err := WriteConnection("myconn", conn, true)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// Empty fields must not appear
	for _, field := range []string{"user =", "role =", "database =", "schema =", "private_key_file ="} {
		if strings.Contains(content, field) {
			t.Errorf("file should not contain %q but got:\n%s", field, content)
		}
	}
}
