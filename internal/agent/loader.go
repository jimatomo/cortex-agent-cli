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
func LoadAgents(path string, recursive bool) ([]ParsedAgent, error) {
	if strings.TrimSpace(path) == "" {
		path = "."
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat path %q: %w", path, err)
	}

	if info.IsDir() {
		return loadFromDir(path, recursive)
	}

	spec, err := loadFromFile(path)
	if err != nil {
		return nil, err
	}
	return []ParsedAgent{{Path: path, Spec: spec}}, nil
}

func loadFromDir(dir string, recursive bool) ([]ParsedAgent, error) {
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
		spec, err := loadFromFile(file)
		if err != nil {
			return nil, err
		}
		results = append(results, ParsedAgent{Path: file, Spec: spec})
	}
	return results, nil
}

func loadFromFile(path string) (AgentSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AgentSpec{}, fmt.Errorf("read file %q: %w", path, err)
	}

	var spec AgentSpec
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&spec); err != nil {
		return AgentSpec{}, fmt.Errorf("parse YAML %q: %w", path, err)
	}

	if err := validateAgentSpec(spec); err != nil {
		return AgentSpec{}, fmt.Errorf("validate YAML %q: %w", path, err)
	}

	return spec, nil
}

func isYAML(path string) bool {
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
	return nil
}
