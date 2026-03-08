package agent

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type ParsedAgent struct {
	Path string
	Spec AgentSpec
}

// LoadAgents loads agent specs from a file or directory.
// If path is empty, it defaults to the current directory.
// If recursive is true and path is a directory, it will recursively load from subdirectories.
// envName selects the vars environment group (empty string uses "default").
func LoadAgents(path string, recursive bool, envName string) ([]ParsedAgent, error) {
	if strings.TrimSpace(path) == "" {
		path = "."
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat path %q: %w", path, err)
	}

	if info.IsDir() {
		return loadFromDir(path, recursive, envName)
	}

	spec, err := loadFromFile(path, envName)
	if err != nil {
		return nil, err
	}
	return []ParsedAgent{{Path: path, Spec: spec}}, nil
}

func loadFromDir(dir string, recursive bool, envName string) ([]ParsedAgent, error) {
	var files []string
	if recursive {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			if isYAML(path) {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk directory %q: %w", dir, err)
		}
	} else {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("read directory %q: %w", dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if isYAML(path) {
				files = append(files, path)
			}
		}
	}

	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("no YAML files found in %q", dir)
	}

	results := make([]ParsedAgent, 0, len(files))
	for _, file := range files {
		spec, err := loadFromFile(file, envName)
		if err != nil {
			return nil, err
		}
		results = append(results, ParsedAgent{Path: file, Spec: spec})
	}
	return results, nil
}

func loadFromFile(path string, envName string) (AgentSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AgentSpec{}, fmt.Errorf("read file %q: %w", path, err)
	}

	// 1st pass: extract vars section (lenient parse)
	var wrapper varsWrapper
	if err := yaml.Unmarshal(data, &wrapper); err != nil {
		return AgentSpec{}, fmt.Errorf("parse YAML %q: %w", path, err)
	}

	// Resolve variables if vars section exists
	resolved, err := resolveVars(wrapper.Vars, envName)
	if err != nil {
		return AgentSpec{}, fmt.Errorf("%s: %w", path, err)
	}

	// 2nd pass: parse into yaml.Node tree for manipulation
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return AgentSpec{}, fmt.Errorf("parse YAML %q: %w", path, err)
	}

	// Strip vars node before KnownFields check
	stripVarsNode(&doc)

	// Substitute variable references
	if err := substituteVars(&doc, resolved); err != nil {
		return AgentSpec{}, fmt.Errorf("%s: %w", path, err)
	}

	// Re-encode node to bytes, then decode with KnownFields(true)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return AgentSpec{}, fmt.Errorf("re-encode YAML %q: %w", path, err)
	}
	if err := enc.Close(); err != nil {
		return AgentSpec{}, fmt.Errorf("flush YAML encoder %q: %w", path, err)
	}

	var spec AgentSpec
	dec := yaml.NewDecoder(&buf)
	dec.KnownFields(true)
	if err := dec.Decode(&spec); err != nil {
		return AgentSpec{}, fmt.Errorf("parse YAML %q: %w", path, err)
	}

	if spec.Deploy != nil && spec.Deploy.Grant != nil {
		resolvedGrant, err := resolveGrantConfig(spec.Deploy.Grant, envName)
		if err != nil {
			return AgentSpec{}, fmt.Errorf("validate YAML %q: grant: %w", path, err)
		}
		spec.Deploy.Grant = resolvedGrant
	}

	if err := validateAgentSpec(spec); err != nil {
		return AgentSpec{}, fmt.Errorf("validate YAML %q: %w", path, err)
	}

	return spec, nil
}

func isYAML(path string) bool {
	name := filepath.Base(path)
	if strings.HasPrefix(name, ".") {
		return false
	}
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}

func validateAgentSpec(spec AgentSpec) error {
	if strings.TrimSpace(spec.Name) == "" {
		return fmt.Errorf("name is required")
	}
	for i, tool := range spec.Tools {
		if len(tool.ToolSpec) == 0 {
			return fmt.Errorf("tools[%d].tool_spec is required", i)
		}
	}
	if spec.Deploy != nil && spec.Deploy.Grant != nil {
		if err := validateGrantConfig(spec.Deploy.Grant); err != nil {
			return fmt.Errorf("grant: %w", err)
		}
	}
	if spec.Eval != nil {
		for i, tc := range spec.Eval.Tests {
			if len(tc.ExpectedTools) == 0 && strings.TrimSpace(tc.Command) == "" && strings.TrimSpace(tc.ExpectedResponse) == "" {
				return fmt.Errorf("eval.tests[%d]: expected_tools, expected_response, or command is required", i)
			}
		}
	}
	return nil
}

