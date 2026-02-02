package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"coragent/internal/api"
	"coragent/internal/auth"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newRunCmd(opts *RootOptions) *cobra.Command {
	var message string
	var showThinking bool
	var showTools bool

	cmd := &cobra.Command{
		Use:   "run <agent-name>",
		Short: "Run an agent with a message",
		Long: `Run a Cortex Agent and stream the response in real-time.

The agent's response is streamed to stdout as it is generated.
Use --show-thinking to display reasoning tokens on stderr.
Use --show-tools to display tool usage on stderr.`,
		Example: `  # Basic usage
  coragent run my-agent -m "What are the top sales by region?"

  # With database/schema
  coragent run my-agent -d MY_DB -s MY_SCHEMA -m "Summarize Q4 results"

  # Show thinking/reasoning
  coragent run my-agent -m "Complex query" --show-thinking

  # Show tool usage
  coragent run my-agent -m "Query data" --show-tools`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			agentName := args[0]

			cfg := auth.FromEnv()
			applyAuthOverrides(&cfg, opts)

			client, err := api.NewClientWithDebug(cfg, opts.Debug)
			if err != nil {
				return err
			}

			target, err := ResolveTargetForExport(opts)
			if err != nil {
				return err
			}

			req := api.RunAgentRequest{
				Messages: []api.Message{
					api.NewTextMessage("user", message),
				},
			}

			// Setup spinner for status updates
			spinner := newSpinner()
			spinner.Start()

			// Track if we've received any content
			var contentStarted bool
			var contentMu sync.Mutex

			// Setup streaming callbacks
			dimColor := color.New(color.FgHiBlack)
			cyanColor := color.New(color.FgCyan)

			runOpts := api.RunAgentOptions{
				OnStatus: func(status, message string) {
					contentMu.Lock()
					started := contentStarted
					contentMu.Unlock()
					if !started {
						spinner.SetMessage(message)
					}
				},
				OnTextDelta: func(delta string) {
					contentMu.Lock()
					if !contentStarted {
						contentStarted = true
						spinner.Stop()
					}
					contentMu.Unlock()
					fmt.Fprint(os.Stdout, delta)
				},
				OnThinkingDelta: func(delta string) {
					contentMu.Lock()
					if !contentStarted {
						contentStarted = true
						spinner.Stop()
					}
					contentMu.Unlock()
					if showThinking {
						dimColor.Fprint(os.Stderr, delta)
					}
				},
				OnToolUse: func(name string, input json.RawMessage) {
					if showTools {
						contentMu.Lock()
						started := contentStarted
						contentMu.Unlock()
						if started {
							cyanColor.Fprintf(os.Stderr, "\n[Tool: %s]\n", name)
						} else {
							// Tool use before content - just update spinner
							spinner.SetMessage(fmt.Sprintf("Using %s...", name))
						}
						if opts.Debug && len(input) > 0 {
							fmt.Fprintf(os.Stderr, "  Input: %s\n", string(input))
						}
					}
				},
				OnToolResult: func(name string, result json.RawMessage) {
					if showTools && opts.Debug {
						fmt.Fprintf(os.Stderr, "  Result (%s): %s\n", name, truncateResult(result))
					}
				},
			}

			// Execute with 15-minute timeout
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()

			_, err = client.RunAgent(ctx, target.Database, target.Schema, agentName, req, runOpts)
			spinner.Stop()
			fmt.Fprintln(os.Stdout) // newline after streaming
			return err
		},
	}

	cmd.Flags().StringVarP(&message, "message", "m", "", "Message to send to the agent (required)")
	cmd.MarkFlagRequired("message")
	cmd.Flags().BoolVar(&showThinking, "show-thinking", false, "Display reasoning tokens on stderr")
	cmd.Flags().BoolVar(&showTools, "show-tools", false, "Display tool usage on stderr")

	return cmd
}

// truncateResult truncates long tool results for display.
func truncateResult(data json.RawMessage) string {
	const maxLen = 200
	s := string(data)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// spinner provides a simple terminal spinner with status message.
type spinner struct {
	frames   []string
	message  string
	mu       sync.Mutex
	stop     chan struct{}
	stopped  bool
	isTTY    bool
	msgColor *color.Color
}

func newSpinner() *spinner {
	return &spinner{
		frames:   []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		message:  "Connecting...",
		stop:     make(chan struct{}),
		isTTY:    term.IsTerminal(int(os.Stderr.Fd())),
		msgColor: color.New(color.FgCyan),
	}
}

func (s *spinner) SetMessage(msg string) {
	s.mu.Lock()
	s.message = msg
	s.mu.Unlock()
}

func (s *spinner) Start() {
	if !s.isTTY {
		// Non-TTY: just print initial message
		fmt.Fprintf(os.Stderr, "%s\n", s.message)
		return
	}

	go func() {
		i := 0
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-s.stop:
				return
			case <-ticker.C:
				s.mu.Lock()
				msg := s.message
				stopped := s.stopped
				s.mu.Unlock()

				if stopped {
					return
				}

				frame := s.frames[i%len(s.frames)]
				// Clear line and print spinner with message
				fmt.Fprintf(os.Stderr, "\r\033[K%s %s", s.msgColor.Sprint(frame), msg)
				i++
			}
		}
	}()
}

func (s *spinner) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	s.mu.Unlock()

	close(s.stop)

	if s.isTTY {
		// Clear the spinner line
		fmt.Fprintf(os.Stderr, "\r\033[K")
	}
}
