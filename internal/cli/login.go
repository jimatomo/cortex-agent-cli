package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"coragent/internal/auth"

	"github.com/spf13/cobra"
)

type loginOptions struct {
	account     string
	redirectURI string
	noBrowser   bool
	timeout     time.Duration
}

func newLoginCmd(opts *RootOptions) *cobra.Command {
	loginOpts := &loginOptions{}

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Snowflake using OAuth",
		Long: `Authenticate with Snowflake using OAuth Authorization Code Flow.

This command opens your browser to authenticate with Snowflake using the
built-in LOCAL_APPLICATION security integration. After successful authentication,
the OAuth tokens are stored locally for use with other commands.

Example:
  # Login to your Snowflake account
  coragent login --account myaccount

  # Login without opening browser (manual URL copy)
  coragent login --account myaccount --no-browser`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogin(cmd.Context(), opts, loginOpts)
		},
	}

	cmd.Flags().StringVarP(&loginOpts.account, "account", "a", "", "Snowflake account identifier (overrides global flag)")
	cmd.Flags().StringVar(&loginOpts.redirectURI, "redirect-uri", auth.DefaultOAuthRedirectURI, "OAuth redirect URI")
	cmd.Flags().BoolVar(&loginOpts.noBrowser, "no-browser", false, "Print URL instead of opening browser")
	cmd.Flags().DurationVar(&loginOpts.timeout, "timeout", 5*time.Minute, "Timeout waiting for authentication")

	return cmd
}

func runLogin(ctx context.Context, rootOpts *RootOptions, opts *loginOptions) error {
	// Determine account
	account := opts.account
	if account == "" {
		account = rootOpts.Account
	}
	if account == "" {
		account = os.Getenv("SNOWFLAKE_ACCOUNT")
	}
	if account == "" {
		cfg := auth.LoadConfig(rootOpts.Connection)
		account = cfg.Account
	}
	if account == "" {
		return fmt.Errorf("account is required; use --account flag or set SNOWFLAKE_ACCOUNT")
	}

	// Create callback server
	server := auth.NewCallbackServer(auth.DefaultCallbackPort)
	if err := server.Start(); err != nil {
		return fmt.Errorf("start callback server: %w", err)
	}
	defer server.Stop()

	// Generate state for CSRF protection
	state, err := auth.GenerateState()
	if err != nil {
		return fmt.Errorf("generate state: %w", err)
	}

	// Generate PKCE challenge (required for local applications)
	pkce, err := auth.GeneratePKCE()
	if err != nil {
		return fmt.Errorf("generate PKCE challenge: %w", err)
	}

	// Build authorization URL using LOCAL_APPLICATION defaults
	oauthCfg := auth.OAuthConfig{
		Account:     account,
		RedirectURI: opts.redirectURI,
		// ClientID and ClientSecret use LOCAL_APPLICATION defaults
	}

	authURL, err := auth.BuildAuthorizationURL(oauthCfg, state, pkce)
	if err != nil {
		return fmt.Errorf("build authorization URL: %w", err)
	}

	// Open browser or display URL
	if opts.noBrowser {
		fmt.Println("Open the following URL in your browser to authenticate:")
		fmt.Println()
		fmt.Println(authURL)
		fmt.Println()
		fmt.Println("Waiting for authentication...")
	} else {
		fmt.Println("Opening browser for authentication...")
		if err := openBrowser(authURL); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open browser: %v\n", err)
			fmt.Println()
			fmt.Println("Please open the following URL manually:")
			fmt.Println(authURL)
		}
		fmt.Println("Waiting for authentication...")
	}

	// Wait for callback
	ctx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()

	code, returnedState, err := server.WaitForCode(ctx)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Verify state
	if returnedState != state {
		return fmt.Errorf("invalid state returned; possible CSRF attack")
	}

	// Exchange code for tokens (include PKCE code verifier)
	fmt.Println("Exchanging authorization code for tokens...")
	tokens, err := auth.ExchangeCodeForTokens(ctx, oauthCfg, code, pkce.CodeVerifier)
	if err != nil {
		return fmt.Errorf("exchange code for tokens: %w", err)
	}

	// Save tokens
	store, err := auth.LoadTokenStore()
	if err != nil {
		return fmt.Errorf("load token store: %w", err)
	}

	store.SetTokens(*tokens)
	if err := store.Save(); err != nil {
		return fmt.Errorf("save tokens: %w", err)
	}

	fmt.Println()
	fmt.Printf("Successfully authenticated with account: %s\n", tokens.Account)
	fmt.Printf("Token expires: %s\n", tokens.ExpiresAt.Format(time.RFC3339))
	fmt.Println()
	fmt.Println("You can now use OAuth authentication by setting:")
	fmt.Println("  export SNOWFLAKE_AUTHENTICATOR=OAUTH")

	return nil
}

// openBrowser opens the specified URL in the default browser.
func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return cmd.Start()
}
