package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"
)

const (
	loginRequestPath = "/session/v1/login-request"
	clientAppID      = "coragent"
	clientAppVersion = "1.0.0"
)

// loginRequest represents the Snowflake login request body.
type loginRequest struct {
	Data loginRequestData `json:"data"`
}

type loginRequestData struct {
	ClientAppID       string            `json:"CLIENT_APP_ID"`
	ClientAppVersion  string            `json:"CLIENT_APP_VERSION"`
	AccountName       string            `json:"ACCOUNT_NAME"`
	LoginName         string            `json:"LOGIN_NAME,omitempty"`
	Authenticator     string            `json:"AUTHENTICATOR"`
	Token             string            `json:"TOKEN,omitempty"`
	Provider          string            `json:"PROVIDER,omitempty"`
	SessionParameters map[string]any    `json:"SESSION_PARAMETERS,omitempty"`
	ClientEnvironment map[string]string `json:"CLIENT_ENVIRONMENT,omitempty"`
}

// loginResponse represents the Snowflake login response.
type loginResponse struct {
	Data    loginResponseData `json:"data"`
	Message string            `json:"message"`
	Code    string            `json:"code"`
	Success bool              `json:"success"`
}

type loginResponseData struct {
	Token          string `json:"token"`
	MasterToken    string `json:"masterToken"`
	SessionID      int64  `json:"sessionId"`
	ValidityInSecs int64  `json:"validityInSeconds"`
}

// SessionToken holds the Snowflake session token and metadata.
type SessionToken struct {
	Token       string
	MasterToken string
	SessionID   int64
	ExpiresAt   time.Time
}

// Login authenticates with Snowflake and returns a session token.
func Login(ctx context.Context, cfg Config) (*SessionToken, error) {
	auth := strings.ToUpper(strings.TrimSpace(cfg.Authenticator))
	if auth == "" {
		auth = AuthenticatorKeyPair
	}

	var token string
	var provider string
	var err error

	switch auth {
	case AuthenticatorKeyPair:
		token, err = keyPairJWT(cfg)
		if err != nil {
			return nil, err
		}
	case AuthenticatorWorkloadIdentity:
		provider = strings.ToUpper(strings.TrimSpace(cfg.WorkloadIdentityProvider))
		if provider == "AWS" {
			token, err = getAWSWIFToken(ctx)
			if err != nil {
				return nil, err
			}
		} else {
			if cfg.OAuthToken == "" {
				return nil, fmt.Errorf("missing OAuth token for Workload Identity Federation")
			}
			token = cfg.OAuthToken
		}
	default:
		return nil, fmt.Errorf("unsupported authenticator: %s", cfg.Authenticator)
	}

	return doLogin(ctx, cfg, auth, token, provider)
}

// doLogin performs the actual login request to Snowflake.
func doLogin(ctx context.Context, cfg Config, authenticator, token, provider string) (*SessionToken, error) {
	// Build login URL
	baseURL := fmt.Sprintf("https://%s.snowflakecomputing.com", cfg.Account)
	loginURL, err := url.Parse(baseURL + loginRequestPath)
	if err != nil {
		return nil, fmt.Errorf("parse login URL: %w", err)
	}

	// Add query parameters
	query := loginURL.Query()
	if cfg.Role != "" {
		query.Set("roleName", cfg.Role)
	}
	if cfg.Warehouse != "" {
		query.Set("warehouse", cfg.Warehouse)
	}
	loginURL.RawQuery = query.Encode()

	// Build request body
	reqData := loginRequestData{
		ClientAppID:      clientAppID,
		ClientAppVersion: clientAppVersion,
		AccountName:      cfg.Account,
		Authenticator:    authenticator,
		Token:            token,
		ClientEnvironment: map[string]string{
			"OS":      runtime.GOOS,
			"ARCH":    runtime.GOARCH,
			"RUNTIME": runtime.Version(),
		},
	}

	if cfg.User != "" {
		reqData.LoginName = cfg.User
	}
	if provider != "" {
		reqData.Provider = provider
	}

	reqBody := loginRequest{Data: reqData}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal login request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create login request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", clientAppID)

	// Send request
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read login response: %w", err)
	}

	// Parse response
	var loginResp loginResponse
	if err := json.Unmarshal(respBody, &loginResp); err != nil {
		return nil, fmt.Errorf("parse login response: %w (body: %s)", err, truncate(string(respBody), 500))
	}

	if !loginResp.Success {
		return nil, fmt.Errorf("login failed: %s (code: %s)", loginResp.Message, loginResp.Code)
	}

	expiresAt := time.Now().Add(time.Duration(loginResp.Data.ValidityInSecs) * time.Second)

	return &SessionToken{
		Token:       loginResp.Data.Token,
		MasterToken: loginResp.Data.MasterToken,
		SessionID:   loginResp.Data.SessionID,
		ExpiresAt:   expiresAt,
	}, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
