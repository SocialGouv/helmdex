package helmutil

import (
	"fmt"
	"strings"
)

// OCIChartRef returns the OCI chart reference to pass to Helm commands.
//
// helmdex supports two input styles for repoURL when it starts with oci://:
//
//  1) Recommended (full chart ref): oci://registry/org/chart
//  2) Backward-compatible (namespace only): oci://registry/org  (chartName is appended)
//
// The function avoids duplicating the chart segment (e.g. oci://.../chart/chart).
func OCIChartRef(repoURL, chartName string) (string, error) {
	repoURL = strings.TrimSpace(repoURL)
	chartName = strings.TrimSpace(chartName)
	if repoURL == "" {
		return "", fmt.Errorf("repoURL is required")
	}
	if !strings.HasPrefix(repoURL, "oci://") {
		return "", fmt.Errorf("not an OCI repoURL: %q", repoURL)
	}
	base := strings.TrimRight(repoURL, "/")
	if chartName == "" {
		return base, nil
	}
	// If repoURL already includes the chart name as the final path segment, keep as-is.
	if strings.HasSuffix(base, "/"+chartName) {
		return base, nil
	}
	return base + "/" + chartName, nil
}

