package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"coragent/internal/api"
	"coragent/internal/auth"

	"gopkg.in/yaml.v3"

	"github.com/spf13/cobra"
)

func newExportCmd(opts *RootOptions) *cobra.Command {
	var outPath string
	cmd := &cobra.Command{
		Use:   "export <agent-name>",
		Short: "Export existing agent to YAML",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			cfg := auth.LoadConfig(opts.Connection)
			applyAuthOverrides(&cfg, opts)

			client, err := api.NewClientWithDebug(cfg, opts.Debug)
			if err != nil {
				return err
			}

			target, err := ResolveTargetForExport(opts, cfg)
			if err != nil {
				return err
			}

			result, err := client.DescribeAgent(context.Background(), target.Database, target.Schema, name)
			if err != nil {
				return err
			}
			if !result.Exists {
				return fmt.Errorf("agent %q not found", name)
			}
			spec := result.Spec
			for _, col := range result.UnmappedColumns {
				fmt.Fprintf(os.Stderr, "\033[33mWarning: DESCRIBE AGENT returned unmapped column %q (not exported)\033[0m\n", col)
			}
			for _, key := range result.UnmappedSpecKeys {
				fmt.Fprintf(os.Stderr, "\033[33mWarning: agent_spec contains unmapped key %q (not exported)\033[0m\n", key)
			}

			var doc yaml.Node
			if err := doc.Encode(spec); err != nil {
				return fmt.Errorf("marshal YAML: %w", err)
			}
			setLiteralStyleForMultiline(&doc)
			reorderExportKeys(&doc)

			var buf bytes.Buffer
			enc := yaml.NewEncoder(&buf)
			enc.SetIndent(2)
			if err := enc.Encode(&doc); err != nil {
				return fmt.Errorf("marshal YAML: %w", err)
			}
			enc.Close()
			data := buf.Bytes()

			if outPath == "" {
				_, err = cmd.OutOrStdout().Write(data)
				return err
			}

			if err := os.WriteFile(outPath, data, 0o644); err != nil {
				return fmt.Errorf("write %q: %w", outPath, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "exported to %s\n", outPath)
			return nil
		},
	}
	cmd.Flags().StringVarP(&outPath, "out", "o", "", "Output file path (default: stdout)")
	return cmd
}

// setLiteralStyleForMultiline walks a yaml.Node tree and sets LiteralStyle
// on scalar nodes whose value contains newlines, producing "|" block syntax.
func setLiteralStyleForMultiline(node *yaml.Node) {
	if node == nil {
		return
	}
	if node.Kind == yaml.ScalarNode && strings.Contains(node.Value, "\n") {
		node.Style = yaml.LiteralStyle
	}
	for _, child := range node.Content {
		setLiteralStyleForMultiline(child)
	}
}

// reorderExportKeys reorders map keys in the YAML node tree so that
// tool_spec keys appear as name, type, description first and
// tool_resources entries have semantic_view / search_service first.
func reorderExportKeys(node *yaml.Node) {
	if node == nil {
		return
	}
	// Unwrap document node.
	if node.Kind == yaml.DocumentNode {
		for _, child := range node.Content {
			reorderExportKeys(child)
		}
		return
	}
	if node.Kind != yaml.MappingNode {
		for _, child := range node.Content {
			reorderExportKeys(child)
		}
		return
	}

	// Iterate key-value pairs to find tool_spec and tool_resources mappings.
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valNode := node.Content[i+1]

		if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "tool_spec" && valNode.Kind == yaml.MappingNode {
			reorderMappingKeys(valNode, []string{"name", "type", "description"})
		}

		if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "tool_resources" && valNode.Kind == yaml.MappingNode {
			// Each child of tool_resources is a tool name → resource config mapping.
			for j := 0; j+1 < len(valNode.Content); j += 2 {
				resVal := valNode.Content[j+1]
				if resVal.Kind == yaml.MappingNode {
					reorderMappingKeys(resVal, []string{"semantic_view", "search_service"})
				}
			}
		}

		// Recurse into value nodes.
		reorderExportKeys(valNode)
	}
}

// reorderMappingKeys moves the specified keys to the front of a mapping node,
// preserving their relative order. Keys not in the priority list keep their
// original order after the prioritized keys.
func reorderMappingKeys(node *yaml.Node, priority []string) {
	if node.Kind != yaml.MappingNode || len(node.Content) < 4 {
		return
	}

	type pair struct {
		key *yaml.Node
		val *yaml.Node
	}

	pairs := make([]pair, 0, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		pairs = append(pairs, pair{node.Content[i], node.Content[i+1]})
	}

	// Build index of priority keys.
	priorityIndex := make(map[string]int, len(priority))
	for i, k := range priority {
		priorityIndex[k] = i
	}

	// Split into priority and rest.
	priorityPairs := make([]pair, len(priority))
	found := make([]bool, len(priority))
	var rest []pair

	for _, p := range pairs {
		if idx, ok := priorityIndex[p.key.Value]; ok {
			priorityPairs[idx] = p
			found[idx] = true
		} else {
			rest = append(rest, p)
		}
	}

	// Rebuild: priority keys first (only those that exist), then rest.
	result := make([]*yaml.Node, 0, len(node.Content))
	for i, p := range priorityPairs {
		if found[i] {
			result = append(result, p.key, p.val)
		}
	}
	for _, p := range rest {
		result = append(result, p.key, p.val)
	}
	node.Content = result
}

