package cli

import (
	"fmt"
	"strings"

	"helmdex/internal/presets"
	"helmdex/internal/yamlchart"

	"github.com/spf13/cobra"
)

func newInstancePresetsCmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "presets",
		Short: "Inspect preset resolution (non-interactive)",
	}
	cmd.AddCommand(newInstancePresetsResolveCmd(f))
	cmd.AddCommand(newInstanceDepPresetsResolveCmd(f))
	return cmd
}

func newInstancePresetsResolveCmd(f *rootFlags) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "resolve <instance>",
		Short: "Resolve preset file paths for all dependencies in an instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, _, cfg, err := resolveRepoAndConfig(f)
			if err != nil {
				return err
			}
			inst, err := resolveInstanceByName(repoRoot, cfg, args[0])
			if err != nil {
				return err
			}
			chart, err := readInstanceChart(inst)
			if err != nil {
				return err
			}
			res, err := presets.Resolve(repoRoot, cfg, chart.Dependencies)
			if err != nil {
				return err
			}
			ff := parseFormat(format, formatJSON)
			if ff == formatTable {
				// compact table: depID, default, platform, sets...
				for id, rd := range res.ByID {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", id, rd.DefaultPath, rd.PlatformPath)
				}
				return nil
			}
			return writeJSON(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().StringVar(&format, "format", string(formatJSON), "Output format: json|table")
	return cmd
}

func newInstanceDepPresetsResolveCmd(f *rootFlags) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "resolve-dep <instance> <depID>",
		Short: "Resolve preset file paths for one dependency in an instance",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, _, cfg, err := resolveRepoAndConfig(f)
			if err != nil {
				return err
			}
			inst, err := resolveInstanceByName(repoRoot, cfg, args[0])
			if err != nil {
				return err
			}
			chart, err := readInstanceChart(inst)
			if err != nil {
				return err
			}
			res, err := presets.Resolve(repoRoot, cfg, chart.Dependencies)
			if err != nil {
				return err
			}
			want := yamlchart.DepID(strings.TrimSpace(args[1]))
			rd, ok := res.ByID[want]
			if !ok {
				return fmt.Errorf("dependency %q not found in preset resolution", want)
			}
			ff := parseFormat(format, formatJSON)
			if ff == formatTable {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", want, rd.DefaultPath, rd.PlatformPath)
				return nil
			}
			return writeJSON(cmd.OutOrStdout(), rd)
		},
	}
	cmd.Flags().StringVar(&format, "format", string(formatJSON), "Output format: json|table")
	return cmd
}
