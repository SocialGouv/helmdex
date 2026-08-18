package cli

import "testing"

// A dep id/alias containing '.' or '[' must target that literal top-level key,
// not be mis-split into a nested path (which would clobber unrelated values).
func TestDepValuesPath_DottedDepID(t *testing.T) {
	p, err := depValuesPath("demo.image", "$.repository")
	if err != nil {
		t.Fatal(err)
	}
	// Expect two segments: "demo.image" then "repository".
	if len(p) != 2 {
		t.Fatalf("got %d segments, want 2 (dotted dep id was split)", len(p))
	}
}
