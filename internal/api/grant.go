package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// ShowGrantsRow represents a row from SHOW GRANTS ON AGENT.
type ShowGrantsRow struct {
	Privilege   string
	GrantedTo   string // "ROLE" or "DATABASE_ROLE"
	GranteeName string
}

// ShowGrants executes SHOW GRANTS ON AGENT and returns current grants.
func (c *Client) ShowGrants(ctx context.Context, db, schema, agentName string) ([]ShowGrantsRow, error) {
	fqAgent := fmt.Sprintf("%s.%s.%s",
		identifierSegment(db),
		identifierSegment(schema),
		identifierSegment(agentName))

	stmt := fmt.Sprintf("SHOW GRANTS ON AGENT %s", fqAgent)

	payload := sqlStatementRequest{
		Statement: stmt,
		Database:  unquoteIdentifier(db),
		Schema:    unquoteIdentifier(schema),
	}
	if strings.TrimSpace(c.authCfg.Warehouse) != "" {
		payload.Warehouse = c.authCfg.Warehouse
	}
	if strings.TrimSpace(c.role) != "" {
		payload.Role = c.role
	}

	var resp sqlStatementResponse
	if err := c.doJSON(ctx, http.MethodPost, c.sqlURL(), payload, &resp); err != nil {
		return nil, err
	}

	// Parse response - expected columns:
	// created_on, privilege, granted_on, name, granted_to, grantee_name, grant_option, granted_by
	var rows []ShowGrantsRow
	colIndex := make(map[string]int)
	for i, col := range resp.ResultSetMetaData.RowType {
		colIndex[strings.ToLower(col.Name)] = i
	}

	for _, row := range resp.Data {
		privIdx, ok1 := colIndex["privilege"]
		grantedToIdx, ok2 := colIndex["granted_to"]
		granteeIdx, ok3 := colIndex["grantee_name"]

		if !ok1 || !ok2 || !ok3 {
			continue
		}

		if privIdx >= len(row) || grantedToIdx >= len(row) || granteeIdx >= len(row) {
			continue
		}

		priv, _ := row[privIdx].(string)
		grantedTo, _ := row[grantedToIdx].(string)
		grantee, _ := row[granteeIdx].(string)

		// For DATABASE_ROLE, prefix with database name if not already qualified
		if grantedTo == "DATABASE_ROLE" && !strings.Contains(grantee, ".") {
			grantee = unquoteIdentifier(db) + "." + grantee
		}

		rows = append(rows, ShowGrantsRow{
			Privilege:   priv,
			GrantedTo:   grantedTo, // "ROLE" or "DATABASE_ROLE"
			GranteeName: grantee,
		})
	}

	return rows, nil
}

// ExecuteGrant executes a GRANT statement for the given privilege.
func (c *Client) ExecuteGrant(ctx context.Context, db, schema, agentName, roleType, roleName, privilege string) error {
	fqAgent := fmt.Sprintf("%s.%s.%s",
		identifierSegment(db),
		identifierSegment(schema),
		identifierSegment(agentName))

	var stmt string
	if roleType == "ROLE" {
		stmt = fmt.Sprintf("GRANT %s ON AGENT %s TO ROLE %s",
			privilege, fqAgent, identifierSegment(roleName))
	} else {
		// DATABASE ROLE - roleName is already fully qualified (DB.ROLE_NAME)
		stmt = fmt.Sprintf("GRANT %s ON AGENT %s TO DATABASE ROLE %s",
			privilege, fqAgent, roleName)
	}

	payload := sqlStatementRequest{
		Statement: stmt,
		Database:  unquoteIdentifier(db),
		Schema:    unquoteIdentifier(schema),
	}
	if strings.TrimSpace(c.authCfg.Warehouse) != "" {
		payload.Warehouse = c.authCfg.Warehouse
	}
	if strings.TrimSpace(c.role) != "" {
		payload.Role = c.role
	}

	return c.doJSON(ctx, http.MethodPost, c.sqlURL(), payload, nil)
}

// ExecuteRevoke executes a REVOKE statement for the given privilege.
func (c *Client) ExecuteRevoke(ctx context.Context, db, schema, agentName, roleType, roleName, privilege string) error {
	fqAgent := fmt.Sprintf("%s.%s.%s",
		identifierSegment(db),
		identifierSegment(schema),
		identifierSegment(agentName))

	var stmt string
	if roleType == "ROLE" {
		stmt = fmt.Sprintf("REVOKE %s ON AGENT %s FROM ROLE %s",
			privilege, fqAgent, identifierSegment(roleName))
	} else {
		// DATABASE ROLE - roleName is already fully qualified (DB.ROLE_NAME)
		stmt = fmt.Sprintf("REVOKE %s ON AGENT %s FROM DATABASE ROLE %s",
			privilege, fqAgent, roleName)
	}

	payload := sqlStatementRequest{
		Statement: stmt,
		Database:  unquoteIdentifier(db),
		Schema:    unquoteIdentifier(schema),
	}
	if strings.TrimSpace(c.authCfg.Warehouse) != "" {
		payload.Warehouse = c.authCfg.Warehouse
	}
	if strings.TrimSpace(c.role) != "" {
		payload.Role = c.role
	}

	return c.doJSON(ctx, http.MethodPost, c.sqlURL(), payload, nil)
}
