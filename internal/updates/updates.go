// Package updates checks GitHub for newer helmdex releases.
package updates

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Release is the subset of GitHub's "latest release" payload helmdex uses.
type Release struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// Latest fetches the latest published release of a GitHub repo ("owner/name").
// apiBase is the GitHub API root — tests point it at a local stub.
func Latest(ctx context.Context, apiBase, repo string) (Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", apiBase, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, fmt.Errorf("build update check request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "helmdex")

	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("check latest release: %w", err)
	}
	defer res.Body.Close() //nolint:errcheck // response body close

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return Release{}, fmt.Errorf("check latest release: %s returned %d: %s",
			url, res.StatusCode, strings.TrimSpace(string(body)))
	}
	var rel Release
	if err := json.NewDecoder(res.Body).Decode(&rel); err != nil {
		return Release{}, fmt.Errorf("decode latest release: %w", err)
	}
	if rel.TagName == "" {
		return Release{}, fmt.Errorf("latest release payload from %s has no tag_name", url)
	}
	return rel, nil
}
