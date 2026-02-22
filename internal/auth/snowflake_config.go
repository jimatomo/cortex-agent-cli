package auth

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// DiagLevel indicates the severity of a diagnostic message.
type DiagLevel int

const (
	DiagInfo    DiagLevel = iota
	DiagWarning
	DiagError
)

// DiagMessage represents a single diagnostic finding.
type DiagMessage struct {
	Level   DiagLevel
	Message string
}

// ConfigDiagnostics contains the results of config file analysis.
type ConfigDiagnostics struct {
	ConfigPath      string
	ConnectionName  string
	ConnectionNames []string
	Messages        []DiagMessage
}

// SnowflakeConnection represents a [connections.<name>] section in config.toml.
type SnowflakeConnection struct {
	Account           string `toml:"account"`
	User              string `toml:"user"`
	Role              string `toml:"role"`
	Warehouse         string `toml:"warehouse"`
	Database          string `toml:"database"`
	Schema            string `toml:"schema"`
	Authenticator     string `toml:"authenticator"`
	PrivateKeyFile    string `toml:"private_key_file"`
	PrivateKeyPath    string `toml:"private_key_path"`
	PrivateKeyRaw     string `toml:"private_key_raw"`
	OAuthClientID     string `toml:"oauth_client_id"`
	OAuthClientSecret string `toml:"oauth_client_secret"`
	OAuthRedirectURI  string `toml:"oauth_redirect_uri"`
}

// snowflakeConfig represents the top-level structure of config.toml.
type snowflakeConfig struct {
	DefaultConnectionName string                         `toml:"default_connection_name"`
	Connections           map[string]SnowflakeConnection `toml:"connections"`
}

// findConfigPath locates the Snowflake CLI config.toml file.
// Search order:
//  1. $SNOWFLAKE_HOME/config.toml
//  2. ~/.snowflake/config.toml
//  3. OS-specific path (Linux: ~/.config/snowflake/config.toml)
func findConfigPath() string {
	candidates := []string{}

	if snowHome := os.Getenv("SNOWFLAKE_HOME"); snowHome != "" {
		candidates = append(candidates, filepath.Join(snowHome, "config.toml"))
	}

	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".snowflake", "config.toml"))
		if runtime.GOOS == "linux" {
			candidates = append(candidates, filepath.Join(home, ".config", "snowflake", "config.toml"))
		}
	}

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// LoadSnowflakeConnection reads the specified connection from config.toml.
// If connectionName is empty, the default_connection_name from config.toml is used.
// Returns nil if config.toml is not found or the connection doesn't exist.
func LoadSnowflakeConnection(connectionName string) (*SnowflakeConnection, error) {
	path := findConfigPath()
	if path == "" {
		return nil, nil
	}

	var cfg snowflakeConfig
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	if connectionName == "" {
		connectionName = cfg.DefaultConnectionName
	}
	if connectionName == "" {
		connectionName = os.Getenv("SNOWFLAKE_DEFAULT_CONNECTION_NAME")
	}
	if connectionName == "" {
		return nil, nil
	}

	conn, ok := cfg.Connections[connectionName]
	if !ok {
		return nil, fmt.Errorf("connection %q not found in %s", connectionName, path)
	}
	return &conn, nil
}

// ToAuthConfig converts a SnowflakeConnection to an auth.Config.
func (c *SnowflakeConnection) ToAuthConfig() (Config, error) {
	cfg := Config{
		Account:          c.Account,
		User:             c.User,
		Role:             c.Role,
		Warehouse:        c.Warehouse,
		Database:         c.Database,
		Schema:           c.Schema,
		Authenticator:    mapAuthenticator(c.Authenticator),
		OAuthRedirectURI: c.OAuthRedirectURI,
	}

	// Resolve private key: private_key_file → private_key_path → private_key_raw
	if keyFile := firstNonEmptyStr(c.PrivateKeyFile, c.PrivateKeyPath); keyFile != "" {
		expanded := expandHome(keyFile)
		data, err := os.ReadFile(expanded)
		if err != nil {
			return Config{}, fmt.Errorf("read private key file %s: %w", expanded, err)
		}
		cfg.PrivateKey = string(data)
	} else if c.PrivateKeyRaw != "" {
		cfg.PrivateKey = c.PrivateKeyRaw
	}

	// OAuth credentials are stored in the connection config and used at login/token time.

	if cfg.OAuthRedirectURI == "" {
		cfg.OAuthRedirectURI = DefaultOAuthRedirectURI
	}
	if cfg.Authenticator == "" {
		cfg.Authenticator = AuthenticatorKeyPair
	}

	return cfg, nil
}

// mapAuthenticator maps Snowflake CLI authenticator names to internal constants.
func mapAuthenticator(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "SNOWFLAKE_JWT":
		return AuthenticatorKeyPair
	case "OAUTH_AUTHORIZATION_CODE":
		return AuthenticatorOAuth
	case "":
		return AuthenticatorKeyPair
	default:
		return strings.ToUpper(strings.TrimSpace(s))
	}
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}

