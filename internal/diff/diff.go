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

func DiffWithOptions(local, remote agent.AgentSpec, opts Options) ([]Change, error) {
	localMap, err := toMap(local)
	if err != nil {
		return nil, err
	}
	remoteMap, err := toMap(remote)
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

func toMap(spec agent.AgentSpec) (map[string]any, error) {
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
		*changes = append(*changes, Change{Path: path, Type: Added, Before: nil, After: remote})
		return
	}
	if remote == nil {
		if opts.IgnoreMissingRemote {
			return
		}
		*changes = append(*changes, Change{Path: path, Type: Removed, Before: local, After: nil})
		return
	}

	switch l := local.(type) {
	case map[string]any:
		r, ok := remote.(map[string]any)
		if !ok {
			*changes = append(*changes, Change{Path: path, Type: Modified, Before: local, After: remote})
			return
		}
		keys := uniqueKeys(l, r)
		if opts.IgnoreMissingRemote {
			keys = keysOf(r)
		}
		sort.Strings(keys)
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
			*changes = append(*changes, Change{Path: path, Type: Modified, Before: local, After: remote})
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
