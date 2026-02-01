package diff

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"

	"coragent/internal/agent"
)

type ChangeType string

const (
	Added    ChangeType = "ADDED"
	Removed  ChangeType = "REMOVED"
	Modified ChangeType = "MODIFIED"
)

type Change struct {
	Path   string
	Type   ChangeType
	Before any
	After  any
}

type Options struct {
	IgnoreMissingRemote bool
}

func Diff(local, remote agent.AgentSpec) ([]Change, error) {
	return DiffWithOptions(local, remote, Options{})
}

// DiffForCreate returns changes representing a new resource creation.
// All non-empty fields in the spec are shown as Added changes.
func DiffForCreate(spec agent.AgentSpec) ([]Change, error) {
	specMap, err := ToMap(spec)
	if err != nil {
		return nil, err
	}
	var changes []Change
	collectAdded("", specMap, &changes)
	return changes, nil
}

// collectAdded recursively collects all non-nil values as Added changes.
func collectAdded(path string, value any, changes *[]Change) {
	if value == nil {
		return
	}
	switch v := value.(type) {
	case map[string]any:
		if len(v) == 0 {
			return
		}
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		// Use API field order for top-level keys, alphabetical for nested
		if path == "" {
			sortAgentKeys(keys)
		} else {
			sort.Strings(keys)
		}
		for _, k := range keys {
			nextPath := joinPath(path, k)
			collectAdded(nextPath, v[k], changes)
		}
	case []any:
		if len(v) == 0 {
			return
		}
		for i, item := range v {
			nextPath := fmt.Sprintf("%s[%d]", path, i)
			collectAdded(nextPath, item, changes)
		}
	default:
		*changes = append(*changes, Change{Path: path, Type: Added, Before: value, After: nil})
	}
}

func DiffWithOptions(local, remote agent.AgentSpec, opts Options) ([]Change, error) {
	localMap, err := ToMap(local)
	if err != nil {
		return nil, err
	}
	remoteMap, err := ToMap(remote)
	if err != nil {
		return nil, err
	}

	var changes []Change
	diffAny("", localMap, remoteMap, &changes, opts)
	return changes, nil
}

func HasChanges(changes []Change) bool {
	return len(changes) > 0
}

// ToMap converts an AgentSpec to a map for comparison.
func ToMap(spec agent.AgentSpec) (map[string]any, error) {
	data, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("marshal spec: %w", err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("unmarshal spec: %w", err)
	}
	return out, nil
}

func diffAny(path string, local, remote any, changes *[]Change, opts Options) {
	if local == nil && remote == nil {
		return
	}
	if local == nil {
		// Remote has value, local doesn't -> removing the field
		*changes = append(*changes, Change{Path: path, Type: Removed, Before: remote, After: nil})
		return
	}
	if remote == nil {
		if opts.IgnoreMissingRemote {
			return
		}
		// Local has value, remote doesn't -> adding the field
		*changes = append(*changes, Change{Path: path, Type: Added, Before: nil, After: local})
		return
	}

	switch l := local.(type) {
	case map[string]any:
		r, ok := remote.(map[string]any)
		if !ok {
			*changes = append(*changes, Change{Path: path, Type: Modified, Before: remote, After: local})
			return
		}
		keys := uniqueKeys(l, r)
		if opts.IgnoreMissingRemote {
			keys = keysOf(r)
		}
		// Use API field order for top-level keys, alphabetical for nested
		if path == "" {
			sortAgentKeys(keys)
		} else {
			sort.Strings(keys)
		}
		for _, k := range keys {
			nextPath := joinPath(path, k)
			diffAny(nextPath, l[k], r[k], changes, opts)
		}
	case []any:
		r, ok := remote.([]any)
		if !ok {
			*changes = append(*changes, Change{Path: path, Type: Modified, Before: local, After: remote})
			return
		}
		maxLen := len(l)
		if len(r) > maxLen {
			maxLen = len(r)
		}
		for i := 0; i < maxLen; i++ {
			var lv any
			var rv any
			if i < len(l) {
				lv = l[i]
			}
			if i < len(r) {
				rv = r[i]
			}
			nextPath := fmt.Sprintf("%s[%s]", path, strconv.Itoa(i))
			diffAny(nextPath, lv, rv, changes, opts)
		}
	default:
		if !reflect.DeepEqual(local, remote) {
			*changes = append(*changes, Change{Path: path, Type: Modified, Before: remote, After: local})
		}
	}
}

func uniqueKeys(a, b map[string]any) []string {
	keys := make(map[string]struct{})
	for k := range a {
		keys[k] = struct{}{}
	}
	for k := range b {
		keys[k] = struct{}{}
	}
	out := make([]string, 0, len(keys))
	for k := range keys {
		out = append(out, k)
	}
	return out
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func joinPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// agentFieldOrder defines the preferred field order based on Snowflake Cortex Agents REST API documentation.
var agentFieldOrder = map[string]int{
	"name":           0,
	"comment":        1,
	"profile":        2,
	"models":         3,
	"instructions":   4,
	"orchestration":  5,
	"tools":          6,
	"tool_resources": 7,
}

// sortAgentKeys sorts keys according to the Snowflake Cortex Agents REST API field order.
// Keys not in the predefined order are sorted alphabetically and placed at the end.
func sortAgentKeys(keys []string) {
	sort.Slice(keys, func(i, j int) bool {
		oi, oki := agentFieldOrder[keys[i]]
		oj, okj := agentFieldOrder[keys[j]]
		if oki && okj {
			return oi < oj
		}
		if oki {
			return true
		}
		if okj {
			return false
		}
		return keys[i] < keys[j]
	})
}