// firstNonEmptyStr returns the first non-empty string from the arguments.
func firstNonEmptyStr(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// LoadConfig loads auth configuration with the following priority:
//  1. Environment variables (high)
//  2. Snowflake CLI config.toml (low)
//
// CLI flags are applied separately via applyAuthOverrides.
func LoadConfig(connectionName string) Config {
	// Start from config.toml as base (errors silently ignored = file not found is OK)
	base := Config{Authenticator: AuthenticatorKeyPair}
	if conn, err := LoadSnowflakeConnection(connectionName); err == nil && conn != nil {
		base, _ = conn.ToAuthConfig()
	}

	// Overlay environment variables (only if set)
	overlayEnv(&base)

	return base
}

// DiagnoseConfig analyzes the config.toml file and returns diagnostic information.
// This is intended for the "auth status" command to help users identify configuration issues.
func DiagnoseConfig(connectionName string) ConfigDiagnostics {
	diag := ConfigDiagnostics{}

	// Find config file
	diag.ConfigPath = findConfigPath()
	if diag.ConfigPath == "" {
		diag.Messages = append(diag.Messages, DiagMessage{
			Level:   DiagInfo,
			Message: "No config.toml found. Using environment variables only.",
		})
		return diag
	}

	// Parse TOML
	var cfg snowflakeConfig
	if _, err := toml.DecodeFile(diag.ConfigPath, &cfg); err != nil {
		diag.Messages = append(diag.Messages, DiagMessage{
			Level:   DiagError,
			Message: fmt.Sprintf("TOML syntax error in %s: %v", diag.ConfigPath, err),
		})
		return diag
	}

	// Collect available connection names
	for name := range cfg.Connections {
		diag.ConnectionNames = append(diag.ConnectionNames, name)
	}
	sort.Strings(diag.ConnectionNames)

	// Check empty connections section
	if len(cfg.Connections) == 0 {
		diag.Messages = append(diag.Messages, DiagMessage{
			Level:   DiagWarning,
			Message: "No [connections] entries defined in config.toml.",
		})
		return diag
	}

	// Resolve connection name
	resolvedName := connectionName
	if resolvedName == "" {
		resolvedName = cfg.DefaultConnectionName
	}
	if resolvedName == "" {
		resolvedName = os.Getenv("SNOWFLAKE_DEFAULT_CONNECTION_NAME")
	}
	if resolvedName == "" {
		diag.Messages = append(diag.Messages, DiagMessage{
			Level:   DiagWarning,
			Message: fmt.Sprintf("No default_connection_name set and no --connection flag provided. Available connections: %s", strings.Join(diag.ConnectionNames, ", ")),
		})
		return diag
	}
	diag.ConnectionName = resolvedName

	// Check connection exists
	conn, ok := cfg.Connections[resolvedName]
	if !ok {
		diag.Messages = append(diag.Messages, DiagMessage{
			Level:   DiagError,
			Message: fmt.Sprintf("Connection %q not found in config.toml. Available: %s", resolvedName, strings.Join(diag.ConnectionNames, ", ")),
		})
		return diag
	}

	// Check required fields (only warn if env var also missing)
	if conn.Account == "" && os.Getenv("SNOWFLAKE_ACCOUNT") == "" {
		diag.Messages = append(diag.Messages, DiagMessage{
			Level:   DiagWarning,
			Message: fmt.Sprintf("Connection %q is missing 'account'. Set it in config.toml or via SNOWFLAKE_ACCOUNT.", resolvedName),
		})
	}
	// Check authenticator value
	authVal := strings.ToUpper(strings.TrimSpace(conn.Authenticator))
	isOAuth := authVal == "OAUTH_AUTHORIZATION_CODE"

	if conn.User == "" && os.Getenv("SNOWFLAKE_USER") == "" && !isOAuth {
		diag.Messages = append(diag.Messages, DiagMessage{
			Level:   DiagWarning,
			Message: fmt.Sprintf("Connection %q is missing 'user'. Set it in config.toml or via SNOWFLAKE_USER.", resolvedName),
		})
	}
	if authVal != "" && authVal != "SNOWFLAKE_JWT" && authVal != "OAUTH_AUTHORIZATION_CODE" {
		diag.Messages = append(diag.Messages, DiagMessage{
			Level:   DiagWarning,
			Message: fmt.Sprintf("Unknown authenticator %q. Expected: SNOWFLAKE_JWT or OAUTH_AUTHORIZATION_CODE.", conn.Authenticator),
		})
	}

	// Check private key file existence
	if keyFile := firstNonEmptyStr(conn.PrivateKeyFile, conn.PrivateKeyPath); keyFile != "" {
		expanded := expandHome(keyFile)
		if _, err := os.Stat(expanded); err != nil {
			diag.Messages = append(diag.Messages, DiagMessage{
				Level:   DiagError,
				Message: fmt.Sprintf("Private key file not found: %s", expanded),
			})
		}
	}

	return diag
}

// WriteConnection upserts a named connection entry in ~/.snowflake/config.toml.
// The file (and its parent directory) are created if they do not exist.
// If setAsDefault is true, default_connection_name is updated as well.
// Returns the path of the file that was written.
//
// Note: the file is rewritten in its entirety; inline comments are not preserved.
func WriteConnection(connName string, conn SnowflakeConnection, setAsDefault bool) (string, error) {
	// Determine target path: prefer $SNOWFLAKE_HOME when set (even if the file
	// doesn't exist yet), otherwise fall through to the first existing config or
	// the default ~/.snowflake location.
	var path string
	if snowHome := os.Getenv("SNOWFLAKE_HOME"); snowHome != "" {
		path = filepath.Join(snowHome, "config.toml")
	} else {
		path = findConfigPath()
		if path == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("get home dir: %w", err)
			}
			path = filepath.Join(home, ".snowflake", "config.toml")
		}
	}

	var cfg snowflakeConfig
	if _, err := os.Stat(path); err == nil {
		if _, err2 := toml.DecodeFile(path, &cfg); err2 != nil {
			return "", fmt.Errorf("parse %s: %w", path, err2)
		}
	}
	if cfg.Connections == nil {
		cfg.Connections = make(map[string]SnowflakeConnection)
	}
	cfg.Connections[connName] = conn
	if setAsDefault {
		cfg.DefaultConnectionName = connName
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}

	var sb strings.Builder
	if cfg.DefaultConnectionName != "" {
		fmt.Fprintf(&sb, "default_connection_name = %q\n", cfg.DefaultConnectionName)
	}

	names := make([]string, 0, len(cfg.Connections))
	for n := range cfg.Connections {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, n := range names {
		c := cfg.Connections[n]
		fmt.Fprintf(&sb, "\n[connections.%s]\n", n)
		writeTomlField(&sb, "account", c.Account)
		writeTomlField(&sb, "user", c.User)
		writeTomlField(&sb, "role", c.Role)
		writeTomlField(&sb, "warehouse", c.Warehouse)
		writeTomlField(&sb, "database", c.Database)
		writeTomlField(&sb, "schema", c.Schema)
		writeTomlField(&sb, "authenticator", c.Authenticator)
		writeTomlField(&sb, "private_key_file", c.PrivateKeyFile)
		writeTomlField(&sb, "private_key_path", c.PrivateKeyPath)
		writeTomlField(&sb, "private_key_raw", c.PrivateKeyRaw)
		writeTomlField(&sb, "oauth_client_id", c.OAuthClientID)
		writeTomlField(&sb, "oauth_client_secret", c.OAuthClientSecret)
		writeTomlField(&sb, "oauth_redirect_uri", c.OAuthRedirectURI)
	}

	if err := os.WriteFile(path, []byte(sb.String()), 0600); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

// writeTomlField appends key = "value" to sb only when value is non-empty.
func writeTomlField(sb *strings.Builder, key, value string) {
	if value != "" {
		fmt.Fprintf(sb, "%s = %q\n", key, value)
	}
}

// overlayEnv overrides base config fields with environment variables when set.
func overlayEnv(cfg *Config) {
	if v := os.Getenv("SNOWFLAKE_ACCOUNT"); v != "" {
		cfg.Account = v
	}
	if v := os.Getenv("SNOWFLAKE_USER"); v != "" {
		cfg.User = v
	}
	if v := os.Getenv("SNOWFLAKE_ROLE"); v != "" {
		cfg.Role = v
	}
	if v := os.Getenv("SNOWFLAKE_WAREHOUSE"); v != "" {
		cfg.Warehouse = v
	}
	if v := os.Getenv("SNOWFLAKE_DATABASE"); v != "" {
		cfg.Database = v
	}
	if v := os.Getenv("SNOWFLAKE_SCHEMA"); v != "" {
		cfg.Schema = v
	}
	if v := os.Getenv("SNOWFLAKE_PRIVATE_KEY"); v != "" {
		cfg.PrivateKey = v
	}
	if v := envOrDefault("SNOWFLAKE_PRIVATE_KEY_PASSPHRASE", os.Getenv("PRIVATE_KEY_PASSPHRASE")); v != "" {
		cfg.PrivateKeyPassphrase = v
	}
	if v := strings.TrimSpace(os.Getenv("SNOWFLAKE_AUTHENTICATOR")); v != "" {
		cfg.Authenticator = v
	}
	if v := strings.TrimSpace(os.Getenv("SNOWFLAKE_OAUTH_REDIRECT_URI")); v != "" {
		cfg.OAuthRedirectURI = v
	}
}
