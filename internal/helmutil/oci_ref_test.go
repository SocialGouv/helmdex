package helmutil

import (
	"strings"
	"testing"
)

func TestOCIChartRef(t *testing.T) {
	t.Parallel()

	t.Run("namespace appends chart", func(t *testing.T) {
		got, err := OCIChartRef("oci://registry-1.docker.io/cloudpirates", "postgres")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		want := "oci://registry-1.docker.io/cloudpirates/postgres"
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})

	t.Run("trailing slash is ignored", func(t *testing.T) {
		got, err := OCIChartRef("oci://registry-1.docker.io/cloudpirates/", "postgres")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		want := "oci://registry-1.docker.io/cloudpirates/postgres"
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})

	// Helm appends the chart name unconditionally. Collapsing a repository that
	// already ends with it would disagree with `helm dependency update`, and
	// would silently retarget a namespace that genuinely ends with the chart
	// name — so helmdex resolves it exactly as Helm does.
	t.Run("repository ending with the chart name is not collapsed", func(t *testing.T) {
		got, err := OCIChartRef("oci://registry-1.docker.io/cloudpirates/postgres", "postgres")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		want := "oci://registry-1.docker.io/cloudpirates/postgres/postgres"
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})

	t.Run("no chart name yields the namespace", func(t *testing.T) {
		got, err := OCIChartRef("oci://registry-1.docker.io/cloudpirates", "")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got != "oci://registry-1.docker.io/cloudpirates" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("rejects a non-OCI url", func(t *testing.T) {
		if _, err := OCIChartRef("https://charts.example.org", "postgres"); err == nil {
			t.Fatal("want an error for a non-OCI repository")
		}
	})
}

func TestOCIFullRefHint(t *testing.T) {
	t.Parallel()

	t.Run("names the mistake and the fix", func(t *testing.T) {
		hint := OCIFullRefHint("oci://ghcr.io/nginxinc/charts/nginx-ingress", "nginx-ingress")
		if hint == "" {
			t.Fatal("a repository ending with the chart name must be explained")
		}
		if want := `"oci://ghcr.io/nginxinc/charts"`; !strings.Contains(hint, want) {
			t.Fatalf("hint must point at the namespace %s: %s", want, hint)
		}
	})

	t.Run("silent for a correct repository", func(t *testing.T) {
		if hint := OCIFullRefHint("oci://ghcr.io/nginxinc/charts", "nginx-ingress"); hint != "" {
			t.Fatalf("unexpected hint: %s", hint)
		}
	})

	t.Run("silent for a classic repository", func(t *testing.T) {
		if hint := OCIFullRefHint("https://charts.example.org/nginx", "nginx"); hint != "" {
			t.Fatalf("unexpected hint: %s", hint)
		}
	})
}
