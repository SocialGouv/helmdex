package helmutil

import "testing"

func TestParseSearchRepoVersionsForRef_ExactNameOnly(t *testing.T) {
	raw := `[
	  {"name":"helmdex-abc/postgresql","version":"9.4.2"},
	  {"name":"helmdex-abc/postgresql-ha","version":"1.2.3"},
	  {"name":"helmdex-abc/postgresql","version":"9.4.10"}
	]`

	vs, err := parseSearchRepoVersionsForRef(raw, "helmdex-abc/postgresql")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vs) != 2 {
		t.Fatalf("expected 2 versions, got %d: %v", len(vs), vs)
	}
	if vs[0] != "9.4.10" || vs[1] != "9.4.2" {
		t.Fatalf("unexpected versions order/content: %v", vs)
	}
}

func TestParseSearchRepoVersionsForRef_AcceptsChartOnlyName(t *testing.T) {
	raw := `[
	  {"name":"postgresql","version":"1.0.0"},
	  {"name":"postgresql-ha","version":"2.0.0"}
	]`

	vs, err := parseSearchRepoVersionsForRef(raw, "helmdex-abc/postgresql")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vs) != 1 || vs[0] != "1.0.0" {
		t.Fatalf("unexpected versions: %v", vs)
	}
}

func TestParseSearchRepoVersionsForRef_EmptyWhenNoMatch(t *testing.T) {
	raw := `[{"name":"helmdex-abc/postgresql-ha","version":"1.2.3"}]`
	vs, err := parseSearchRepoVersionsForRef(raw, "helmdex-abc/postgresql")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vs) != 0 {
		t.Fatalf("expected 0 versions, got %d: %v", len(vs), vs)
	}
}
