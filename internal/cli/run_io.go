package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"coragent/internal/api"
	"coragent/internal/thread"

	"github.com/fatih/color"
	"golang.org/x/term"
)

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
