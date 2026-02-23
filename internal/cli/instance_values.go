package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"helmdex/internal/config"
	"helmdex/internal/instances"
	"helmdex/internal/repo"
	"helmdex/internal/values"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newInstanceValuesCmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "values",
		Short: "Manage instance values files (non-interactive)",
	}
	cmd.AddCommand(newInstanceValuesGetCmd(f))
	cmd.AddCommand(newInstanceValuesSetCmd(f))
	cmd.AddCommand(newInstanceValuesUnsetCmd(f))
	cmd.AddCommand(newInstanceValuesReplaceCmd(f))
	cmd.AddCommand(newInstanceValuesRegenCmd(f))
	return cmd
}

func resolveInstance(repoRoot, cfgPath, instanceName string) (instances.Instance, config.Config, error) {
	cfg, err := config.LoadFile(cfgPath)
	if err != nil {
		return instances.Instance{}, config.Config{}, err
	}
	inst, err := instances.Get(repoRoot, cfg.Repo.AppsDir, instanceName)
	if err != nil {
		return instances.Instance{}, config.Config{}, err
	}
	return inst, cfg, nil
}

func loadValueFromFlags(valueYAML, valueJSON string) (any, error) {
	if strings.TrimSpace(valueYAML) != "" && strings.TrimSpace(valueJSON) != "" {
		return nil, fmt.Errorf("only one of --value-yaml or --value-json may be set")
	}
	if strings.TrimSpace(valueYAML) != "" {
		var v any
		if err := yaml.Unmarshal([]byte(valueYAML), &v); err != nil {
			// allow scalar without newline
			return nil, fmt.Errorf("parse --value-yaml: %w", err)
		}
		return v, nil
	}
	if strings.TrimSpace(valueJSON) != "" {
		var v any
		dec := json.NewDecoder(strings.NewReader(valueJSON))
		dec.UseNumber()
		if err := dec.Decode(&v); err != nil {
			return nil, fmt.Errorf("parse --value-json: %w", err)
		}
		return v, nil
	}
	return nil, fmt.Errorf("one of --value-yaml or --value-json is required")
}

func readFromStdin() ([]byte, error) {
	// Support piping large YAML docs.
	return io.ReadAll(os.Stdin)
}

func newInstanceValuesGetCmd(f *rootFlags) *cobra.Command {
	var path string
	var format string
	cmd := &cobra.Command{
		Use:   "get <instance>",
		Short: "Get a value from values.instance.yaml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := repo.ResolveRoot(f.RepoRoot)
			if err != nil {
				return err
			}
			cfgPath := f.Config
			if cfgPath == "" {
				cfgPath = filepath.Join(repoRoot, "helmdex.yaml")
			}
			inst, _, err := resolveInstance(repoRoot, cfgPath, args[0])
			if err != nil {
				return err
			}
			root, err := values.ReadInstanceValues(inst.Path)
			if err != nil {
				return err
			}
			p, err := values.ParsePath(path)
			if err != nil {
				return err
			}
			v, ok := values.GetAt(root, p)
			if !ok {
				return fmt.Errorf("path not found: %s", path)
			}
			ff := parseFormat(format, formatJSON)
			switch ff {
			case formatTable, formatPlain:
				// YAML output for humans.
				b, err := yaml.Marshal(v)
				if err != nil {
					return err
				}
				_, _ = cmd.OutOrStdout().Write(b)
				return nil
			default:
				return writeJSON(cmd.OutOrStdout(), v)
			}
		},
	}
	cmd.Flags().StringVar(&path, "path", "$", "Path in TUI syntax, e.g. '$.foo.bar[0]' (defaults to '$')")
	cmd.Flags().StringVar(&format, "format", string(formatJSON), "Output format: json|table")
	return cmd
}

