package api

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
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
	log       *slog.Logger
}

// APIError represents a non-2xx HTTP response from the Snowflake API.
type APIError struct {
	StatusCode int
	Body       string
}

func (e APIError) Error() string {
	return fmt.Sprintf("api error: status=%d body=%s", e.StatusCode, e.Body)
}

// IsNotFoundError reports whether err indicates that a resource does not exist.
// It returns true for HTTP 404 responses and for Snowflake SQL errors that
// carry "does not exist" or "object not found" messages (including error code 002003).
//
// Most callers should prefer the (spec, exists, error) return pattern from
// GetAgent rather than inspecting errors directly; this function is exported
// for cases such as tests that receive raw errors from the API.
func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	if apiErr, ok := err.(APIError); ok {
		if apiErr.StatusCode == 404 {
			return true
		}
		// Snowflake SQL errors for non-existent objects
		bodyLower := strings.ToLower(apiErr.Body)
		if strings.Contains(bodyLower, "does not exist") ||
			strings.Contains(bodyLower, "object does not exist") ||
			strings.Contains(bodyLower, "agent") && strings.Contains(bodyLower, "not found") ||
			strings.Contains(bodyLower, "002003") { // Snowflake error code for object not found
			return true
		}
	}
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "does not exist") ||
		strings.Contains(errMsg, "not found")
}

// isNotFoundError is the internal alias used within the api package.
func isNotFoundError(err error) bool { return IsNotFoundError(err) }

// discardLogger returns a slog.Logger that discards all output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// NewClient constructs a Client using the given auth configuration.
func NewClient(cfg auth.Config) (*Client, error) {
	return NewClientWithDebug(cfg, false)
}

// NewClientForTest creates a Client pointing at the given base URL.
// Intended for use in tests against mock HTTP servers — no real Snowflake credentials required.
func NewClientForTest(base *url.URL, cfg auth.Config) *Client {
	return &Client{
		baseURL:   base,
		userAgent: "test",
		http:      &http.Client{Timeout: 30 * time.Second},
		authCfg:   cfg,
		log:       discardLogger(),
	}
}

// NewClientWithDebug constructs a Client with optional debug logging enabled.
// If debug is true, HTTP requests and responses are logged to stderr.
// If the environment variable CORAGENT_API_BASE_URL is set, it overrides the
// computed Snowflake endpoint — useful for testing against a mock HTTP server.
func NewClientWithDebug(cfg auth.Config, debug bool) (*Client, error) {
	if cfg.Account == "" {
		return nil, fmt.Errorf("SNOWFLAKE_ACCOUNT is required")
	}
	rawURL := fmt.Sprintf("https://%s.snowflakecomputing.com", cfg.Account)
	if override := os.Getenv("CORAGENT_API_BASE_URL"); override != "" {
		rawURL = override
	}
	base, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}

	var log *slog.Logger
	if debug {
		log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	} else {
		log = discardLogger()
	}

	client := &Client{
		baseURL:   base,
		role:      strings.ToUpper(strings.TrimSpace(cfg.Role)),
		userAgent: "coragent",
		http:      &http.Client{Timeout: 60 * time.Second},
		authCfg:   cfg,
		log:       log,
	}

	return client, nil
}
