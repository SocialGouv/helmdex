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

// depValuesPath builds the values path for a per-dependency override:
// $.<depID><rel>. The depID is added as a LITERAL map key via Path.Child —
// never spliced into a string and re-parsed — so a dependency whose id/alias
// contains '.' or '[' targets the correct top-level key instead of being
// mis-split into a nested path (which would silently write to, and clobber,
// the wrong location). Only the caller-supplied rel goes through ParsePath.
func depValuesPath(depID string, rel string) (values.Path, error) {
	depID = strings.TrimSpace(depID)
	if depID == "" {
		return nil, fmt.Errorf("depID is required")
	}
	rel = strings.TrimSpace(rel)
	if rel == "" {
		rel = "$"
	}
	sub, err := values.ParsePath(rel)
	if err != nil {
		return nil, err
	}
	return append(values.Path{}.Child(depID), sub...), nil
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
			p, err := depValuesPath(args[1], path)
			if err != nil {
				return err
			}
			v, ok := values.GetAt(root, p)
			if !ok {
				return fmt.Errorf("path not found: dep %q %s", args[1], path)
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
			p, err := depValuesPath(args[1], path)
			if err != nil {
				return err
			}
			if err := values.SetInFile(values.EditFilePath(inst.Path), p, v); err != nil {
				return err
			}
			if regen {
				return values.GenerateIfManaged(inst.Path)
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
			p, err := depValuesPath(args[1], path)
			if err != nil {
				return err
			}
			if err := values.SetInFile(values.EditFilePath(inst.Path), p, nil); err != nil {
				return err
			}
			if regen {
				return values.GenerateIfManaged(inst.Path)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "Path relative to dep override root (required); use '$' to delete the entire dep override")
	cmd.Flags().BoolVar(&regen, "regen", true, "Regenerate values.yaml after write")
	_ = cmd.MarkFlagRequired("path")
	return cmd
}
