package agent

import (
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

// VarsConfig maps environment names to variable key-value pairs.
// Example: {"dev": {"DB": "DEV_DB"}, "default": {"DB": "MY_DB"}}
type VarsConfig map[string]map[string]string

// varsWrapper is used in the first pass to extract only the vars section.
type varsWrapper struct {
	Vars VarsConfig `yaml:"vars"`
}

// varPattern matches ${ vars.VARIABLE_NAME } with optional whitespace.
var varPattern = regexp.MustCompile(`\$\{\s*vars\.(\w+)\s*\}`)

// resolveVars returns a flat map of variable values for the given environment.
// Resolution order:
//  1. If envName is specified, use that environment's values
//  2. Fall back to "default" for any missing keys
//  3. If envName is empty, use only "default"
//  4. Error if a required variable has no value in either group
func resolveVars(vars VarsConfig, envName string) (map[string]string, error) {
	if len(vars) == 0 {
		return nil, nil
	}

	resolved := make(map[string]string)
	defaultVars := vars["default"]

	if envName == "" {
		// Use only default group
		if defaultVars == nil {
			return nil, fmt.Errorf("vars: no 'default' environment defined and --env not specified")
		}
		for k, v := range defaultVars {
			resolved[k] = v
		}
		return resolved, nil
	}

	envVars := vars[envName]

	// Collect all variable names from both env and default
	allKeys := make(map[string]bool)
	for k := range envVars {
		allKeys[k] = true
	}
	for k := range defaultVars {
		allKeys[k] = true
	}

	if len(allKeys) == 0 {
		return nil, fmt.Errorf("vars: environment %q not found and no 'default' defined", envName)
	}

	for k := range allKeys {
		if v, ok := envVars[k]; ok {
			resolved[k] = v
		} else if v, ok := defaultVars[k]; ok {
			resolved[k] = v
		}
		// Both missing shouldn't happen since we iterated from those maps
	}

	return resolved, nil
}

// substituteVars recursively walks the yaml.Node tree and replaces
// ${ vars.XXX } references in scalar values with resolved values.
func substituteVars(node *yaml.Node, resolved map[string]string) error {
	if len(resolved) == 0 {
		return nil
	}
	return walkAndSubstitute(node, resolved)
}

func walkAndSubstitute(node *yaml.Node, resolved map[string]string) error {
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			if err := walkAndSubstitute(child, resolved); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		for i := 0; i < len(node.Content)-1; i += 2 {
			if err := walkAndSubstitute(node.Content[i+1], resolved); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			if err := walkAndSubstitute(child, resolved); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		if varPattern.MatchString(node.Value) {
			replaced, err := replaceVarRefs(node.Value, resolved)
			if err != nil {
				return err
			}
			node.Value = replaced
		}
	}
	return nil
}

// replaceVarRefs replaces all ${ vars.XXX } occurrences in a string.
func replaceVarRefs(s string, resolved map[string]string) (string, error) {
	var replaceErr error
	result := varPattern.ReplaceAllStringFunc(s, func(match string) string {
		if replaceErr != nil {
			return match
		}
		sub := varPattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		varName := sub[1]
		val, ok := resolved[varName]
		if !ok {
			replaceErr = fmt.Errorf("vars: undefined variable %q", varName)
			return match
		}
		return val
	})
	if replaceErr != nil {
		return "", replaceErr
	}
	return result, nil
}

// stripVarsNode removes the "vars" key from the top-level mapping node.
// This prevents KnownFields(true) from rejecting the vars section.
func stripVarsNode(doc *yaml.Node) {
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return
	}
	mapping := doc.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return
	}

	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == "vars" {
			// Remove key and value (2 elements)
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return
		}
	}
}
