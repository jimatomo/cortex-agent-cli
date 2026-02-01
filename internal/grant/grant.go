// Package grant provides grant state management and diff computation for Cortex Agents.
package grant

import (
	"fmt"
	"strings"

	"coragent/internal/agent"
)

// GrantEntry represents a single grant on an agent.
type GrantEntry struct {
	Privilege string // USAGE, MODIFY, MONITOR, ALL
	RoleType  string // "ROLE" or "DATABASE ROLE"
	RoleName  string // role name (fully qualified for database roles)
}

// GrantState represents the current grant state of an agent.
type GrantState struct {
	Entries []GrantEntry
}

// GrantDiff represents the difference between desired and current state.
type GrantDiff struct {
	ToGrant  []GrantEntry // Entries to add
	ToRevoke []GrantEntry // Entries to remove
}

// HasChanges returns true if there are grants to add or revoke.
func (d GrantDiff) HasChanges() bool {
	return len(d.ToGrant) > 0 || len(d.ToRevoke) > 0
}

// entryKey generates a unique key for a GrantEntry for set comparison.
// Keys are uppercased for case-insensitive comparison.
func entryKey(e GrantEntry) string {
	return fmt.Sprintf("%s|%s|%s",
		strings.ToUpper(e.Privilege),
		strings.ToUpper(e.RoleType),
		strings.ToUpper(e.RoleName))
}

// ComputeDiff compares desired state (from YAML) with current state (from SHOW GRANTS).
func ComputeDiff(desired, current GrantState) GrantDiff {
	// Build sets for comparison
	desiredSet := make(map[string]bool)
	for _, e := range desired.Entries {
		desiredSet[entryKey(e)] = true
	}

	currentSet := make(map[string]bool)
	for _, e := range current.Entries {
		currentSet[entryKey(e)] = true
	}

	var diff GrantDiff

	// Find entries to grant (in desired but not in current)
	for _, e := range desired.Entries {
		if !currentSet[entryKey(e)] {
			diff.ToGrant = append(diff.ToGrant, e)
		}
	}

	// Find entries to revoke (in current but not in desired)
	for _, e := range current.Entries {
		if !desiredSet[entryKey(e)] {
			diff.ToRevoke = append(diff.ToRevoke, e)
		}
	}

	return diff
}

// allPrivileges is the list of individual privileges that ALL expands to.
var allPrivileges = []string{"USAGE", "MODIFY", "MONITOR"}

// expandPrivilege expands ALL to individual privileges, or returns the privilege as-is.
func expandPrivilege(priv string) []string {
	if strings.ToUpper(priv) == "ALL" {
		return allPrivileges
	}
	return []string{strings.ToUpper(priv)}
}

// FromGrantConfig converts YAML GrantConfig to GrantState.
// ALL privilege is expanded to individual privileges (USAGE, MODIFY, MONITOR).
func FromGrantConfig(cfg *agent.GrantConfig) GrantState {
	if cfg == nil {
		return GrantState{}
	}

	var entries []GrantEntry

	for _, rg := range cfg.AccountRoles {
		for _, priv := range rg.Privileges {
			for _, expandedPriv := range expandPrivilege(priv) {
				entries = append(entries, GrantEntry{
					Privilege: expandedPriv,
					RoleType:  "ROLE",
					RoleName:  rg.Role,
				})
			}
		}
	}

	for _, rg := range cfg.DatabaseRoles {
		for _, priv := range rg.Privileges {
			for _, expandedPriv := range expandPrivilege(priv) {
				entries = append(entries, GrantEntry{
					Privilege: expandedPriv,
					RoleType:  "DATABASE ROLE",
					RoleName:  rg.Role,
				})
			}
		}
	}

	return GrantState{Entries: entries}
}

// ShowGrantsRow represents a row from SHOW GRANTS ON AGENT.
type ShowGrantsRow struct {
	Privilege   string
	GrantedTo   string // "ROLE" or "DATABASE_ROLE"
	GranteeName string
}

// FromShowGrantsRows converts API response rows to GrantState.
// OWNERSHIP privilege is ignored as it is managed automatically by Snowflake.
func FromShowGrantsRows(rows []ShowGrantsRow) GrantState {
	var entries []GrantEntry

	for _, row := range rows {
		// Skip OWNERSHIP privilege - it's managed automatically by Snowflake
		if strings.ToUpper(row.Privilege) == "OWNERSHIP" {
			continue
		}

		roleType := "ROLE"
		if row.GrantedTo == "DATABASE_ROLE" {
			roleType = "DATABASE ROLE"
		}

		entries = append(entries, GrantEntry{
			Privilege: row.Privilege,
			RoleType:  roleType,
			RoleName:  row.GranteeName,
		})
	}

	return GrantState{Entries: entries}
}
