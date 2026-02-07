package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"coragent/internal/api"
	"coragent/internal/auth"
	"coragent/internal/thread"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newRunCmd(opts *RootOptions) *cobra.Command {
	var message string
	var showThinking bool
	var newThread bool
	var threadID string
	var withoutThread bool

	cmd := &cobra.Command{
		Use:   "run [agent-name]",
		Short: "Run an agent with a message",
		Long: `Run a Cortex Agent and stream the response in real-time.

If agent-name is omitted, you'll be prompted to select from available agents.
If -m is omitted, you'll be prompted to enter a message interactively.

The agent's response is streamed to stdout as it is generated.
Tool usage is displayed on stderr automatically.
Use --show-thinking to display reasoning tokens on stderr.

By default, you'll be prompted to select from existing conversation threads
or create a new one. Use --new to skip selection and start fresh, --thread
to continue a specific thread, or --without-thread for single-turn mode.`,
		Example: `  # Fully interactive (select agent, then enter message)
  coragent run

  # Interactive agent selection with message
  coragent run -m "What are the top sales by region?"

  # Specify agent, enter message interactively
  coragent run my-agent

  # Specify both agent and message
  coragent run my-agent -m "What are the top sales by region?"

  # Start a new conversation thread
  coragent run my-agent --new -m "Starting fresh topic"

  # Continue a specific thread
  coragent run my-agent --thread 12345 -m "Follow-up question"

  # Single-turn mode (no thread tracking)
  coragent run my-agent --without-thread -m "One-off question"

  # With database/schema
  coragent run my-agent -d MY_DB -s MY_SCHEMA -m "Summarize Q4 results"

  # Show thinking/reasoning
  coragent run my-agent -m "Complex query" --show-thinking`,
		Args: cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := auth.LoadConfig(opts.Connection)
			applyAuthOverrides(&cfg, opts)

			client, err := api.NewClientWithDebug(cfg, opts.Debug)
			if err != nil {
				return err
			}

			target, err := ResolveTargetForExport(opts, cfg)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()

			// Determine agent name
			var agentName string
			if len(args) == 1 {
				agentName = args[0]
			} else {
				agents, err := client.ListAgents(ctx, target.Database, target.Schema)
				if err != nil {
					return fmt.Errorf("list agents: %w", err)
				}
				if len(agents) == 0 {
					return fmt.Errorf("no agents found in %s.%s", target.Database, target.Schema)
				}
				agentName = selectAgent(agents)
			}

			// Prompt for message if not provided via flag
			if message == "" {
				line, err := readLine("Enter message: ")
				if err != nil {
					if errors.Is(err, errInterrupted) {
						return nil
					}
					return fmt.Errorf("read message: %w", err)
				}
				message = strings.TrimSpace(line)
				if message == "" {
					return fmt.Errorf("message cannot be empty")
				}
			}

			ctx, cancel = context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()

			// Determine thread settings
			var reqThreadID string
			var reqParentMsgID *int64

			if withoutThread {
				// Single-turn: no thread tracking
			} else if newThread {
				// Create new thread via Threads API
				fmt.Fprintf(os.Stderr, "Creating new thread...\n")
				tid, err := client.CreateThread(ctx)
				if err != nil {
					return fmt.Errorf("create thread: %w", err)
				}
				reqThreadID = tid
				zero := int64(0)
				reqParentMsgID = &zero
			} else if threadID != "" {
				// Explicit thread specified
				reqThreadID = threadID
				state, _ := thread.LoadState()
				if ts := state.FindThread(cfg.Account, target.Database, target.Schema, agentName, threadID); ts != nil {
					reqParentMsgID = &ts.LastMessageID
				} else {
					zero := int64(0)
					reqParentMsgID = &zero
				}
			} else {
				// Default: interactive thread selection
				state, _ := thread.LoadState()
				threads := state.GetThreads(cfg.Account, target.Database, target.Schema, agentName)

				selectedThread := selectThread(threads, agentName)
				if selectedThread == nil {
					// User chose "Create new thread"
					fmt.Fprintf(os.Stderr, "Creating new thread...\n")
					tid, err := client.CreateThread(ctx)
					if err != nil {
						return fmt.Errorf("create thread: %w", err)
					}
					reqThreadID = tid
					zero := int64(0)
					reqParentMsgID = &zero
				} else {
					reqThreadID = selectedThread.ThreadID
					reqParentMsgID = &selectedThread.LastMessageID
				}
			}

			req := api.RunAgentRequest{
				Messages: []api.Message{
					api.NewTextMessage("user", message),
				},
				ThreadID:        reqThreadID,
				ParentMessageID: reqParentMsgID,
			}

			// Setup spinner for status updates
			spinner := newSpinner()
			spinner.Start()

			// Track if we've received any content
			var contentStarted bool
			var contentMu sync.Mutex

			// Capture thread/message IDs from response
			var respThreadID string
			var respMessageID int64

			// Setup streaming callbacks
			dimColor := color.New(color.FgHiBlack)
			cyanColor := color.New(color.FgCyan)

			runOpts := api.RunAgentOptions{
				OnProgress: func(phase string) {
					spinner.SetMessage(phase)
				},
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
					contentMu.Lock()
					started := contentStarted
					contentMu.Unlock()
					if !started {
						spinner.SetMessage(fmt.Sprintf("Using %s...", name))
					} else {
						cyanColor.Fprintf(os.Stderr, "\n[Tool: %s]\n", name)
					}
					if opts.Debug && len(input) > 0 {
						fmt.Fprintf(os.Stderr, "  Input: %s\n", string(input))
					}
				},
				OnToolResult: func(name string, result json.RawMessage) {
					contentMu.Lock()
					started := contentStarted
					contentMu.Unlock()
					if !started {
						spinner.SetMessage("Processing results...")
					}
					if opts.Debug {
						fmt.Fprintf(os.Stderr, "  Result (%s): %s\n", name, truncateResult(result))
					}
				},
				OnMetadata: func(tid string, mid int64) {
					respThreadID = tid
					respMessageID = mid
				},
			}

			_, err = client.RunAgent(ctx, target.Database, target.Schema, agentName, req, runOpts)
			spinner.Stop()
			fmt.Fprintln(os.Stdout) // newline after streaming

			// Save thread state (unless --without-thread)
			if err == nil && !withoutThread && reqThreadID != "" {
				// Use request thread ID if response didn't provide one
				finalThreadID := respThreadID
				if finalThreadID == "" {
					finalThreadID = reqThreadID
				}
				state, _ := thread.LoadState()
				state.AddOrUpdateThread(cfg.Account, target.Database, target.Schema, agentName, thread.ThreadState{
					ThreadID:      finalThreadID,
					LastMessageID: respMessageID,
					LastUsed:      time.Now(),
					Summary:       truncateSummary(message),
				})
				state.Save()
			}

			return err
		},
	}

	cmd.Flags().StringVarP(&message, "message", "m", "", "Message to send to the agent (omit for interactive input)")
	cmd.Flags().BoolVar(&showThinking, "show-thinking", false, "Display reasoning tokens on stderr")
	cmd.Flags().BoolVar(&newThread, "new", false, "Start a new conversation thread")
	cmd.Flags().StringVar(&threadID, "thread", "", "Continue a specific thread by ID")
	cmd.Flags().BoolVar(&withoutThread, "without-thread", false, "Run without thread support (single-turn)")

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
	frames    []string
	message   string
	mu        sync.Mutex
	stop      chan struct{}
	stopped   bool
	isTTY     bool
	msgColor  *color.Color
	dimColor  *color.Color
	startTime time.Time
}

