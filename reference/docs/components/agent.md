# Agent Component

The agent package handles YAML spec loading, variable substitution, and validation.

## Key Files

- `internal/agent/loader.go` — `LoadAgents`, `ParsedAgent`, `loadFromFile`, `loadFromDir`
- `internal/agent/agent.go` — `AgentSpec`, `DeployConfig`, `EvalConfig`, struct definitions
- `internal/agent/vars.go` — `resolveVars`, `substituteVars`, vars/env substitution
- `internal/agent/validate.go` — `validateAgentSpec`, `validateGrantConfig`

## LoadAgents

```go
func LoadAgents(path string, recursive bool, envName string) ([]ParsedAgent, error)
```

- **path:** File or directory; `""` or `"."` → current directory
- **recursive:** If directory, walk subdirs for YAML files
- **envName:** Selects vars group (e.g., `--env prod` → `vars.prod`)

## Parsing Pipeline

1. **Read file** — `os.ReadFile(path)`
2. **Extract vars** — Parse with `varsWrapper` to get `vars` section
3. **Resolve vars** — `resolveVars(wrapper.Vars, envName)` → map of key→value (vars.default fallback)
4. **Parse YAML node** — `yaml.Unmarshal` into `yaml.Node` tree
5. **Strip vars node** — Remove vars from tree before KnownFields check
6. **Substitute** — `substituteVars(&doc, resolved)` replaces `${ vars.KEY }` and `${ env.KEY }`
7. **Re-encode and decode** — Encode node to bytes, decode with `KnownFields(true)` into `AgentSpec`
8. **Validate** — `validateAgentSpec(spec)`

## Variable Substitution

- `${ vars.KEY }` — From vars section; env selected by `--env`; fallback to `vars.default`
- `${ env.KEY }` — From OS environment at runtime

Both can appear in any scalar; partial strings and multiple refs in one value are supported.

## Validation Rules

Enforced by `AgentSpec.Validate()` in `internal/agent/validate.go`:

- `name` must not be empty
- `tools[i].tool_spec` must not be empty and must contain a non-empty `name` field
- `tool_resources` keys must match at least one tool name in `tools` (when both are present)
- `eval.tests[i].question` is required for each test case
- `eval.response_score_threshold` must be between 0 and 100
- `deploy.grant.account_roles[i]` / `database_roles[i]` — `role` required, `privileges` must not be empty

## Related Docs

- [flows/plan-apply-flow.md](../flows/plan-apply-flow.md) — Load step in plan/apply
- [reference/yaml-spec.md](../../yaml-spec.md) — User-facing YAML reference
