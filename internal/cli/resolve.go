package cli

import (
	"fmt"
	"strings"

	"coragent/internal/agent"
	"coragent/internal/auth"
)

type Target struct {
	Database string
	Schema   string
}

func ResolveTarget(spec agent.AgentSpec, opts *RootOptions, cfg auth.Config) (Target, error) {
	db := firstNonEmpty(opts.Database, deployValue(spec, "database"), cfg.Database)
	schema := firstNonEmpty(opts.Schema, deployValue(spec, "schema"), cfg.Schema)

	if db == "" || schema == "" {
		return Target{}, fmt.Errorf("database/schema is required (use --database/--schema, YAML deploy.database/schema, or env SNOWFLAKE_DATABASE/SNOWFLAKE_SCHEMA)")
	}
	return Target{Database: db, Schema: schema}, nil
}

func ResolveTargetForExport(opts *RootOptions, cfg auth.Config) (Target, error) {
	db := firstNonEmpty(opts.Database, cfg.Database)
	schema := firstNonEmpty(opts.Schema, cfg.Schema)
	if db == "" || schema == "" {
		return Target{}, fmt.Errorf("database/schema is required for export (use --database/--schema or env SNOWFLAKE_DATABASE/SNOWFLAKE_SCHEMA)")
	}
	return Target{Database: db, Schema: schema}, nil
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

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
