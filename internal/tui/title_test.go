package tui

import (
	"testing"

	"helmdex/internal/artifacthub"
	"helmdex/internal/catalog"
	"helmdex/internal/instances"
)

func TestBuildWindowTitle(t *testing.T) {
	tests := []struct {
		name string
		m    AppModel
		want string
	}{
		{
			name: "dashboard",
			m:    AppModel{screen: ScreenDashboard},
			want: "🧭 HelmDex — Dashboard",
		},
		{
			name: "instance with name",
			m: func() AppModel {
				inst := instances.Instance{Name: "my-app"}
				return AppModel{screen: ScreenInstance, selected: &inst}
			}(),
			want: "🧭 HelmDex — Dashboard › Instance › my-app",
		},
		{
			name: "add dep wizard step label",
			m: func() AppModel {
				inst := instances.Instance{Name: "my-app"}
				return AppModel{screen: ScreenInstance, selected: &inst, addingDep: true, depStep: depStepAHQuery}
			}(),
			want: "🧭 HelmDex — Dashboard › Instance › my-app › Add dep › Artifact Hub search",
		},
		{
			name: "add dep catalog detail includes source + entry id",
			m: func() AppModel {
				inst := instances.Instance{Name: "my-app"}
				m := AppModel{screen: ScreenInstance, selected: &inst, addingDep: true, depStep: depStepCatalogDetail}
				m.catalogDetailEntry = &catalog.EntryWithSource{SourceName: "remote-source", Entry: catalog.Entry{ID: "bitnami-nginx-15.0.0"}}
				return m
			}(),
			want: "🧭 HelmDex — Dashboard › Instance › my-app › Add dep › Catalog › remote-source › bitnami-nginx-15.0.0",
		},
		{
			name: "add dep artifact hub detail includes chart + version",
			m: func() AppModel {
				inst := instances.Instance{Name: "my-app"}
				sel := artifacthub.PackageSummary{Name: "postgresql", DisplayName: "postgresql"}
				m := AppModel{screen: ScreenInstance, selected: &inst, addingDep: true, depStep: depStepAHDetail}
				m.ahSelected = &sel
				m.ahSelectedVersion = "15.5.0"
				return m
			}(),
			want: "🧭 HelmDex — Dashboard › Instance › my-app › Add dep › Artifact Hub › postgresql › 15.5.0",
		},
		{
			name: "help wins over wizard",
			m: func() AppModel {
				inst := instances.Instance{Name: "my-app"}
				return AppModel{screen: ScreenInstance, selected: &inst, addingDep: true, depStep: depStepAHQuery, infoOpen: true, infoTab: 0}
			}(),
			want: "🧭 HelmDex — Dashboard › Instance › my-app › Help / About",
		},
		{
			name: "apply wins over other overlays",
			m: func() AppModel {
				inst := instances.Instance{Name: "my-app"}
				return AppModel{screen: ScreenInstance, selected: &inst, paletteOpen: true, applyOpen: true}
			}(),
			want: "🧭 HelmDex — Dashboard › Instance › my-app › Applying",
		},
		{
			name: "confirm wins over other overlays",
			m: func() AppModel {
				inst := instances.Instance{Name: "my-app"}
				return AppModel{screen: ScreenInstance, selected: &inst, confirmOpen: true, depDetailOpen: true}
			}(),
			want: "🧭 HelmDex — Dashboard › Instance › my-app › Confirm",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got := buildWindowTitle(tt.m)
			if got != tt.want {
				t.Fatalf("unexpected title\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}