func newInstanceValuesSetCmd(f *rootFlags) *cobra.Command {
	var path string
	var valueYAML string
	var valueJSON string
	var regen bool

	cmd := &cobra.Command{
		Use:   "set <instance>",
		Short: "Set a value in values.instance.yaml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := repo.ResolveRoot(f.RepoRoot)
			if err != nil {
				return err
			}
			cfgPath := f.Config
			if cfgPath == "" {
				cfgPath = filepath.Join(repoRoot, "helmdex.yaml")
			}
			inst, _, err := resolveInstance(repoRoot, cfgPath, args[0])
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
			p, err := values.ParsePath(path)
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
				if err := values.GenerateMergedValues(inst.Path); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "Path in TUI syntax, e.g. '$.foo.bar[0]' (required)")
	cmd.Flags().StringVar(&valueYAML, "value-yaml", "", "Value as YAML (scalar, object, or array)")
	cmd.Flags().StringVar(&valueJSON, "value-json", "", "Value as JSON")
	cmd.Flags().BoolVar(&regen, "regen", true, "Regenerate values.yaml after write")
	_ = cmd.MarkFlagRequired("path")
	return cmd
}

func newInstanceValuesUnsetCmd(f *rootFlags) *cobra.Command {
	var path string
	var regen bool
	cmd := &cobra.Command{
		Use:   "unset <instance>",
		Short: "Unset (delete) a value in values.instance.yaml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := repo.ResolveRoot(f.RepoRoot)
			if err != nil {
				return err
			}
			cfgPath := f.Config
			if cfgPath == "" {
				cfgPath = filepath.Join(repoRoot, "helmdex.yaml")
			}
			inst, _, err := resolveInstance(repoRoot, cfgPath, args[0])
			if err != nil {
				return err
			}
			root, err := values.ReadInstanceValues(inst.Path)
			if err != nil {
				return err
			}
			p, err := values.ParsePath(path)
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
				if err := values.GenerateMergedValues(inst.Path); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "Path in TUI syntax, e.g. '$.foo.bar[0]' (required)")
	cmd.Flags().BoolVar(&regen, "regen", true, "Regenerate values.yaml after write")
	_ = cmd.MarkFlagRequired("path")
	return cmd
}

func newInstanceValuesReplaceCmd(f *rootFlags) *cobra.Command {
	var fromFile string
	var stdin bool
	var regen bool
	cmd := &cobra.Command{
		Use:   "replace <instance>",
		Short: "Replace values.instance.yaml with provided YAML (from --file or --stdin)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := repo.ResolveRoot(f.RepoRoot)
			if err != nil {
				return err
			}
			cfgPath := f.Config
			if cfgPath == "" {
				cfgPath = filepath.Join(repoRoot, "helmdex.yaml")
			}
			inst, _, err := resolveInstance(repoRoot, cfgPath, args[0])
			if err != nil {
				return err
			}
			if (strings.TrimSpace(fromFile) == "") == (!stdin) {
				return fmt.Errorf("exactly one of --file or --stdin is required")
			}
			var b []byte
			if stdin {
				b, err = readFromStdin()
			} else {
				b, err = os.ReadFile(fromFile)
			}
			if err != nil {
				return err
			}
			// Validate YAML parses into an object.
			var root any
			dec := yaml.NewDecoder(bytes.NewReader(b))
			if err := dec.Decode(&root); err != nil {
				return fmt.Errorf("invalid YAML: %w", err)
			}
			obj, ok := root.(map[string]any)
			if !ok {
				return fmt.Errorf("values.instance.yaml root must be a YAML mapping")
			}
			if err := values.WriteInstanceValues(inst.Path, obj); err != nil {
				return err
			}
			if regen {
				if err := values.GenerateMergedValues(inst.Path); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&fromFile, "file", "", "Read YAML from file")
	cmd.Flags().BoolVar(&stdin, "stdin", false, "Read YAML from stdin")
	cmd.Flags().BoolVar(&regen, "regen", true, "Regenerate values.yaml after write")
	return cmd
}

func newInstanceValuesRegenCmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "regen <instance>",
		Short: "Regenerate merged values.yaml for an instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := repo.ResolveRoot(f.RepoRoot)
			if err != nil {
				return err
			}
			cfgPath := f.Config
			if cfgPath == "" {
				cfgPath = filepath.Join(repoRoot, "helmdex.yaml")
			}
			inst, _, err := resolveInstance(repoRoot, cfgPath, args[0])
			if err != nil {
				return err
			}
			return values.GenerateMergedValues(inst.Path)
		},
	}
	return cmd
}
