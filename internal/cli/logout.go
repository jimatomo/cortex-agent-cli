package cli

import (
	"fmt"
	"os"
	"strings"

	"coragent/internal/auth"

	"github.com/spf13/cobra"
)

type logoutOptions struct {
	account string
	all     bool
}

func newLogoutCmd(opts *RootOptions) *cobra.Command {
	logoutOpts := &logoutOptions{}

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Remove stored OAuth tokens",
		Long: `Remove stored OAuth tokens for one or all Snowflake accounts.

Example:
  # Logout from specific account
  coragent logout --account myaccount

  # Logout from all accounts
  coragent logout --all`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogout(opts, logoutOpts)
		},
	}

	cmd.Flags().StringVarP(&logoutOpts.account, "account", "a", "", "Snowflake account to logout from (overrides global flag)")
	cmd.Flags().BoolVar(&logoutOpts.all, "all", false, "Logout from all accounts")

	return cmd
}

func runLogout(rootOpts *RootOptions, opts *logoutOptions) error {
	store, err := auth.LoadTokenStore()
	if err != nil {
		return fmt.Errorf("load token store: %w", err)
	}

	if opts.all {
		accounts := store.ListAccounts()
		if len(accounts) == 0 {
			fmt.Println("No stored OAuth tokens found.")
			return nil
		}

		store.Clear()
		if err := store.Save(); err != nil {
			return fmt.Errorf("save token store: %w", err)
		}

		fmt.Printf("Logged out from %d account(s):\n", len(accounts))
		for _, account := range accounts {
			fmt.Printf("  - %s\n", account)
		}
		return nil
	}

	// Determine account
	account := opts.account
	if account == "" {
		account = rootOpts.Account
	}
	if account == "" {
		account = os.Getenv("SNOWFLAKE_ACCOUNT")
	}
	if account == "" {
		return fmt.Errorf("account is required; use --account flag, set SNOWFLAKE_ACCOUNT, or use --all")
	}

	account = strings.ToUpper(strings.TrimSpace(account))

	tokens := store.GetTokens(account)
	if tokens == nil {
		fmt.Printf("No stored OAuth tokens found for account: %s\n", account)
		return nil
	}

	store.DeleteTokens(account)
	if err := store.Save(); err != nil {
		return fmt.Errorf("save token store: %w", err)
	}

	fmt.Printf("Logged out from account: %s\n", account)
	return nil
}
