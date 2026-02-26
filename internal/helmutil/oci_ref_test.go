package helmutil

import "testing"

func TestOCIChartRef(t *testing.T) {
	t.Parallel()

	t.Run("full ref already includes chart", func(t *testing.T) {
		got, err := OCIChartRef("oci://registry-1.docker.io/cloudpirates/postgres", "postgres")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		want := "oci://registry-1.docker.io/cloudpirates/postgres"
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})

	t.Run("full ref with trailing slash", func(t *testing.T) {
		got, err := OCIChartRef("oci://registry-1.docker.io/cloudpirates/postgres/", "postgres")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		want := "oci://registry-1.docker.io/cloudpirates/postgres"
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})

	t.Run("base namespace appends chart", func(t *testing.T) {
		got, err := OCIChartRef("oci://registry-1.docker.io/cloudpirates", "postgres")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		want := "oci://registry-1.docker.io/cloudpirates/postgres"
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})
}
