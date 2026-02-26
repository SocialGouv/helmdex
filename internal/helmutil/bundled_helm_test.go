package helmutil

import "testing"

func TestParseChecksumsTxt(t *testing.T) {
	s := "" +
		"deadbeef  helm-v4.1.1-linux-amd64.tar.gz\n" +
		"0123456789abcdef  helm-v4.1.1-darwin-arm64.tar.gz\n" +
		"\n" +
		"# ignore\n" +
		"cafebabe helm-v4.1.1-windows-amd64.zip\n"

	m := parseChecksumsTxt(s)
	if m["helm-v4.1.1-linux-amd64.tar.gz"] != "deadbeef" {
		t.Fatalf("unexpected sum: %q", m["helm-v4.1.1-linux-amd64.tar.gz"])
	}
	if m["helm-v4.1.1-darwin-arm64.tar.gz"] != "0123456789abcdef" {
		t.Fatalf("unexpected sum: %q", m["helm-v4.1.1-darwin-arm64.tar.gz"])
	}
	if m["helm-v4.1.1-windows-amd64.zip"] != "cafebabe" {
		t.Fatalf("unexpected sum: %q", m["helm-v4.1.1-windows-amd64.zip"])
	}
}

func TestParseSHA256File(t *testing.T) {
	// sha256sum style
	sum, err := parseSHA256File("deadbeef  helm-v4.1.1-linux-amd64.tar.gz\n", "helm-v4.1.1-linux-amd64.tar.gz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sum != "deadbeef" {
		t.Fatalf("unexpected sum: %q", sum)
	}

	// single token style
	sum, err = parseSHA256File("cafebabe\n", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sum != "cafebabe" {
		t.Fatalf("unexpected sum: %q", sum)
	}
}

func TestHelmArchiveName(t *testing.T) {
	got, err := helmArchiveName("v4.1.1", "linux", "amd64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "helm-v4.1.1-linux-amd64.tar.gz" {
		t.Fatalf("unexpected name: %q", got)
	}

	got, err = helmArchiveName("v4.1.1", "windows", "amd64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "helm-v4.1.1-windows-amd64.zip" {
		t.Fatalf("unexpected name: %q", got)
	}
}

func TestBundledHelmPathForHome(t *testing.T) {
	if got := bundledHelmPathForHome("/home/x", "linux"); got != "/home/x/.helmdex/bin/helm" {
		t.Fatalf("unexpected path: %q", got)
	}
	// On a non-windows runtime, filepath.Join uses '/' separators, even if the input
	// string contains backslashes.
	if got := bundledHelmPathForHome("C:\\Users\\x", "windows"); got != "C:\\Users\\x/.helmdex/bin/helm.exe" {
		t.Fatalf("unexpected path: %q", got)
	}
}
