package artifacthub

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_SearchHelm(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/packages/search" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"packages": [
				{
					"name": "postgresql",
					"display_name": "PostgreSQL",
					"description": "db",
					"repository": {"repository_id":"64117528-d525-41d6-862c-41a43207c431","name":"bitnami","display_name":"Bitnami","url":"https://charts.bitnami.com/bitnami"}
				}
			]
		}`))
	}))
	defer srv.Close()

	c := NewClient()
	c.BaseURL = srv.URL

	pkgs, err := c.SearchHelm(context.Background(), "postgres", 10)
	if err != nil {
		t.Fatalf("SearchHelm: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 result")
	}
	if pkgs[0].RepositoryURL == "" || pkgs[0].Name != "postgresql" {
		t.Fatalf("unexpected result: %#v", pkgs[0])
	}
	if pkgs[0].RepositoryKey != "bitnami" {
		t.Fatalf("expected repository key")
	}
}

func TestClient_GetHelmPackage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/packages/helm/bitnami/postgresql" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"name": "postgresql",
			"available_versions": ["1.0.0","2.0.0"]
		}`))
	}))
	defer srv.Close()

	c := NewClient()
	c.BaseURL = srv.URL

	detail, err := c.GetHelmPackage(context.Background(), "bitnami", "postgresql")
	if err != nil {
		t.Fatalf("GetHelmPackage: %v", err)
	}
	if len(detail.Versions) != 2 {
		t.Fatalf("expected 2 versions")
	}
}
