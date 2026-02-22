package cli

import (
	"context"
	"fmt"

	"coragent/internal/agent"
	"coragent/internal/api"
	"coragent/internal/auth"
	"coragent/internal/diff"
	"coragent/internal/grant"
)

// buildPlanItems queries the current state of each spec and computes the
// changes required. It accepts service interfaces so it can be called from
// both the plan and apply commands and tested with fake implementations.
func buildPlanItems(
	ctx context.Context,
	specs []agent.ParsedAgent,
	opts *RootOptions,
	cfg auth.Config,
	agentSvc api.AgentService,
	grantSvc api.GrantService,
) ([]applyItem, error) {
	items := make([]applyItem, 0, len(specs))

	for _, item := range specs {
		target, err := ResolveTarget(item.Spec, opts, cfg)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", item.Path, err)
		}

		remote, exists, err := agentSvc.GetAgent(ctx, target.Database, target.Schema, item.Spec.Name)
		if err != nil {
			return nil, fmt.Errorf("snowflake API error: %w", err)
		}

		var grantCfg *agent.GrantConfig
		if item.Spec.Deploy != nil {
			grantCfg = item.Spec.Deploy.Grant
		}
		desiredGrants := grant.FromGrantConfig(grantCfg)

		if !exists {
			grantDiff := grant.ComputeDiff(desiredGrants, grant.GrantState{})
			items = append(items, applyItem{
				Parsed:    item,
				Target:    target,
				Exists:    false,
				GrantDiff: grantDiff,
			})
			continue
		}

		grantRows, err := grantSvc.ShowGrants(ctx, target.Database, target.Schema, item.Spec.Name)
		if err != nil {
			return nil, fmt.Errorf("show grants: %w", err)
		}
		currentGrants := grant.FromShowGrantsRows(convertGrantRows(grantRows))
		grantDiff := grant.ComputeDiff(desiredGrants, currentGrants)

		changes, err := diff.Diff(item.Spec, remote)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", item.Path, err)
		}

		items = append(items, applyItem{
			Parsed:    item,
			Target:    target,
			Exists:    true,
			Changes:   changes,
			GrantDiff: grantDiff,
		})
	}

	return items, nil
}
