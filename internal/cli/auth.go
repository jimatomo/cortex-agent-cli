package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"coragent/internal/auth"

	"github.com/spf13/cobra"
)

func newAuthCmd(opts *RootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication management commands",
		Long:  `Commands for managing authentication with Snowflake.`,
	}

	cmd.AddCommand(newAuthStatusCmd(opts))

	return cmd
}

type authStatusOptions struct {
	account string
}

func newAuthStatusCmd(opts *RootOptions) *cobra.Command {
	statusOpts := &authStatusOptions{}

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show authentication status",
		Long: `Display the current authentication method and token status.

Example:
  # Show status for current account
  coragent auth status

  # Show status for specific account
  coragent auth status --account myaccount`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthStatus(opts, statusOpts)
		},
	}

	cmd.Flags().StringVarP(&statusOpts.account, "account", "a", "", "Snowflake account to check (overrides global flag)")

	return cmd
}

func runAuthStatus(rootOpts *RootOptions, opts *authStatusOptions) error {
	// Determine account
	account := opts.account
	if account == "" {
		account = rootOpts.Account
	}
	if account == "" {
		account = os.Getenv("SNOWFLAKE_ACCOUNT")
	}

	// Determine authenticator
	authenticator := strings.ToUpper(strings.TrimSpace(os.Getenv("SNOWFLAKE_AUTHENTICATOR")))
	if authenticator == "" {
		authenticator = auth.AuthenticatorKeyPair
	}

	fmt.Println("Authentication Status")
	fmt.Println("=====================")
	fmt.Println()

	if account == "" {
		fmt.Println("Account: (not configured)")
		fmt.Println()
		fmt.Println("Set SNOWFLAKE_ACCOUNT or use --account flag")
		return nil
	}

	fmt.Printf("Account: %s\n", strings.ToUpper(account))
	fmt.Printf("Method:  %s\n", authenticator)
	fmt.Println()

	switch authenticator {
	case auth.AuthenticatorKeyPair:
		return showKeyPairStatus()
	case auth.AuthenticatorOAuth:
		return showOAuthStatus(account)
	default:
		fmt.Printf("Unknown authenticator: %s\n", authenticator)
	}

	return nil
}

func showKeyPairStatus() error {
	privateKey := os.Getenv("SNOWFLAKE_PRIVATE_KEY")
	user := os.Getenv("SNOWFLAKE_USER")

	if privateKey == "" {
		fmt.Println("Status:  Not configured")
		fmt.Println()
		fmt.Println("Missing: SNOWFLAKE_PRIVATE_KEY")
	} else if user == "" {
		fmt.Println("Status:  Not configured")
		fmt.Println()
		fmt.Println("Missing: SNOWFLAKE_USER")
	} else {
		fmt.Println("Status:  Configured")
		fmt.Printf("User:    %s\n", strings.ToUpper(user))
	}

	return nil
}

func showOAuthStatus(account string) error {
	store, err := auth.LoadTokenStore()
	if err != nil {
		return fmt.Errorf("load token store: %w", err)
	}

	tokens := store.GetTokens(account)
	if tokens == nil {
		fmt.Println("Status:  Not authenticated")
		fmt.Println()
		fmt.Println("Run 'coragent login' to authenticate with OAuth")
		return nil
	}

	now := time.Now()
	if tokens.IsExpired() {
		fmt.Println("Status:  Expired")
		fmt.Printf("Expired: %s (%s ago)\n",
			tokens.ExpiresAt.Format(time.RFC3339),
			formatDuration(now.Sub(tokens.ExpiresAt)))

		if tokens.RefreshToken != "" {
			fmt.Println()
			fmt.Println("A refresh token is available. The token will be")
			fmt.Println("automatically refreshed on next API call.")
		} else {
			fmt.Println()
			fmt.Println("No refresh token available. Run 'coragent login' again.")
		}
	} else {
		fmt.Println("Status:  Authenticated")
		fmt.Printf("Expires: %s (%s remaining)\n",
			tokens.ExpiresAt.Format(time.RFC3339),
			formatDuration(tokens.ExpiresAt.Sub(now)))

		if tokens.RefreshToken != "" {
			fmt.Println("Refresh: Available (automatic renewal)")
		}
	}

	if tokens.Scope != "" {
		fmt.Printf("Scope:   %s\n", tokens.Scope)
	}

	return nil
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}

	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		if minutes > 0 {
			return fmt.Sprintf("%dh %dm", hours, minutes)
		}
		return fmt.Sprintf("%d hours", hours)
	}

	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	if hours > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	return fmt.Sprintf("%d days", days)
}
