package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"coragent/internal/auth"
)

// Client is the Snowflake Cortex Agent API client.
type Client struct {
	baseURL   *url.URL
	role      string
	userAgent string
	http      *http.Client
	authCfg   auth.Config
	debug     bool
}

// APIError represents a non-2xx HTTP response from the Snowflake API.
type APIError struct {
	StatusCode int
	Body       string
}

func (e APIError) Error() string {
	return fmt.Sprintf("api error: status=%d body=%s", e.StatusCode, e.Body)
}

// isNotFoundError checks if the error indicates a resource does not exist.
// This includes HTTP 404 errors and Snowflake SQL errors for non-existent objects.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	if apiErr, ok := err.(APIError); ok {
		if apiErr.StatusCode == 404 {
			return true
		}
		// Check for Snowflake SQL error messages indicating object does not exist
		bodyLower := strings.ToLower(apiErr.Body)
		if strings.Contains(bodyLower, "does not exist") ||
			strings.Contains(bodyLower, "object does not exist") ||
			strings.Contains(bodyLower, "agent") && strings.Contains(bodyLower, "not found") ||
			strings.Contains(bodyLower, "002003") { // Snowflake error code for object not found
			return true
		}
	}
	errMsg := strings.ToLower(err.Error())
	if strings.Contains(errMsg, "does not exist") ||
		strings.Contains(errMsg, "not found") {
		return true
	}
	return false
}

// NewClient constructs a Client using the given auth configuration.
func NewClient(cfg auth.Config) (*Client, error) {
	return NewClientWithDebug(cfg, false)
}

// NewClientWithDebug constructs a Client with optional debug logging enabled.
func NewClientWithDebug(cfg auth.Config, debug bool) (*Client, error) {
	if cfg.Account == "" {
		return nil, fmt.Errorf("SNOWFLAKE_ACCOUNT is required")
	}
	base, err := url.Parse(fmt.Sprintf("https://%s.snowflakecomputing.com", cfg.Account))
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}

	client := &Client{
		baseURL:   base,
		role:      strings.ToUpper(strings.TrimSpace(cfg.Role)),
		userAgent: "coragent",
		http:      &http.Client{Timeout: 60 * time.Second},
		authCfg:   cfg,
		debug:     debug,
	}

	return client, nil
}
