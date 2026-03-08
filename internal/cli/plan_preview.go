package cli

import (
	"fmt"
	"io"

	"coragent/internal/diff"
	"coragent/internal/grant"

	"github.com/fatih/color"
)

type planPreviewSummary struct {
	createCount   int
	updateCount   int
	noChangeCount int
}

func writePlanPreview(w io.Writer, items []applyItem) (planPreviewSummary, error) {
	summary := summarizePlanPreview(items)

	for _, item := range items {
		if isUnchangedPlanItem(item) {
			continue
		}

		fmt.Fprintf(w, "%s:\n", item.Parsed.Spec.Name)
		fmt.Fprintf(w, "  database: %s\n", item.Target.Database)
		fmt.Fprintf(w, "  schema:   %s\n", item.Target.Schema)

		if !item.Exists {
			color.New(color.FgGreen).Fprintln(w, "  + create")
			createChanges, err := diff.DiffForCreate(item.Parsed.Spec)
			if err != nil {
				return planPreviewSummary{}, fmt.Errorf("%s: %w", item.Parsed.Path, err)
			}
			for _, c := range createChanges {
				fmt.Fprintf(w, "    %s %s: %s\n",
					color.New(color.FgGreen).Sprint("+"),
					c.Path,
					formatValue(c.After),
				)
			}
			writeGrantPlan(w, item.GrantDiff)
			continue
		}

		for _, c := range item.Changes {
			fmt.Fprintf(w, "  %s %s: %s\n",
				changeSymbol(c.Type),
				c.Path,
				formatChange(c),
			)
		}
		writeGrantPlan(w, item.GrantDiff)
	}

	fmt.Fprintf(w, "\nPlan: %d to create, %d to update, %d unchanged\n",
		summary.createCount,
		summary.updateCount,
		summary.noChangeCount,
	)

	return summary, nil
}

func summarizePlanPreview(items []applyItem) planPreviewSummary {
	var summary planPreviewSummary

	for _, item := range items {
		switch {
		case !item.Exists:
			summary.createCount++
		case diff.HasChanges(item.Changes) || item.GrantDiff.HasChanges():
			summary.updateCount++
		default:
			summary.noChangeCount++
		}
	}

	return summary
}

func isUnchangedPlanItem(item applyItem) bool {
	return item.Exists && !diff.HasChanges(item.Changes) && !item.GrantDiff.HasChanges()
}

func writeGrantPlan(w io.Writer, diff grant.GrantDiff) {
	if !diff.HasChanges() {
		return
	}

	fmt.Fprintf(w, "  grants:\n")

	for _, e := range diff.ToRevoke {
		fmt.Fprintf(w, "    %s %s TO %s %s\n",
			color.New(color.FgRed).Sprint("-"),
			e.Privilege,
			e.RoleType,
			e.RoleName)
	}

	for _, e := range diff.ToGrant {
		fmt.Fprintf(w, "    %s %s TO %s %s\n",
			color.New(color.FgGreen).Sprint("+"),
			e.Privilege,
			e.RoleType,
			e.RoleName)
	}
}
