package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"coragent/internal/api"
	"coragent/internal/auth"
	"coragent/internal/grant"
)

// buildClient constructs an API client from the root options.
// It loads the auth config, applies CLI flag overrides, and creates the client.
func buildClient(opts *RootOptions) (*api.Client, error) {
	cfg := auth.LoadConfig(opts.Connection)
	applyAuthOverrides(&cfg, opts)
	client, err := api.NewClientWithDebug(cfg, opts.Debug)
	if err != nil {
		return nil, UserErr(err)
	}
	return client, nil
}

// buildClientAndCfg constructs an API client and also returns the resolved
// auth config, which commands need for ResolveTarget.
func buildClientAndCfg(opts *RootOptions) (*api.Client, auth.Config, error) {
	cfg := auth.LoadConfig(opts.Connection)
	applyAuthOverrides(&cfg, opts)
	client, err := api.NewClientWithDebug(cfg, opts.Debug)
	if err != nil {
		return nil, auth.Config{}, UserErr(err)
	}
	return client, cfg, nil
}

// confirm prints a [y/N] prompt to stdout and reads one line from r.
// Returns true if the user answers "y" or "yes" (case-insensitive).
// It is used by apply and delete to guard destructive operations.
func confirm(prompt string, r io.Reader) bool {
	reader := bufio.NewReader(r)
	fmt.Fprint(os.Stdout, prompt+" [y/N]: ")
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes"
}

// convertGrantRows converts api.ShowGrantsRow values to grant.ShowGrantsRow values.
func convertGrantRows(rows []api.ShowGrantsRow) []grant.ShowGrantsRow {
	result := make([]grant.ShowGrantsRow, len(rows))
	for i, r := range rows {
		result[i] = grant.ShowGrantsRow{
			Privilege:   r.Privilege,
			GrantedTo:   r.GrantedTo,
			GranteeName: r.GranteeName,
		}
	}
	return result
}
