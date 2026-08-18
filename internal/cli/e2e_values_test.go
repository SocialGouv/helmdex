package cli

import (
	"strings"
	"testing"

	"helmdex/internal/testutil"
)

func TestCLI_Values_SetGetUnsetReplaceRegen(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	r.run("instance", "create", "alpha")

	// set + get, including a dotted path and an array index.
	r.run("instance", "values", "set", "alpha", "--path", "$.image.tag", "--value-yaml", "1.2.3")
	r.run("instance", "values", "set", "alpha", "--path", "$.ports", "--value-json", `[80, 443]`)

	var tag string
	r.runJSON(&tag, "instance", "values", "get", "alpha", "--path", "$.image.tag")
	if tag != "1.2.3" {
		t.Fatalf("tag = %q", tag)
	}
	var port float64
	r.runJSON(&port, "instance", "values", "get", "alpha", "--path", "$.ports[1]")
	if port != 443 {
		t.Fatalf("ports[1] = %v", port)
	}

	// The edit landed in the managed layer and the merged output was refreshed.
	if layer := r.Read(t, "apps", "alpha", "values.instance.yaml"); !strings.Contains(layer, "tag: 1.2.3") {
		t.Fatalf("edit layer:\n%s", layer)
	}
	if merged := r.Read(t, "apps", "alpha", "values.yaml"); !strings.Contains(merged, "tag: 1.2.3") {
		t.Fatalf("merged values not regenerated:\n%s", merged)
	}

	// unset removes only the targeted key.
	r.run("instance", "values", "unset", "alpha", "--path", "$.image.tag")
	if layer := r.Read(t, "apps", "alpha", "values.instance.yaml"); strings.Contains(layer, "1.2.3") {
		t.Fatalf("unset left the value behind:\n%s", layer)
	}
	if layer := r.Read(t, "apps", "alpha", "values.instance.yaml"); !strings.Contains(layer, "ports") {
		t.Fatalf("unset removed an unrelated key:\n%s", layer)
	}

	// replace swaps the whole document.
	r.Write(t, "replicaCount: 7\n", "new-values.yaml")
	r.run("instance", "values", "replace", "alpha", "--file", r.Path("new-values.yaml"))
	layer := r.Read(t, "apps", "alpha", "values.instance.yaml")
	if !strings.Contains(layer, "replicaCount: 7") || strings.Contains(layer, "ports") {
		t.Fatalf("replace did not swap the document:\n%s", layer)
	}

	// regen with a stale merged file.
	r.Write(t, "stale: true\n", "apps", "alpha", "values.yaml")
	r.run("instance", "values", "regen", "alpha")
	merged := r.Read(t, "apps", "alpha", "values.yaml")
	if strings.Contains(merged, "stale") || !strings.Contains(merged, "replicaCount: 7") {
		t.Fatalf("regen did not rebuild values.yaml:\n%s", merged)
	}
}

func TestCLI_Values_RegenFalseDefersMerge(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	r.run("instance", "create", "alpha")

	r.run("instance", "values", "set", "alpha", "--path", "$.replicaCount", "--value-yaml", "5", "--regen=false")
	if merged := r.Read(t, "apps", "alpha", "values.yaml"); strings.Contains(merged, "replicaCount: 5") {
		t.Fatalf("--regen=false must not touch values.yaml:\n%s", merged)
	}
	r.run("instance", "values", "regen", "alpha")
	if merged := r.Read(t, "apps", "alpha", "values.yaml"); !strings.Contains(merged, "replicaCount: 5") {
		t.Fatalf("regen did not pick up the deferred edit:\n%s", merged)
	}
}

func TestCLI_Values_RejectsBadInput(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	r.run("instance", "create", "alpha")

	if msg := r.mustFail("instance", "values", "set", "alpha", "--value-yaml", "1"); !strings.Contains(msg, "path") {
		t.Fatalf("missing --path should be reported, got %q", msg)
	}
	if msg := r.mustFail("instance", "values", "get", "ghost"); strings.TrimSpace(msg) == "" {
		t.Fatal("an unknown instance must produce an error")
	}
	r.mustFail("instance", "values", "set", "alpha", "--path", "not-a-path", "--value-yaml", "1")
	r.mustFail("instance", "values", "set", "alpha", "--path", "$.a", "--value-json", "{not json")
}

func TestCLI_Values_TableFormat(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{SourceMode: testutil.SourceNone})
	r.run("instance", "create", "alpha")
	r.run("instance", "values", "set", "alpha", "--path", "$.image.tag", "--value-yaml", "1.2.3")

	out := r.run("instance", "values", "get", "alpha", "--path", "$.image", "--format", "table")
	if !strings.Contains(out, "tag") || !strings.Contains(out, "1.2.3") {
		t.Fatalf("table output:\n%s", out)
	}
}

// In an agnostic repo helmdex edits values.yaml in place and never generates
// it. The CLI must honour that exactly like the API does.
func TestCLI_Values_DirectModeEditsInPlace(t *testing.T) {
	r := newCLIRepo(t, testutil.RepoOpts{Agnostic: true})

	before := r.Read(t, "apps", "demo-preprod", "values.yaml")
	r.run("instance", "values", "set", "demo-preprod", "--path", "$.demo.replicaCount", "--value-yaml", "3")
	after := r.Read(t, "apps", "demo-preprod", "values.yaml")

	if after == before {
		t.Fatal("the edit was not applied")
	}
	if !strings.Contains(after, "replicaCount: 3") {
		t.Fatalf("edit not applied:\n%s", after)
	}
	if got, want := len(strings.Split(after, "\n")), len(strings.Split(before, "\n")); got != want {
		t.Fatalf("in-place edit changed the line count (%d -> %d):\n%s", want, got, after)
	}
	for _, want := range []string{
		"# Wrapper defaults (hand-written, user-owned — helmdex must never regenerate this).",
		"# keep small in preprod",
		"# NetworkPolicies toggle.",
	} {
		if !strings.Contains(after, want) {
			t.Fatalf("comment %q lost:\n%s", want, after)
		}
	}
	if r.Exists(t, "apps", "demo-preprod", "values.instance.yaml") {
		t.Fatal("a managed layer file was created in a direct-mode instance")
	}
	if r.Exists(t, ".helmdex") {
		t.Fatal("in-repo state was written into an agnostic repo")
	}
}
