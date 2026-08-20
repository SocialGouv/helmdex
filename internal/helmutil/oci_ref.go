package helmutil

import (
	"fmt"
	"strings"
)

// OCIChartRef returns the OCI chart reference to pass to Helm commands.
//
// repoURL is the namespace that holds the chart, read exactly as Helm reads
// `dependencies[].repository` in Chart.yaml: Helm appends the chart name, so
// oci://registry/org + demo addresses oci://registry/org/demo. helmdex derives
// the reference the same way, so the versions a picker lists are the versions
// `helm dependency update` can install.
//
// A repoURL that already ends with the chart name is deliberately NOT
// special-cased. The two readings — "namespace org/demo" and "full reference to
// demo" — are indistinguishable, so collapsing one into the other would
// silently retarget a dependency that works. Such a repository addresses
// <chart>/<chart> and fails; OCIFullRefHint explains why.
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
	return base + "/" + chartName, nil
}

// OCIFullRefHint explains a failed OCI operation whose repository looks like a
// full chart reference rather than the namespace holding it — the classic
// mistake, since Helm appends the chart name and the request then targets
// <chart>/<chart>. It returns "" when the repository does not look like that,
// so callers can append it unconditionally.
//
// It is a possibility to check, never a diagnosis. A namespace may legitimately
// end with the chart name (oci://reg/nginx holding nginx/nginx), and a registry
// answers 403 both for "absent" and for "forbidden" — deliberately, so that
// repository existence is not leaked — so the two cases are indistinguishable.
// The wording therefore states what was requested and leaves the conclusion to
// the reader, and callers keep treating the underlying failure as what it is:
// an auth failure stays an auth failure, with its sign-in prompt.
// ociRefHintMarker identifies an explanation already present in an error, so
// it is never appended twice.
const ociRefHintMarker = "Helm appends the chart name"

func OCIFullRefHint(repoURL, chartName string) string {
	repoURL = strings.TrimRight(strings.TrimSpace(repoURL), "/")
	chartName = strings.TrimSpace(chartName)
	if chartName == "" || !strings.HasPrefix(repoURL, "oci://") {
		return ""
	}
	if !strings.HasSuffix(repoURL, "/"+chartName) {
		return ""
	}
	return fmt.Sprintf(
		"note: "+ociRefHintMarker+" to the repository, so this asked the registry for %q; "+
			"if the chart is not published under that path, set repository to %q",
		strings.TrimPrefix(repoURL+"/"+chartName, "oci://"), strings.TrimSuffix(repoURL, "/"+chartName))
}
