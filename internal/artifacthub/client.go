package artifacthub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"time"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewClient() *Client {
	return &Client{
		BaseURL: "https://artifacthub.io",
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type PackageSummary struct {
	// RepositoryID is Artifact Hub's internal UUID for the repository.
	RepositoryID string
	// RepositoryKey is the human-readable repository name used in some Artifact Hub URLs (e.g. "bitnami").
	RepositoryKey string
	// RepositoryName is a display label for UI.
	RepositoryName string
	RepositoryURL  string
	Name           string
	DisplayName    string
	Description    string
	// LastUpdated is the Artifact Hub timestamp for this package (as returned by search).
	LastUpdated time.Time
	// LatestVersion is the package version shown by Artifact Hub search results.
	LatestVersion string
}

type Version struct {
	Version string
}

type PackageDetail struct {
	RepositoryKey string
	Name         string
	Versions     []Version
}

// SearchHelm searches Helm charts on Artifact Hub.
// v0.2: uses Artifact Hub public API. The exact response shape can evolve; this parser is defensive.
func (c *Client) SearchHelm(ctx context.Context, query string, limit int) ([]PackageSummary, error) {
	if limit <= 0 {
		limit = 20
	}
	base, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	base.Path = path.Join(base.Path, "/api/v1/packages/search")
	q := base.Query()
	// Artifact Hub has historically supported both `ts_query_web` and `ts_query`.
	// Setting both makes the client more resilient to API changes.
	q.Set("ts_query_web", query)
	q.Set("ts_query", query)
	q.Set("limit", fmt.Sprintf("%d", limit))
	q.Set("offset", "0")
	q.Set("facets", "false")
	// kind=0 is Helm in Artifact Hub API.
	q.Set("kind", "0")
	base.RawQuery = q.Encode()

	var resp struct {
		Packages []struct {
			Name        string `json:"name"`
			DisplayName string `json:"display_name"`
			Description string `json:"description"`
			Version     string `json:"version"`
			TS          int64  `json:"ts"`
			Repository  struct {
				RepositoryID string `json:"repository_id"`
				Name         string `json:"name"`
				URL          string `json:"url"`
				DisplayName  string `json:"display_name"`
			} `json:"repository"`
		} `json:"packages"`
	}

	if err := c.doJSON(ctx, base.String(), &resp); err != nil {
		return nil, err
	}

	out := make([]PackageSummary, 0, len(resp.Packages))
	for _, p := range resp.Packages {
		repoName := p.Repository.DisplayName
		if repoName == "" {
			repoName = p.Repository.Name
		}
		var updated time.Time
		if p.TS > 0 {
			updated = time.Unix(p.TS, 0).UTC()
		}
		out = append(out, PackageSummary{
			RepositoryID:   p.Repository.RepositoryID,
			RepositoryKey:  p.Repository.Name,
			RepositoryName: repoName,
			RepositoryURL:  p.Repository.URL,
			Name:           p.Name,
			DisplayName:    p.DisplayName,
			Description:    p.Description,
			LastUpdated:    updated,
			LatestVersion:  p.Version,
		})
	}
	return out, nil
}

// GetHelmPackage fetches details including available versions for a Helm chart.
// Artifact Hub endpoint: /api/v1/packages/helm/<repoKey>/<packageName>
// where repoKey is a human-readable repo name (e.g. "bitnami").
func (c *Client) GetHelmPackage(ctx context.Context, repoKey, packageName string) (PackageDetail, error) {
	base, err := url.Parse(c.BaseURL)
	if err != nil {
		return PackageDetail{}, err
	}
	base.Path = path.Join(base.Path, "/api/v1/packages/helm/", repoKey, packageName)

	// `available_versions` sometimes comes back as an array of strings, and
	// sometimes as an array of objects (with a `version` field). Parse both.
	var resp struct {
		Name              string          `json:"name"`
		AvailableVersions json.RawMessage `json:"available_versions"`
	}
	if err := c.doJSON(ctx, base.String(), &resp); err != nil {
		return PackageDetail{}, err
	}

	out := PackageDetail{RepositoryKey: repoKey, Name: resp.Name}
	// Default: try []string
	var vsStr []string
	if err := json.Unmarshal(resp.AvailableVersions, &vsStr); err == nil {
		out.Versions = make([]Version, 0, len(vsStr))
		for _, v := range vsStr {
			out.Versions = append(out.Versions, Version{Version: v})
		}
		return out, nil
	}
	// Fallback: []{version: "x.y.z"}
	var vsObj []struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(resp.AvailableVersions, &vsObj); err == nil {
		out.Versions = make([]Version, 0, len(vsObj))
		for _, o := range vsObj {
			if o.Version == "" {
				continue
			}
			out.Versions = append(out.Versions, Version{Version: o.Version})
		}
		return out, nil
	}

	return PackageDetail{}, fmt.Errorf("artifacthub: unsupported available_versions shape")
	return out, nil
}

func (c *Client) doJSON(ctx context.Context, url string, out any) error {
	hc := c.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("artifacthub http %d", resp.StatusCode)
	}
	dec := json.NewDecoder(resp.Body)
	return dec.Decode(out)
}
