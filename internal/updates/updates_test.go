package updates

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLatestParsesRelease(t *testing.T) {
	var gotPath, gotUA string
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotUA = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","html_url":"https://github.com/o/r/releases/tag/v1.2.3"}`))
	}))
	defer stub.Close()

	rel, err := Latest(context.Background(), stub.URL, "o/r")
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if gotPath != "/repos/o/r/releases/latest" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotUA != "helmdex" {
		t.Fatalf("user-agent = %q", gotUA)
	}
	if rel.TagName != "v1.2.3" || !strings.HasSuffix(rel.HTMLURL, "/v1.2.3") {
		t.Fatalf("release = %+v", rel)
	}
}

func TestLatestSurfacesHTTPErrors(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"API rate limit exceeded"}`, http.StatusForbidden)
	}))
	defer stub.Close()

	_, err := Latest(context.Background(), stub.URL, "o/r")
	if err == nil {
		t.Fatal("expected error on 403")
	}
	if !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("error should carry status and body, got: %v", err)
	}
}

func TestLatestRejectsEmptyTag(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer stub.Close()

	if _, err := Latest(context.Background(), stub.URL, "o/r"); err == nil {
		t.Fatal("expected error on payload without tag_name")
	}
}
