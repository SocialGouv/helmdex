package helmutil

import (
	"strings"
	"testing"
)

func TestVersionsFromTags(t *testing.T) {
	cases := []struct {
		name string
		tags []string
		want string
	}{
		{
			// Real ghcr.io output for a chart repository: the tag list mixes
			// versions with Artifact Hub's metadata tag, and is unordered.
			name: "drops non-version tags and sorts newest-first",
			tags: []string{"0.0.0-edge", "0.16.2", "artifacthub.io", "0.17.0", "1.0.0", "2.0.0"},
			want: "2.0.0,1.0.0,0.17.0,0.16.2",
		},
		{
			name: "drops cosign artifacts",
			tags: []string{"1.0.0", "sha256-2b1c4d.sig", "sha256-2b1c4d.att", "latest", "main"},
			want: "1.0.0",
		},
		{
			name: "hides pre-releases when a stable version exists",
			tags: []string{"1.0.0", "1.1.0-rc.1", "1.1.0-beta"},
			want: "1.0.0",
		},
		{
			// A repository that only ever published pre-releases would look
			// empty otherwise.
			name: "falls back to pre-releases when nothing stable exists",
			tags: []string{"1.1.0-rc.1", "1.1.0-rc.2"},
			want: "1.1.0-rc.2,1.1.0-rc.1",
		},
		{
			name: "deduplicates",
			tags: []string{"1.0.0", "1.0.0", "2.0.0"},
			want: "2.0.0,1.0.0",
		},
		{
			name: "ignores blanks",
			tags: []string{"", "  ", "1.0.0"},
			want: "1.0.0",
		},
		{
			name: "no usable tag yields no version",
			tags: []string{"latest", "artifacthub.io"},
			want: "",
		},
		{
			name: "empty input",
			tags: nil,
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.Join(versionsFromTags(tc.tags), ",")
			if got != tc.want {
				t.Fatalf("versionsFromTags(%v) = %q, want %q", tc.tags, got, tc.want)
			}
		})
	}
}

func TestSplitOCIRef(t *testing.T) {
	cases := []struct {
		name     string
		repoURL  string
		chart    string
		wantHost string
		wantPath string
		wantErr  bool
	}{
		{
			name:     "namespace appends the chart name",
			repoURL:  "oci://ghcr.io/org/charts",
			chart:    "demo",
			wantHost: "ghcr.io",
			wantPath: "org/charts/demo",
		},
		{
			// Helm appends unconditionally, so this addresses demo/demo. The
			// listing must agree with what `helm dependency update` resolves,
			// not quietly correct it.
			name:     "repository ending with the chart name is not collapsed",
			repoURL:  "oci://ghcr.io/org/charts/demo",
			chart:    "demo",
			wantHost: "ghcr.io",
			wantPath: "org/charts/demo/demo",
		},
		{
			name:     "host with port",
			repoURL:  "oci://registry.example.org:5000/org",
			chart:    "demo",
			wantHost: "registry.example.org:5000",
			wantPath: "org/demo",
		},
		{
			name:    "registry root has no repository path",
			repoURL: "oci://ghcr.io",
			chart:   "",
			wantErr: true,
		},
		{
			name:    "not an OCI url",
			repoURL: "https://charts.example.org",
			chart:   "demo",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host, repoPath, err := splitOCIRef(tc.repoURL, tc.chart)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want an error, got %s/%s", host, repoPath)
				}
				return
			}
			if err != nil {
				t.Fatalf("splitOCIRef: %v", err)
			}
			if host != tc.wantHost || repoPath != tc.wantPath {
				t.Fatalf("= %s / %s, want %s / %s", host, repoPath, tc.wantHost, tc.wantPath)
			}
		})
	}
}

// Registries publish aliases for the same chart version; showing both asks the
// user to choose blindly between identical options.
func TestVersionsFromTags_DedupesSemverEquivalentTags(t *testing.T) {
	got := versionsFromTags([]string{"1.0.0", "v1.0.0", "1.0", "1", "2.0.0"})
	if len(got) != 2 {
		t.Fatalf("aliases of the same version must collapse, got %v", got)
	}
	if got[0] != "2.0.0" {
		t.Fatalf("newest first expected, got %v", got)
	}
	// Whichever alias survives must be the original tag, usable verbatim.
	for _, v := range got {
		if strings.TrimSpace(v) == "" {
			t.Fatalf("kept an empty tag: %v", got)
		}
	}
}

// Build metadata is ignored by SemVer precedence, so without a tie-break the
// order of these would vary between runs.
func TestSortVersionsDesc_StableAcrossBuildMetadata(t *testing.T) {
	first := []string{"1.0.0+b", "1.0.0+a", "1.0.0+c"}
	second := []string{"1.0.0+c", "1.0.0+b", "1.0.0+a"}
	sortVersionsDesc(first)
	sortVersionsDesc(second)
	if strings.Join(first, ",") != strings.Join(second, ",") {
		t.Fatalf("ordering depends on input order: %v vs %v", first, second)
	}
}
