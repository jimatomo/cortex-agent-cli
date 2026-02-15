package cli

import (
	"context"
	"bytes"
	"fmt"
	"os"

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

			spec, exists, err := client.GetAgent(context.Background(), target.Database, target.Schema, name)
			if err != nil {
				return err
			}
			if !exists {
				return fmt.Errorf("agent %q not found", name)
			}

			var buf bytes.Buffer
			enc := yaml.NewEncoder(&buf)
			enc.SetIndent(2)
			if err := enc.Encode(spec); err != nil {
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

