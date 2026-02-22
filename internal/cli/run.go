package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"coragent/internal/api"
	"coragent/internal/thread"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
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
			client, cfg, err := buildClientAndCfg(opts)
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
				_ = state.Save()
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

