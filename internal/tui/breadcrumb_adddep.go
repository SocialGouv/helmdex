package tui

import "strings"

// addDepCrumbsPlain builds the breadcrumb crumbs for the add-dependency wizard
// for window titles (no ANSI, no icons).
//
// Rules (agreed):
// - Always start with: Add dep
// - Prefer normalized source-kind crumbs when we have a selected chart/entry:
//     Add dep › Catalog › <catalogSourceName> › <entryID>
//     Add dep › Artifact Hub › <chartName> › <version>
// - Keep step-label crumbs only for:
//     Choose source / Artifact Hub search / Artifact Hub results / Artifact Hub versions
func addDepCrumbsPlain(m AppModel) []string {
	crumbs := []string{"Add dep"}

	switch m.depStep {
	case depStepChooseSource:
		return append(crumbs, "Choose source")

	case depStepCatalog:
		crumbs = append(crumbs, "Catalog")
		// While on the catalog list, include the currently highlighted entry.
		// This avoids repeating it in the body and makes navigation clearer.
		if it := m.catalogList.SelectedItem(); it != nil {
			if cli, ok := it.(catalogListItem); ok {
				if src := strings.TrimSpace(cli.E.SourceName); src != "" {
					crumbs = append(crumbs, src)
				}
				if id := strings.TrimSpace(cli.E.Entry.ID); id != "" {
					crumbs = append(crumbs, id)
				}
			}
		}
		return crumbs

	case depStepCatalogDetail:
		crumbs = append(crumbs, "Catalog")
		if m.catalogDetailEntry != nil {
			if src := strings.TrimSpace(m.catalogDetailEntry.SourceName); src != "" {
				crumbs = append(crumbs, src)
			}
			if id := strings.TrimSpace(m.catalogDetailEntry.Entry.ID); id != "" {
				crumbs = append(crumbs, id)
			}
		}
		return crumbs

	case depStepCatalogCollision:
		// Collision is always in the Catalog flow.
		return append(crumbs, "Catalog", "Resolve collision")

	case depStepAHQuery:
		return append(crumbs, "Artifact Hub search")
	case depStepAHResults:
		return append(crumbs, "Artifact Hub results")
	case depStepAHVersions:
		return append(crumbs, "Artifact Hub versions")

	case depStepAHDetail:
		crumbs = append(crumbs, "Artifact Hub")
		if m.ahSelected != nil {
			name := strings.TrimSpace(m.ahSelected.DisplayName)
			if name == "" {
				name = strings.TrimSpace(m.ahSelected.Name)
			}
			if name != "" {
				crumbs = append(crumbs, name)
			}
		}
		if v := strings.TrimSpace(m.ahSelectedVersion); v != "" {
			crumbs = append(crumbs, v)
		}
		return crumbs

	case depStepArbitrary:
		crumbs = append(crumbs, "Arbitrary")
		// Add sub-step for the new arbitrary wizard.
		// Use plain labels so window titles remain stable.
		switch m.arbStep {
		case arbStepRepo:
			return append(crumbs, "Repo")
		case arbStepChart:
			return append(crumbs, "Chart")
		case arbStepVersion:
			return append(crumbs, "Version")
		case arbStepAlias:
			return append(crumbs, "Alias")
		default:
			return crumbs
		}

	default:
		return crumbs
	}
}

// addDepCrumbsStyled is like addDepCrumbsPlain but includes icons (and therefore
// is suitable for the visible top bar).
func addDepCrumbsStyled(m AppModel) []string {
	plain := addDepCrumbsPlain(m)
	if len(plain) == 0 {
		return nil
	}

	// Iconize specific well-known crumbs without changing the overall crumb text.
	out := make([]string, 0, len(plain))
	for i, c := range plain {
		cc := c
		// First crumb is always Add dep.
		if i == 0 {
			out = append(out, withIcon(iconAdd, cc))
			continue
		}
		switch cc {
		case "Choose source":
			out = append(out, withIcon(iconWizard, cc))
		case "Catalog":
			out = append(out, withIcon(iconCatalog, cc))
		case "Artifact Hub":
			out = append(out, withIcon(iconAH, cc))
		case "Artifact Hub search", "Artifact Hub results", "Artifact Hub versions":
			out = append(out, withIcon(iconAH, cc))
		case "Arbitrary":
			out = append(out, withIcon(iconCustom, cc))
		default:
			out = append(out, cc)
		}
	}
	return out
}
