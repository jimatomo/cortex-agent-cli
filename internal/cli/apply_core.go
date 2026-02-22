package cli

import (
	"context"
	"fmt"
	"strings"

	"coragent/internal/api"
	"coragent/internal/diff"
	"coragent/internal/grant"
)

// executeApply applies each plan item to Snowflake.
// It creates or updates agents as needed, then applies the precomputed grant diff.
// Items with no spec changes still have their grants applied to converge on desired state.
// Returns the subset of items that were created or updated (not the no-change ones).
func executeApply(
	ctx context.Context,
	items []applyItem,
	agentSvc api.AgentService,
	grantSvc api.GrantService,
) ([]applyItem, error) {
	var applied []applyItem
	for _, item := range items {
		db, schema, name := item.Target.Database, item.Target.Schema, item.Parsed.Spec.Name

		if !item.Exists {
			if err := agentSvc.CreateAgent(ctx, db, schema, item.Parsed.Spec); err != nil {
				return applied, fmt.Errorf("create %s: %w", name, err)
			}
			if err := applyGrantDiff(ctx, grantSvc, db, schema, name, item.GrantDiff); err != nil {
				return applied, fmt.Errorf("grants for %s: %w", name, err)
			}
			applied = append(applied, item)
			continue
		}

		if diff.HasChanges(item.Changes) {
			payload, err := updatePayload(item.Parsed.Spec, item.Changes)
			if err != nil {
				return applied, fmt.Errorf("%s: %w", item.Parsed.Path, err)
			}
			if err := agentSvc.UpdateAgent(ctx, db, schema, name, payload); err != nil {
				return applied, fmt.Errorf("update %s: %w", name, err)
			}
			applied = append(applied, item)
		}

		if err := applyGrantDiff(ctx, grantSvc, db, schema, name, item.GrantDiff); err != nil {
			return applied, fmt.Errorf("grants for %s: %w", name, err)
		}
	}
	return applied, nil
}

// applyGrantDiff executes the GRANT and REVOKE statements described by the diff.
// It is a no-op when the diff has no changes.
func applyGrantDiff(
	ctx context.Context,
	grantSvc api.GrantService,
	db, schema, agentName string,
	gd grant.GrantDiff,
) error {
	if !gd.HasChanges() {
		return nil
	}
	var errs []string
	for _, e := range gd.ToRevoke {
		if err := grantSvc.ExecuteRevoke(ctx, db, schema, agentName, e.RoleType, e.RoleName, e.Privilege); err != nil {
			errs = append(errs, fmt.Sprintf("REVOKE %s FROM %s %s: %v", e.Privilege, e.RoleType, e.RoleName, err))
		}
	}
	for _, e := range gd.ToGrant {
		if err := grantSvc.ExecuteGrant(ctx, db, schema, agentName, e.RoleType, e.RoleName, e.Privilege); err != nil {
			errs = append(errs, fmt.Sprintf("GRANT %s TO %s %s: %v", e.Privilege, e.RoleType, e.RoleName, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("grant/revoke errors:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}
