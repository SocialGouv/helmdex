package cli

import (
	"context"
	"fmt"

	"helmdex/internal/instances"
	"helmdex/internal/yamlchart"

	"github.com/spf13/cobra"
)

func newInstanceDepInspectCmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect dependency artifacts (readme/default values/schema)",
	}
	cmd.AddCommand(newInstanceDepInspectReadmeCmd(f))
	cmd.AddCommand(newInstanceDepInspectValuesCmd(f))
	cmd.AddCommand(newInstanceDepInspectSchemaCmd(f))
	return cmd
}

func depByIDOrErr(chart yamlchart.Chart, id string) (yamlchart.Dependency, error) {
	return instances.DepByID(chart, id)
}

type depInspectKind = instances.InspectKind

const (
	kindReadme = instances.InspectReadme
	kindValues = instances.InspectValues
	kindSchema = instances.InspectSchema
)

func loadDepInspectContent(ctx context.Context, repoRoot string, instPath string, dep yamlchart.Dependency, kind depInspectKind) (string, error) {
	return instances.LoadDepInspectContent(ctx, repoRoot, instPath, dep, kind)
}

func newInstanceDepInspectReadmeCmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "readme <instance> <depID>",
		Short: "Show dependency README.md (best effort, cached)",
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
			dep, err := depByIDOrErr(chart, args[1])
			if err != nil {
				return err
			}
			s, err := loadDepInspectContent(cmd.Context(), repoRoot, inst.Path, dep, kindReadme)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprint(cmd.OutOrStdout(), s)
			return nil
		},
	}
	return cmd
}

func newInstanceDepInspectValuesCmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "values <instance> <depID>",
		Short: "Show dependency default values.yaml (best effort, cached)",
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
			dep, err := depByIDOrErr(chart, args[1])
			if err != nil {
				return err
			}
			s, err := loadDepInspectContent(cmd.Context(), repoRoot, inst.Path, dep, kindValues)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprint(cmd.OutOrStdout(), s)
			return nil
		},
	}
	return cmd
}

func newInstanceDepInspectSchemaCmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema <instance> <depID>",
		Short: "Show dependency values.schema.json (best effort, cached)",
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
			dep, err := depByIDOrErr(chart, args[1])
			if err != nil {
				return err
			}
			s, err := loadDepInspectContent(cmd.Context(), repoRoot, inst.Path, dep, kindSchema)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprint(cmd.OutOrStdout(), s)
			return nil
		},
	}
	return cmd
}
