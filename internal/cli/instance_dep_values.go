package cli

import (
	"fmt"
	"strings"

	"helmdex/internal/values"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newInstanceDepValuesCmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "values",
		Short: "Manage per-dependency overrides in values.instance.yaml (non-interactive)",
	}
	cmd.AddCommand(newInstanceDepValuesGetCmd(f))
	cmd.AddCommand(newInstanceDepValuesSetCmd(f))
	cmd.AddCommand(newInstanceDepValuesUnsetCmd(f))
	return cmd
}

func depValuesFullPath(depID string, rel string) (string, error) {
	depID = strings.TrimSpace(depID)
	if depID == "" {
		return "", fmt.Errorf("depID is required")
	}
	rel = strings.TrimSpace(rel)
	if rel == "" {
		rel = "$"
	}
	if !strings.HasPrefix(rel, "$") {
		return "", fmt.Errorf("path must start with '$' (got %q)", rel)
	}
	// Join: $.<depID> + suffix ("" or ".x" or "[0]...")
	suffix := strings.TrimPrefix(rel, "$")
	return "$." + depID + suffix, nil
}

func newInstanceDepValuesGetCmd(f *rootFlags) *cobra.Command {
	var path string
	var format string
	cmd := &cobra.Command{
		Use:   "get <instance> <depID>",
		Short: "Get a per-dependency override value",
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
			root, err := values.ReadInstanceValues(inst.Path)
			if err != nil {
				return err
			}
			full, err := depValuesFullPath(args[1], path)
			if err != nil {
				return err
			}
			p, err := values.ParsePath(full)
			if err != nil {
				return err
			}
			v, ok := values.GetAt(root, p)
			if !ok {
				return fmt.Errorf("path not found: %s", full)
			}
			ff := parseFormat(format, formatJSON)
			if ff == formatTable {
				b, err := yaml.Marshal(v)
				if err != nil {
					return err
				}
				_, _ = cmd.OutOrStdout().Write(b)
				return nil
			}
			return writeJSON(cmd.OutOrStdout(), v)
		},
	}
	cmd.Flags().StringVar(&path, "path", "$", "Path relative to dep override root, e.g. '$.replicaCount' (defaults to '$')")
	cmd.Flags().StringVar(&format, "format", string(formatJSON), "Output format: json|table")
	return cmd
}

func newInstanceDepValuesSetCmd(f *rootFlags) *cobra.Command {
	var path string
	var valueYAML string
	var valueJSON string
	var regen bool
	cmd := &cobra.Command{
		Use:   "set <instance> <depID>",
		Short: "Set a per-dependency override value",
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
			v, err := loadValueFromFlags(valueYAML, valueJSON)
			if err != nil {
				return err
			}
			root, err := values.ReadInstanceValues(inst.Path)
			if err != nil {
				return err
			}
			full, err := depValuesFullPath(args[1], path)
			if err != nil {
				return err
			}
			p, err := values.ParsePath(full)
			if err != nil {
				return err
			}
			newRootAny := values.SetAt(root, p, v)
			newRoot, _ := newRootAny.(map[string]any)
			if newRoot == nil {
				newRoot = map[string]any{}
			}
			if err := values.WriteInstanceValues(inst.Path, newRoot); err != nil {
				return err
			}
			if regen {
				return values.GenerateMergedValues(inst.Path)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "Path relative to dep override root (required), e.g. '$.replicaCount'")
	cmd.Flags().StringVar(&valueYAML, "value-yaml", "", "Value as YAML")
	cmd.Flags().StringVar(&valueJSON, "value-json", "", "Value as JSON")
	cmd.Flags().BoolVar(&regen, "regen", true, "Regenerate values.yaml after write")
	_ = cmd.MarkFlagRequired("path")
	return cmd
}

func newInstanceDepValuesUnsetCmd(f *rootFlags) *cobra.Command {
	var path string
	var regen bool
	cmd := &cobra.Command{
		Use:   "unset <instance> <depID>",
		Short: "Unset (delete) a per-dependency override value",
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
			root, err := values.ReadInstanceValues(inst.Path)
			if err != nil {
				return err
			}
			full, err := depValuesFullPath(args[1], path)
			if err != nil {
				return err
			}
			p, err := values.ParsePath(full)
			if err != nil {
				return err
			}
			newRootAny := values.SetAt(root, p, nil)
			newRoot, _ := newRootAny.(map[string]any)
			if newRoot == nil {
				newRoot = map[string]any{}
			}
			if err := values.WriteInstanceValues(inst.Path, newRoot); err != nil {
				return err
			}
			if regen {
				return values.GenerateMergedValues(inst.Path)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "Path relative to dep override root (required); use '$' to delete the entire dep override")
	cmd.Flags().BoolVar(&regen, "regen", true, "Regenerate values.yaml after write")
	_ = cmd.MarkFlagRequired("path")
	return cmd
}
