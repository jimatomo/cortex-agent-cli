package cli

import (
	"fmt"
	"os"
	"strings"

	"coragent/internal/agent"
)

type Target struct {
	Database string
	Schema   string
}

func ResolveTarget(spec agent.AgentSpec, opts *RootOptions) (Target, error) {
	db := firstNonEmpty(opts.Database, deployValue(spec, "database"), os.Getenv("SNOWFLAKE_DATABASE"))
	schema := firstNonEmpty(opts.Schema, deployValue(spec, "schema"), os.Getenv("SNOWFLAKE_SCHEMA"))

	if db == "" || schema == "" {
		return Target{}, fmt.Errorf("database/schema is required (use --database/--schema, YAML deploy.database/schema, or env SNOWFLAKE_DATABASE/SNOWFLAKE_SCHEMA)")
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
