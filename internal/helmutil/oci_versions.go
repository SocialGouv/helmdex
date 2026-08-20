package helmutil

import (
	"context"
	"errors"
	"fmt"
	"strings"

	semver "github.com/Masterminds/semver/v3"

	"helmdex/internal/creds"
	"helmdex/internal/ociregistry"
)

// ociChartVersions lists chart versions for an OCI repository by listing the
// registry's tags — see internal/ociregistry for why this cannot go through
// the helm CLI.
func ociChartVersions(ctx context.Context, repoRoot, repoURL, chartName string) ([]string, error) {
	host, repoPath, err := splitOCIRef(repoURL, chartName)
	if err != nil {
		return nil, err
	}
	username, secret := ociCredential(repoRoot, repoURL, host)

	client := ociregistry.NewClient(host)
	client.Username = username
	client.Secret = secret

	tags, err := client.Tags(ctx, repoPath)
	if err != nil {
		// Explain the doubled path rather than letting the user pick a version
		// that `helm dependency update` would then refuse. Only a 4xx says
		// anything about the path: a timeout, a 5xx or an unreachable registry
		// would turn "try again later" into "your configuration is wrong".
		var status *ociregistry.StatusError
		if errors.As(err, &status) && status.Code >= 400 && status.Code < 500 {
			if hint := OCIFullRefHint(repoURL, chartName); hint != "" {
				return nil, fmt.Errorf("%w — %s", err, hint)
			}
		}
		return nil, err
	}
	return versionsFromTags(tags), nil
}

// splitOCIRef resolves a repoURL/chartName pair to the registry host and the
// repository path below /v2/.
func splitOCIRef(repoURL, chartName string) (host, repoPath string, err error) {
	ref, err := OCIChartRef(repoURL, chartName)
	if err != nil {
		return "", "", err
	}
	host, repoPath, found := strings.Cut(strings.TrimPrefix(ref, "oci://"), "/")
	if !found || strings.TrimSpace(host) == "" || strings.TrimSpace(repoPath) == "" {
		return "", "", fmt.Errorf("OCI reference %q has no repository path; expected oci://<registry>/<path>/<chart>", ref)
	}
	return host, repoPath, nil
}

// ociCredential resolves the credential to authenticate against host, matching
// what a helm pull in the same env would use. Empty values mean anonymous
// access, which is what public charts need.
//
// Listing tags never shells out to helm, so this only reads: helmdex's own
// store answers first, and the env's registry config covers credentials a bare
// `helm registry login` wrote. Neither needs the env materialized.
func ociCredential(repoRoot, repoURL, host string) (username, secret string) {
	if cred, ok := creds.ForURL(repoURL, creds.KindOCI); ok && cred.Secret != "" {
		return creds.DefaultUsername(cred.Username), cred.Secret
	}
	if u, s, ok := creds.DockerAuthFor(EnvForRepoURL(repoRoot, repoURL).RegistryConfig, host); ok {
		return creds.DefaultUsername(u), s
	}
	return "", ""
}

// versionsFromTags turns registry tags into a chart version list.
//
// Only SemVer-parseable tags qualify: registries also hold tags that are not
// chart versions, such as Artifact Hub's "artifacthub.io" metadata tag and
// cosign's "sha256-….sig" artifacts. Pre-releases are hidden, mirroring
// `helm search repo` without --devel, and the result is newest-first.
//
// A repository that has only ever published pre-releases would otherwise look
// empty, so those are returned when nothing stable exists — the same fallback
// the classic path gets from searchRepoVersionsDevel.
func versionsFromTags(tags []string) []string {
	stable := make([]string, 0, len(tags))
	prerelease := make([]string, 0, len(tags))
	seen := map[string]struct{}{}
	for _, tag := range tags {
		v := strings.TrimSpace(tag)
		if v == "" {
			continue
		}
		parsed, err := semver.NewVersion(v)
		if err != nil {
			continue
		}
		// Dedupe on the parsed version, not the raw tag: registries often
		// publish aliases (`1.2.3` and `v1.2.3`) for the same chart, and
		// showing both asks the user to pick blindly between identical
		// options. The original tag is kept, since that is what pulls.
		key := parsed.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if parsed.Prerelease() != "" {
			prerelease = append(prerelease, v)
			continue
		}
		stable = append(stable, v)
	}
	if len(stable) == 0 {
		sortVersionsDesc(prerelease)
		return prerelease
	}
	sortVersionsDesc(stable)
	return stable
}
