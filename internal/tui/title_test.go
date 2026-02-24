package tui

import (
	"testing"

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
			name: "help wins over wizard",
			m: func() AppModel {
				inst := instances.Instance{Name: "my-app"}
				return AppModel{screen: ScreenInstance, selected: &inst, addingDep: true, depStep: depStepAHQuery, helpOpen: true}
			}(),
			want: "🧭 HelmDex — Dashboard › Instance › my-app › Help",
		},
		{
			name: "apply wins over other overlays",
			m: func() AppModel {
				inst := instances.Instance{Name: "my-app"}
				return AppModel{screen: ScreenInstance, selected: &inst, paletteOpen: true, applyOpen: true}
			}(),
			want: "🧭 HelmDex — Dashboard › Instance › my-app › Applying",
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