func newSpinner() *spinner {
	return &spinner{
		frames:   []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		message:  "Connecting...",
		stop:     make(chan struct{}),
		isTTY:    term.IsTerminal(int(os.Stderr.Fd())),
		msgColor: color.New(color.FgCyan),
		dimColor: color.New(color.FgHiBlack),
	}
}

func (s *spinner) SetMessage(msg string) {
	s.mu.Lock()
	s.message = msg
	s.mu.Unlock()
}

func (s *spinner) Start() {
	s.startTime = time.Now()

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
				elapsed := time.Since(s.startTime)
				if elapsed >= 2*time.Second {
					fmt.Fprintf(os.Stderr, "\r\033[K%s %s %s", s.msgColor.Sprint(frame), msg, s.dimColor.Sprintf("(%s)", formatElapsed(elapsed)))
				} else {
					fmt.Fprintf(os.Stderr, "\r\033[K%s %s", s.msgColor.Sprint(frame), msg)
				}
				i++
			}
		}
	}()
}

// formatElapsed formats a duration as a compact elapsed time string.
func formatElapsed(d time.Duration) string {
	d = d.Truncate(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if s == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dm%ds", m, s)
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

// selectThread shows interactive thread selection UI.
func selectThread(threads []thread.ThreadState, agentName string) *thread.ThreadState {
	if len(threads) == 0 {
		return nil
	}

	// Sort by LastUsed descending
	sort.Slice(threads, func(i, j int) bool {
		return threads[i].LastUsed.After(threads[j].LastUsed)
	})

	// Show thread list
	fmt.Fprintf(os.Stderr, "Available threads for %s:\n", agentName)
	for i, t := range threads {
		age := formatAge(t.LastUsed)
		summary := truncateDisplay(t.Summary, 40)
		fmt.Fprintf(os.Stderr, "  [%d] Thread %s (%s) - \"%s\"\n", i+1, t.ThreadID, age, summary)
	}
	fmt.Fprintf(os.Stderr, "  [%d] Create new thread\n", len(threads)+1)

	// Read selection
	fmt.Fprintf(os.Stderr, "Select thread [1-%d]: ", len(threads)+1)

	line, _ := readLine("")
	line = strings.TrimSpace(line)

	selection, err := strconv.Atoi(line)
	if err != nil || selection < 1 || selection > len(threads) {
		return nil // Create new thread
	}
	return &threads[selection-1]
}

// selectAgent shows interactive agent selection UI.
func selectAgent(agents []api.AgentListItem) string {
	fmt.Fprintf(os.Stderr, "Available agents:\n")
	for i, a := range agents {
		if a.Comment != "" {
			fmt.Fprintf(os.Stderr, "  [%d] %s - \"%s\"\n", i+1, a.Name, truncateDisplay(a.Comment, 50))
		} else {
			fmt.Fprintf(os.Stderr, "  [%d] %s\n", i+1, a.Name)
		}
	}

	fmt.Fprintf(os.Stderr, "Select agent [1-%d]: ", len(agents))

	line, _ := readLine("")
	line = strings.TrimSpace(line)

	selection, err := strconv.Atoi(line)
	if err != nil || selection < 1 || selection > len(agents) {
		// Default to first agent
		return agents[0].Name
	}
	return agents[selection-1].Name
}

// formatAge formats a time as a human-readable relative duration.
func formatAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

// truncateDisplay truncates a string for display.
func truncateDisplay(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// truncateSummary creates a summary from the first message.
func truncateSummary(s string) string {
	const maxLen = 100
	// Remove newlines
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
