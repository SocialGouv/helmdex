package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"helmdex/internal/catalog"
	"helmdex/internal/config"
	"helmdex/internal/depmeta"
	"helmdex/internal/presets"
	"helmdex/internal/values"
	"helmdex/internal/yamlchart"

	"github.com/spf13/cobra"
)

// Per-dependency source metadata lives in internal/depmeta (shared with
// the TUI and server).

const (
	depSourceCatalog   = depmeta.KindCatalog
	depSourceArbitrary = depmeta.KindArbitrary
)

type depSourceMeta = depmeta.Meta

func readDepSourceMeta(repoRoot, instanceName string, depID yamlchart.DepID) (depSourceMeta, bool) {
	return depmeta.Read(repoRoot, instanceName, depID)
}

func writeDepSourceMeta(repoRoot, instanceName string, depID yamlchart.DepID, meta depSourceMeta) error {
	return depmeta.Write(repoRoot, instanceName, depID, meta)
}

func removeOrphanDepSetMarkers(instancePath string, depID yamlchart.DepID, allowedSets map[string]struct{}) error {
	glob := filepath.Join(instancePath, fmt.Sprintf("values.dep-set.%s--*.yaml", depID))
	files, err := filepath.Glob(glob)
	if err != nil {
		return err
	}
	for _, f := range files {
		base := filepath.Base(f)
		name := strings.TrimSuffix(strings.TrimPrefix(base, "values.dep-set."), ".yaml")
		parts := strings.SplitN(name, "--", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[0]) != string(depID) {
			continue
		}
		setName := strings.TrimSpace(parts[1])
		if setName == "" {
			continue
		}
		if allowedSets != nil {
			if _, ok := allowedSets[setName]; !ok {
				_ = os.Remove(f)
			}
		}
	}
	return nil
}

func newInstanceDepDetachCmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "detach <instance> <depID>",
		Short: "Detach a catalog-attached dependency (switch depmeta kind to arbitrary)",
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
			wantID := yamlchart.DepID(strings.TrimSpace(args[1]))
			depExists := false
			for _, d := range chart.Dependencies {
				if yamlchart.DependencyID(d) == wantID {
					depExists = true
					break
				}
			}
			if !depExists {
				return fmt.Errorf("dependency %q not found in instance %q", wantID, args[0])
			}

			meta, ok := readDepSourceMeta(repoRoot, inst.Name, wantID)
			if !ok || meta.Kind != depSourceCatalog {
				return fmt.Errorf("dependency %q is not catalog-attached", wantID)
			}
			if err := writeDepSourceMeta(repoRoot, inst.Name, wantID, depSourceMeta{Kind: depSourceArbitrary}); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Detached %s from catalog in %s\n", wantID, args[0])
			return nil
		},
	}
	return cmd
}

func newInstanceDepSyncPresetsCmd(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync-presets <instance> <depID>",
		Short: "Sync preset cache and regenerate values for one dependency",
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
			wantID := yamlchart.DepID(strings.TrimSpace(args[1]))
			var dep *yamlchart.Dependency
			for i := range chart.Dependencies {
				if yamlchart.DependencyID(chart.Dependencies[i]) == wantID {
					dep = &chart.Dependencies[i]
					break
				}
			}
			if dep == nil {
				return fmt.Errorf("dependency %q not found in instance %q", wantID, args[0])
			}

			// Sync only sources that have presets enabled.
			s := catalog.NewSyncer(repoRoot)
			_, err = s.SyncFiltered(cmd.Context(), cfg, func(src config.Source) bool {
				return src.Presets.Enabled
			})
			if err != nil {
				return err
			}

			// Resolve allowed sets and remove orphan per-dependency set markers.
			res, err := presets.Resolve(repoRoot, cfg, []yamlchart.Dependency{*dep})
			if err != nil {
				return err
			}
			allowed := map[string]struct{}{}
			if rd, ok := res.ByID[wantID]; ok {
				for s := range rd.SetPaths {
					allowed[s] = struct{}{}
				}
			}
			_ = removeOrphanDepSetMarkers(inst.Path, wantID, allowed)

			// Re-import presets and regenerate merged values.
			if _, err := presets.Import(presets.ImportParams{RepoRoot: repoRoot, InstancePath: inst.Path, Config: cfg, Dependencies: chart.Dependencies}); err != nil {
				return err
			}
			if err := values.GenerateIfManaged(inst.Path); err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Synced presets for %s in %s (values regenerated)\n", wantID, args[0])
			return nil
		},
	}
	return cmd
}
