package cli

import (
	"errors"
	"fmt"
	"strings"

	"coragent/internal/auth"

	"github.com/spf13/cobra"
)

func newAuthInitCmd(_ *RootOptions) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Interactively configure ~/.snowflake/config.toml",
		Long: `Interactively create or update a connection entry in ~/.snowflake/config.toml.

Existing connection entries are updated in-place; other entries and the
default_connection_name field are preserved.

Note: inline comments in the file are not preserved on rewrite.

Example:
  coragent auth init`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthInit(force)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing connection without confirmation")
	return cmd
}

func runAuthInit(force bool) error {
	fmt.Println("Snowflake Connection Setup")
	fmt.Println("==========================")
	fmt.Println()

	// --- Connection name ---
	connName, err := promptWithDefault("Connection name", "myconn")
	if err != nil {
		return err
	}

	// --- Confirm overwrite if connection already exists ---
	existing, _ := auth.LoadSnowflakeConnection(connName)
	if existing != nil && !force {
		ans, err := promptWithDefault(
			fmt.Sprintf("Connection %q already exists. Overwrite? [y/N]", connName), "N")
		if err != nil {
			return err
		}
		if strings.ToLower(strings.TrimSpace(ans)) != "y" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// --- Account ---
	accountDefault := ""
	if existing != nil {
		accountDefault = existing.Account
	}
	account, err := promptWithDefault("Account identifier (e.g., orgname-accountname)", accountDefault)
	if err != nil {
		return err
	}
	if account == "" {
		return UserErr(fmt.Errorf("account is required"))
	}

	// --- Authenticator ---
	fmt.Println()
	fmt.Println("  Authenticator:")
	fmt.Println("    1) SNOWFLAKE_JWT            (key-pair / service account)")
	fmt.Println("    2) OAUTH_AUTHORIZATION_CODE (browser OAuth)")
	authDefault := "1"
	if existing != nil && strings.EqualFold(existing.Authenticator, "OAUTH_AUTHORIZATION_CODE") {
		authDefault = "2"
	}
	authChoice, err := promptWithDefault("Choose [1/2]", authDefault)
	if err != nil {
		return err
	}

	var authenticator, user, privateKeyFile, oauthClientID, oauthClientSecret string

	fmt.Println()
	switch strings.TrimSpace(authChoice) {
	case "2":
		authenticator = "OAUTH_AUTHORIZATION_CODE"

		oauthClientIDDefault := ""
		if existing != nil {
			oauthClientIDDefault = existing.OAuthClientID
		}
		oauthClientID, err = promptWithDefault("OAuth client ID (optional)", oauthClientIDDefault)
		if err != nil {
			return err
		}

		oauthClientSecretDefault := ""
		if existing != nil {
			oauthClientSecretDefault = existing.OAuthClientSecret
		}
		oauthClientSecret, err = promptWithDefault("OAuth client secret (optional)", oauthClientSecretDefault)
		if err != nil {
			return err
		}

	default:
		authenticator = "SNOWFLAKE_JWT"

		userDefault := ""
		if existing != nil {
			userDefault = existing.User
		}
		user, err = promptWithDefault("User", userDefault)
		if err != nil {
			return err
		}

		pkDefault := ""
		if existing != nil {
			pkDefault = existing.PrivateKeyFile
			if pkDefault == "" {
				pkDefault = existing.PrivateKeyPath
			}
		}
		privateKeyFile, err = promptWithDefault("Private key file (e.g., ~/.ssh/snowflake_key.p8)", pkDefault)
		if err != nil {
			return err
		}
	}

	// --- Optional fields ---
	fmt.Println()

	roleDefault := ""
	if existing != nil {
		roleDefault = existing.Role
	}
	role, err := promptWithDefault("Role (optional)", roleDefault)
	if err != nil {
		return err
	}

	dbDefault := ""
	if existing != nil {
		dbDefault = existing.Database
	}
	database, err := promptWithDefault("Database (optional)", dbDefault)
	if err != nil {
		return err
	}

	schemaDefault := ""
	if existing != nil {
		schemaDefault = existing.Schema
	}
	schema, err := promptWithDefault("Schema (optional)", schemaDefault)
	if err != nil {
		return err
	}

	// --- Set as default connection ---
	fmt.Println()
	setDefaultAns, err := promptWithDefault("Set as default connection? [Y/n]", "Y")
	if err != nil {
		return err
	}
	isDefault := strings.ToLower(strings.TrimSpace(setDefaultAns)) != "n"

	// --- Write ---
	conn := auth.SnowflakeConnection{
		Account:           account,
		User:              user,
		Role:              role,
		Database:          database,
		Schema:            schema,
		Authenticator:     authenticator,
		PrivateKeyFile:    privateKeyFile,
		OAuthClientID:     oauthClientID,
		OAuthClientSecret: oauthClientSecret,
	}

	path, err := auth.WriteConnection(connName, conn, isDefault)
	if err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	fmt.Println()
	fmt.Printf("Wrote connection %q to %s\n", connName, path)
	if isDefault {
		fmt.Println("Set as default connection.")
	}
	fmt.Println()
	fmt.Println("Run 'coragent auth status' to verify the configuration.")
	return nil
}

// promptWithDefault displays a labeled prompt and reads a line of input.
// If the user presses Enter without typing anything, defaultVal is returned.
// Ctrl-C returns a UserErr cancellation.
func promptWithDefault(label, defaultVal string) (string, error) {
	var p string
	if defaultVal != "" {
		p = fmt.Sprintf("  %s [%s]: ", label, defaultVal)
	} else {
		p = fmt.Sprintf("  %s: ", label)
	}
	line, err := readLine(p)
	if err != nil {
		if errors.Is(err, errInterrupted) {
			return "", UserErr(fmt.Errorf("cancelled"))
		}
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultVal, nil
	}
	return line, nil
}
