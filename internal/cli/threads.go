package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"coragent/internal/api"
	"coragent/internal/thread"

	"github.com/spf13/cobra"
)

// threadInfo holds a flattened thread with its agent key for display.
type threadInfo struct {
	AgentKey string
	State    thread.ThreadState
}

func newThreadsCmd(opts *RootOptions) *cobra.Command {
	var listOnly bool
	var deleteID string

	cmd := &cobra.Command{
		Use:   "threads",
		Short: "Manage conversation threads",
		Long: `List and delete conversation threads.

By default, runs in interactive mode where you can view all threads
and select which ones to delete.

Use --list to display threads and exit without interaction.
Use --delete to delete a specific thread by ID.`,
		Example: `  # Interactive mode
  coragent threads

  # List all threads (non-interactive)
  coragent threads --list

  # Delete a specific thread
  coragent threads --delete 29864464`,
		RunE: func(cmd *cobra.Command, args []string) error {
			state, err := thread.LoadState()
			if err != nil {
				return fmt.Errorf("load thread state: %w", err)
			}

			// List mode doesn't need API access
			if listOnly {
				return displayThreads(state)
			}

			// Delete and interactive modes need API client
			client, err := buildClient(opts)
			if err != nil {
				return err
			}

			if deleteID != "" {
				return deleteThreadByID(client, state, deleteID)
			}

			return interactiveThreadManager(client, state)
		},
	}

	cmd.Flags().BoolVar(&listOnly, "list", false, "List threads and exit")
	cmd.Flags().StringVar(&deleteID, "delete", "", "Delete specific thread by ID")

	return cmd
}

// displayThreads shows all threads grouped by agent.
func displayThreads(state *thread.StateStore) error {
	allThreads := state.GetAllThreads()
	if len(allThreads) == 0 {
		fmt.Println("No threads found.")
		return nil
	}

	// Flatten and sort threads
	threads := flattenThreads(allThreads)
	if len(threads) == 0 {
		fmt.Println("No threads found.")
		return nil
	}

	fmt.Println("Threads:")
	for i, t := range threads {
		age := formatAge(t.State.LastUsed)
		summary := truncateDisplay(t.State.Summary, 40)
		fmt.Printf("  [%d] Thread %s (%s) - \"%s\"\n", i+1, t.State.ThreadID, age, summary)
		fmt.Printf("      Agent: %s\n", t.AgentKey)
	}

	return nil
}

// interactiveThreadManager provides an interactive UI for managing threads.
func interactiveThreadManager(client *api.Client, state *thread.StateStore) error {
	reader := bufio.NewReader(os.Stdin)

	for {
		allThreads := state.GetAllThreads()
		threads := flattenThreads(allThreads)

		if len(threads) == 0 {
			fmt.Println("No threads found.")
			return nil
		}

		// Display threads
		fmt.Println("\nThreads:")
		for i, t := range threads {
			age := formatAge(t.State.LastUsed)
			summary := truncateDisplay(t.State.Summary, 40)
			fmt.Printf("  [%d] Thread %s (%s) - \"%s\"\n", i+1, t.State.ThreadID, age, summary)
			fmt.Printf("      Agent: %s\n", t.AgentKey)
		}

		// Show menu
		fmt.Println("\n  [d] Delete threads  [q] Quit")
		fmt.Print("  Select: ")

		line, err := reader.ReadString('\n')
		if err != nil {
			return nil
		}
		line = strings.TrimSpace(strings.ToLower(line))

		switch line {
		case "q", "quit", "exit":
			return nil
		case "d", "delete":
			if err := handleDeleteMode(reader, client, state, threads); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
		default:
			fmt.Println("Invalid selection. Press 'd' to delete or 'q' to quit.")
		}
	}
}

// handleDeleteMode prompts user to select threads for deletion.
func handleDeleteMode(reader *bufio.Reader, client *api.Client, state *thread.StateStore, threads []threadInfo) error {
	fmt.Print("  Select threads to delete (space-separated, or 'all'): ")

	line, err := reader.ReadString('\n')
	if err != nil {
		return nil
	}
	line = strings.TrimSpace(line)

	if line == "" {
		return nil
	}

	var toDelete []threadInfo

	if strings.ToLower(line) == "all" {
		toDelete = threads
	} else {
		// Parse space-separated numbers
		parts := strings.Fields(line)
		for _, p := range parts {
			idx, err := strconv.Atoi(p)
			if err != nil || idx < 1 || idx > len(threads) {
				fmt.Printf("  Invalid selection: %s\n", p)
				continue
			}
			toDelete = append(toDelete, threads[idx-1])
		}
	}

	if len(toDelete) == 0 {
		fmt.Println("  No threads selected.")
		return nil
	}

	// Confirm deletion
	fmt.Printf("  Delete %d thread(s)? [y/N]: ", len(toDelete))
	confirm, _ := reader.ReadString('\n')
	confirm = strings.TrimSpace(strings.ToLower(confirm))

	if confirm != "y" && confirm != "yes" {
		fmt.Println("  Cancelled.")
		return nil
	}

	// Delete threads
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, t := range toDelete {
		if err := client.DeleteThread(ctx, t.State.ThreadID); err != nil {
			fmt.Printf("  Failed to delete thread %s: %v\n", t.State.ThreadID, err)
			continue
		}

		// Remove from local state
		parts := strings.Split(t.AgentKey, "/")
		if len(parts) == 4 {
			state.DeleteThread(parts[0], parts[1], parts[2], parts[3], t.State.ThreadID)
		}
		fmt.Printf("  Deleted thread %s\n", t.State.ThreadID)
	}

	// Save state
	if err := state.Save(); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	return nil
}

// deleteThreadByID deletes a specific thread by ID.
func deleteThreadByID(client *api.Client, state *thread.StateStore, threadID string) error {
	// Find the thread in local state
	allThreads := state.GetAllThreads()
	var found *threadInfo

	for agentKey, threads := range allThreads {
		for _, t := range threads {
			if t.ThreadID == threadID {
				found = &threadInfo{AgentKey: agentKey, State: t}
				break
			}
		}
		if found != nil {
			break
		}
	}

	if found == nil {
		return fmt.Errorf("thread %s not found in local state", threadID)
	}

	// Delete from API
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.DeleteThread(ctx, threadID); err != nil {
		return fmt.Errorf("delete thread: %w", err)
	}

	// Remove from local state
	parts := strings.Split(found.AgentKey, "/")
	if len(parts) == 4 {
		state.DeleteThread(parts[0], parts[1], parts[2], parts[3], threadID)
	}

	if err := state.Save(); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	fmt.Printf("Deleted thread %s\n", threadID)
	return nil
}

// flattenThreads converts the nested thread map into a sorted slice.
func flattenThreads(allThreads map[string][]thread.ThreadState) []threadInfo {
	var result []threadInfo

	for agentKey, threads := range allThreads {
		for _, t := range threads {
			result = append(result, threadInfo{
				AgentKey: agentKey,
				State:    t,
			})
		}
	}

	// Sort by LastUsed descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].State.LastUsed.After(result[j].State.LastUsed)
	})

	return result
}
