package cli

import (
	"fmt"
	"strings"

	"coragent/internal/agent"
	"coragent/internal/auth"
)

type Target struct {
	Database         string
	Schema           string
	QuoteIdentifiers bool
}

func ResolveTarget(spec agent.AgentSpec, opts *RootOptions, cfg auth.Config) (Target, error) {
	db := firstNonEmpty(opts.Database, deployValue(spec, "database"), cfg.Database)
	schema := firstNonEmpty(opts.Schema, deployValue(spec, "schema"), cfg.Schema)

	if db == "" || schema == "" {
		return Target{}, fmt.Errorf("database/schema is required — set SNOWFLAKE_DATABASE/SNOWFLAKE_SCHEMA, use --database/--schema flags, or add deploy.database/schema to the YAML spec")
	}

	quoteIDs := opts.QuoteIdentifiers || deployBoolValue(spec, "quote_identifiers")
	if quoteIDs {
		db = quoteIdentifier(db)
		schema = quoteIdentifier(schema)
	}

	return Target{Database: db, Schema: schema, QuoteIdentifiers: quoteIDs}, nil
}

func ResolveTargetForExport(opts *RootOptions, cfg auth.Config) (Target, error) {
	db := firstNonEmpty(opts.Database, cfg.Database)
	schema := firstNonEmpty(opts.Schema, cfg.Schema)
	if db == "" || schema == "" {
		return Target{}, fmt.Errorf("database/schema is required for export (use --database/--schema or env SNOWFLAKE_DATABASE/SNOWFLAKE_SCHEMA)")
	}

	quoteIDs := opts.QuoteIdentifiers
	if quoteIDs {
		db = quoteIdentifier(db)
		schema = quoteIdentifier(schema)
	}

	return Target{Database: db, Schema: schema, QuoteIdentifiers: quoteIDs}, nil
}

// quoteIdentifier wraps a value in double quotes for case-sensitive SQL identifiers.
// If the value is already quoted, it is returned as-is.
func quoteIdentifier(value string) string {
	if len(value) >= 2 && strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
		return value
	}
	return `"` + value + `"`
}

func deployValue(spec agent.AgentSpec, key string) string {
	if spec.Deploy == nil {
		return ""
	}
	switch key {
	case "database":
		return strings.TrimSpace(spec.Deploy.Database)
	case "schema":
		return strings.TrimSpace(spec.Deploy.Schema)
	default:
		return ""
	}
}

func deployBoolValue(spec agent.AgentSpec, key string) bool {
	if spec.Deploy == nil {
		return false
	}
	switch key {
	case "quote_identifiers":
		return spec.Deploy.QuoteIdentifiers
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
