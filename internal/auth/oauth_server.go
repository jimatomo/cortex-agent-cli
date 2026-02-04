package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	// DefaultCallbackPort is the default port for the OAuth callback server.
	DefaultCallbackPort = 8080
)

// CallbackResult holds the result from an OAuth callback.
type CallbackResult struct {
	Code  string
	State string
	Error string
}

// CallbackServer runs a temporary HTTP server to receive OAuth callbacks.
type CallbackServer struct {
	Port     int
	server   *http.Server
	listener net.Listener
	result   chan CallbackResult
	once     sync.Once
}

// NewCallbackServer creates a new callback server on the specified port.
func NewCallbackServer(port int) *CallbackServer {
	if port <= 0 {
		port = DefaultCallbackPort
	}
	return &CallbackServer{
		Port:   port,
		result: make(chan CallbackResult, 1),
	}
}

// Start starts the callback server.
func (s *CallbackServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleCallback)

	s.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	addr := fmt.Sprintf("127.0.0.1:%d", s.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	s.listener = listener

	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			s.once.Do(func() {
				s.result <- CallbackResult{Error: err.Error()}
			})
		}
	}()

	return nil
}

// WaitForCode waits for an OAuth callback and returns the authorization code.
func (s *CallbackServer) WaitForCode(ctx context.Context) (code, state string, err error) {
	select {
	case <-ctx.Done():
		return "", "", ctx.Err()
	case result := <-s.result:
		if result.Error != "" {
			return "", "", fmt.Errorf("callback error: %s", result.Error)
		}
		return result.Code, result.State, nil
	}
}

// Stop gracefully shuts down the callback server.
func (s *CallbackServer) Stop() error {
	if s.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return s.server.Shutdown(ctx)
}

// handleCallback handles the OAuth callback request.
func (s *CallbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Check for OAuth error response
	if errCode := query.Get("error"); errCode != "" {
		errDesc := query.Get("error_description")
		s.once.Do(func() {
			s.result <- CallbackResult{Error: fmt.Sprintf("%s: %s", errCode, errDesc)}
		})
		s.renderErrorPage(w, errCode, errDesc)
		return
	}

	code := query.Get("code")
	state := query.Get("state")

	if code == "" {
		s.once.Do(func() {
			s.result <- CallbackResult{Error: "no authorization code received"}
		})
		s.renderErrorPage(w, "missing_code", "No authorization code received")
		return
	}

	s.once.Do(func() {
		s.result <- CallbackResult{Code: code, State: state}
	})

	s.renderSuccessPage(w)
}

// renderSuccessPage renders a success page after successful authentication.
func (s *CallbackServer) renderSuccessPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
    <title>Authentication Successful</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
            margin: 0;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
        }
        .container {
            text-align: center;
            background: white;
            padding: 3rem;
            border-radius: 16px;
            box-shadow: 0 20px 60px rgba(0,0,0,0.3);
            max-width: 400px;
        }
        .checkmark {
            width: 80px;
            height: 80px;
            background: #10B981;
            border-radius: 50%;
            display: flex;
            justify-content: center;
            align-items: center;
            margin: 0 auto 1.5rem;
        }
        .checkmark svg {
            width: 40px;
            height: 40px;
            fill: white;
        }
        h1 { color: #1f2937; margin-bottom: 0.5rem; }
        p { color: #6b7280; margin-bottom: 1.5rem; }
        .hint { font-size: 0.875rem; color: #9ca3af; }
    </style>
</head>
<body>
    <div class="container">
        <div class="checkmark">
            <svg viewBox="0 0 24 24"><path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z"/></svg>
        </div>
        <h1>Authentication Successful!</h1>
        <p>You have been successfully authenticated with Snowflake.</p>
        <p class="hint">You can close this browser window and return to the terminal.</p>
    </div>
</body>
</html>`)
}

// renderErrorPage renders an error page when authentication fails.
func (s *CallbackServer) renderErrorPage(w http.ResponseWriter, errCode, errDesc string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>Authentication Failed</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
            margin: 0;
            background: linear-gradient(135deg, #ef4444 0%%, #dc2626 100%%);
        }
        .container {
            text-align: center;
            background: white;
            padding: 3rem;
            border-radius: 16px;
            box-shadow: 0 20px 60px rgba(0,0,0,0.3);
            max-width: 400px;
        }
        .error-icon {
            width: 80px;
            height: 80px;
            background: #EF4444;
            border-radius: 50%%;
            display: flex;
            justify-content: center;
            align-items: center;
            margin: 0 auto 1.5rem;
        }
        .error-icon svg {
            width: 40px;
            height: 40px;
            fill: white;
        }
        h1 { color: #1f2937; margin-bottom: 0.5rem; }
        p { color: #6b7280; margin-bottom: 1rem; }
        .error-code { font-family: monospace; background: #f3f4f6; padding: 0.5rem 1rem; border-radius: 4px; font-size: 0.875rem; }
        .hint { font-size: 0.875rem; color: #9ca3af; margin-top: 1.5rem; }
    </style>
</head>
<body>
    <div class="container">
        <div class="error-icon">
            <svg viewBox="0 0 24 24"><path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"/></svg>
        </div>
        <h1>Authentication Failed</h1>
        <p>%s</p>
        <div class="error-code">Error: %s</div>
        <p class="hint">Please close this window and try again.</p>
    </div>
</body>
</html>`, errDesc, errCode)
}

// GetRedirectURI returns the full redirect URI for this callback server.
// Uses 127.0.0.1 instead of localhost per Snowflake requirements.
func (s *CallbackServer) GetRedirectURI() string {
	return fmt.Sprintf("http://127.0.0.1:%d", s.Port)
}