func resolveGrantConfig(grant *GrantConfig, envName string) (*GrantConfig, error) {
	if grant == nil {
		return nil, nil
	}

	if len(grant.Envs) == 0 {
		return grant, nil
	}
	if len(grant.AccountRoles) > 0 || len(grant.DatabaseRoles) > 0 {
		return nil, fmt.Errorf("cannot mix flat grant fields with grant.envs")
	}

	if err := validateGrantEnvs(grant.Envs); err != nil {
		return nil, err
	}

	defaultGrant, hasDefault := grant.Envs["default"]
	if envName == "" {
		if !hasDefault {
			return nil, fmt.Errorf("envs.default is required when grant.envs is used without --env")
		}
		return grantConfigFromEnv(defaultGrant, GrantEnvConfig{}), nil
	}

	selectedGrant, hasSelected := grant.Envs[envName]
	if !hasSelected && !hasDefault {
		return nil, fmt.Errorf("grant.envs: environment %q not found and no default defined", envName)
	}

	return grantConfigFromEnv(selectedGrant, defaultGrant), nil
}

func validateGrantEnvs(envs map[string]GrantEnvConfig) error {
	for name, cfg := range envs {
		if err := validateGrantEnvConfig(cfg); err != nil {
			return fmt.Errorf("envs.%s: %w", name, err)
		}
	}
	return nil
}

func validateGrantEnvConfig(cfg GrantEnvConfig) error {
	if cfg.AccountRoles != nil {
		if err := validateRoleGrants(*cfg.AccountRoles, false, "account_roles"); err != nil {
			return err
		}
	}
	if cfg.DatabaseRoles != nil {
		if err := validateRoleGrants(*cfg.DatabaseRoles, true, "database_roles"); err != nil {
			return err
		}
	}
	return nil
}

func grantConfigFromEnv(selected, fallback GrantEnvConfig) *GrantConfig {
	return &GrantConfig{
		AccountRoles:  resolveGrantRoleList(selected.AccountRoles, fallback.AccountRoles),
		DatabaseRoles: resolveGrantRoleList(selected.DatabaseRoles, fallback.DatabaseRoles),
	}
}

func resolveGrantRoleList(selected, fallback *[]RoleGrant) []RoleGrant {
	if selected != nil {
		return cloneRoleGrants(*selected)
	}
	if fallback != nil {
		return cloneRoleGrants(*fallback)
	}
	return nil
}

func cloneRoleGrants(in []RoleGrant) []RoleGrant {
	if in == nil {
		return nil
	}
	out := make([]RoleGrant, len(in))
	copy(out, in)
	return out
}

func validateGrantConfig(grant *GrantConfig) error {
	if len(grant.Envs) > 0 {
		if len(grant.AccountRoles) > 0 || len(grant.DatabaseRoles) > 0 {
			return fmt.Errorf("cannot mix flat grant fields with grant.envs")
		}
		return validateGrantEnvs(grant.Envs)
	}

	if len(grant.AccountRoles) == 0 && len(grant.DatabaseRoles) == 0 {
		return nil
	}

	if err := validateRoleGrants(grant.AccountRoles, false, "account_roles"); err != nil {
		return err
	}
	if err := validateRoleGrants(grant.DatabaseRoles, true, "database_roles"); err != nil {
		return err
	}
	return nil
}

func validateRoleGrants(grants []RoleGrant, requireQualifiedRole bool, fieldName string) error {
	validPrivileges := map[string]bool{
		"USAGE": true, "MODIFY": true, "MONITOR": true, "ALL": true,
	}

	for i, rg := range grants {
		if strings.TrimSpace(rg.Role) == "" {
			return fmt.Errorf("%s[%d].role is required", fieldName, i)
		}
		if requireQualifiedRole && !strings.Contains(rg.Role, ".") {
			return fmt.Errorf("%s[%d].role: %q must be fully qualified (DB.ROLE_NAME)", fieldName, i, rg.Role)
		}
		if len(rg.Privileges) == 0 {
			return fmt.Errorf("%s[%d].privileges is required", fieldName, i)
		}
		for j, priv := range rg.Privileges {
			if !validPrivileges[strings.ToUpper(priv)] {
				return fmt.Errorf("%s[%d].privileges[%d]: invalid privilege %q (valid: USAGE, MODIFY, MONITOR, ALL)", fieldName, i, j, priv)
			}
		}
	}

	return nil
}
