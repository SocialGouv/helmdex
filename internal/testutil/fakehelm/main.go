// Command fakehelm is a deterministic stand-in for the `helm` binary.
//
// helmdex resolves Helm through exec.LookPath when HELMDEX_E2E_STUB_HELM=1
// (see internal/helmutil/bundled_helm.go), so placing this binary first on PATH
// under that variable makes every Helm-dependent code path hermetic: no
// network, no chart registry, no real Helm install — while still exercising the
// real argument construction and output parsing in internal/helmutil.
//
// Supported surface (everything internal/helmutil invokes):
//
//	version --short
//	repo add <name> <url> [--force-update]
//	repo list -o json
//	repo update <name>...
//	repo remove <name>
//	search repo <ref> [--versions] [--devel] -o json
//	show chart|values|readme <ref> [--version <v>]
//	pull <ref> --version <v> --destination <dir>
//	dependency update|build          (operates on the working directory)
//
// Behaviour knobs, all optional (helmdex strips HELM_/XDG_/DOCKER_/HOME from
// the child environment but passes everything else through):
//
//	HELMDEX_FAKE_HELM_SPEC        path to a JSON Universe replacing the default charts
//	HELMDEX_FAKE_HELM_LOG         path to append one JSON line per invocation
//	HELMDEX_FAKE_HELM_DELAY_MS    sleep before running, to make async UI states observable
//	HELMDEX_FAKE_HELM_DELAY_CMD   only delay/block when the command starts with this prefix
//	HELMDEX_FAKE_HELM_BLOCK_UNTIL block until this file exists, so a test can hold a
//	                              call open for exactly as long as it needs
//	HELMDEX_FAKE_HELM_FAIL        "<cmd prefix>=<message>" pairs, ';'-separated, forcing failure
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	envSpec       = "HELMDEX_FAKE_HELM_SPEC"
	envLog        = "HELMDEX_FAKE_HELM_LOG"
	envDelayMS    = "HELMDEX_FAKE_HELM_DELAY_MS"
	envDelayCmd   = "HELMDEX_FAKE_HELM_DELAY_CMD"
	envBlockUntil = "HELMDEX_FAKE_HELM_BLOCK_UNTIL"
	envFail       = "HELMDEX_FAKE_HELM_FAIL"

	fakeHelmVersion = "v4.1.1+gfakehelm"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fatalf("no command given")
	}

	recordInvocation(args)
	applyDelay(args)
	applyBlock(args)
	applyForcedFailure(args)

	u, err := loadUniverse()
	if err != nil {
		fatalf("%v", err)
	}

	switch args[0] {
	case "version":
		cmdVersion(args[1:])
	case "repo":
		cmdRepo(args[1:])
	case "search":
		cmdSearch(u, args[1:])
	case "show":
		cmdShow(u, args[1:])
	case "pull":
		cmdPull(u, args[1:])
	case "dependency", "dep":
		cmdDependency(u, args[1:])
	default:
		fatalf("unknown command %q", args[0])
	}
}

// --- behaviour knobs ---

func recordInvocation(args []string) {
	path := strings.TrimSpace(os.Getenv(envLog))
	if path == "" {
		return
	}
	cwd, _ := os.Getwd()
	line, err := json.Marshal(struct {
		Args []string `json:"args"`
		Cwd  string   `json:"cwd"`
	}{Args: args, Cwd: cwd})
	if err != nil {
		fatalf("marshal invocation log: %v", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fatalf("open %s=%s: %v", envLog, path, err)
	}
	defer f.Close() //nolint:errcheck // best-effort close on a log file
	if _, err := f.Write(append(line, '\n')); err != nil {
		fatalf("append to %s=%s: %v", envLog, path, err)
	}
}

func applyDelay(args []string) {
	raw := strings.TrimSpace(os.Getenv(envDelayMS))
	if raw == "" {
		return
	}
	ms, err := strconv.Atoi(raw)
	if err != nil {
		fatalf("%s=%q is not an integer: %v", envDelayMS, raw, err)
	}
	if !commandSelected(args) {
		return
	}
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

// applyBlock holds the call open until the test creates the release file,
// which lets a test observe an in-flight operation without racing a timer.
func applyBlock(args []string) {
	path := strings.TrimSpace(os.Getenv(envBlockUntil))
	if path == "" || !commandSelected(args) {
		return
	}
	// Bounded so a forgotten release cannot hang a suite forever.
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	fatalf("%s=%s never appeared", envBlockUntil, path)
}

// commandSelected reports whether the delay/block knobs apply to this
// invocation. With no prefix configured, every command is selected.
func commandSelected(args []string) bool {
	prefix := strings.TrimSpace(os.Getenv(envDelayCmd))
	return prefix == "" || strings.HasPrefix(strings.Join(args, " "), prefix)
}

func applyForcedFailure(args []string) {
	raw := strings.TrimSpace(os.Getenv(envFail))
	if raw == "" {
		return
	}
	cmd := strings.Join(args, " ")
	for _, rule := range strings.Split(raw, ";") {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		prefix, message, ok := strings.Cut(rule, "=")
		if !ok {
			fatalf("%s rule %q is not <cmd prefix>=<message>", envFail, rule)
		}
		if strings.HasPrefix(cmd, strings.TrimSpace(prefix)) {
			fatalf("%s", strings.TrimSpace(message))
		}
	}
}

// --- version ---

func cmdVersion(args []string) {
	if has(args, "--short") {
		fmt.Println(fakeHelmVersion)
		return
	}
	fmt.Printf("version.BuildInfo{Version:%q, GoVersion:%q}\n", fakeHelmVersion, "go1.25.0")
}

// --- repo ---

// repositoriesFile mirrors the subset of Helm's repositories.yaml that matters
// here. Its location comes from HELM_REPOSITORY_CONFIG, which helmdex always
// sets explicitly.
type repositoriesFile struct {
	APIVersion   string      `yaml:"apiVersion"`
	Repositories []repoEntry `yaml:"repositories"`
}

type repoEntry struct {
	Name string `yaml:"name" json:"name"`
	URL  string `yaml:"url" json:"url"`
}

func repositoriesPath() string {
	p := strings.TrimSpace(os.Getenv("HELM_REPOSITORY_CONFIG"))
	if p == "" {
		fatalf("HELM_REPOSITORY_CONFIG is not set")
	}
	return p
}

func readRepositories() repositoriesFile {
	b, err := os.ReadFile(repositoriesPath())
	if err != nil {
		if os.IsNotExist(err) {
			return repositoriesFile{APIVersion: "v1"}
		}
		fatalf("read %s: %v", repositoriesPath(), err)
	}
	var f repositoriesFile
	if err := yaml.Unmarshal(b, &f); err != nil {
		fatalf("parse %s: %v", repositoriesPath(), err)
	}
	return f
}

func writeRepositories(f repositoriesFile) {
	f.APIVersion = "v1"
	b, err := yaml.Marshal(f)
	if err != nil {
		fatalf("marshal repositories: %v", err)
	}
	path := repositoriesPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		fatalf("write %s: %v", path, err)
	}
}

func cmdRepo(args []string) {
	if len(args) == 0 {
		fatalf("repo: subcommand required")
	}
	switch args[0] {
	case "add":
		repoAdd(args[1:])
	case "list":
		repoList(args[1:])
	case "update":
		repoUpdate(args[1:])
	case "remove", "rm":
		repoRemove(args[1:])
	default:
		fatalf("repo: unknown subcommand %q", args[0])
	}
}

func repoAdd(args []string) {
	force := has(args, "--force-update")
	pos := positional(args)
	if len(pos) != 2 {
		fatalf("repo add: expected <name> <url>, got %v", pos)
	}
	name, url := pos[0], pos[1]

	f := readRepositories()
	for i, r := range f.Repositories {
		if r.Name != name {
			continue
		}
		if f.Repositories[i].URL == url {
			// Helm only refuses a name reused for a *different* URL.
			fmt.Printf("%q already exists with the same configuration, skipping\n", name)
			return
		}
		if !force {
			fatalf("repository name (%s) already exists, please specify a different name", name)
		}
		f.Repositories[i].URL = url
		writeRepositories(f)
		fmt.Printf("%q has been updated. Happy Helming!\n", name)
		return
	}
	f.Repositories = append(f.Repositories, repoEntry{Name: name, URL: url})
	writeRepositories(f)
	fmt.Printf("%q has been added to your repositories\n", name)
}

func repoList(args []string) {
	f := readRepositories()
	if outputFlag(args) != "json" {
		// Helm only reports "nothing to show" in its human output.
		if len(f.Repositories) == 0 {
			fatalf("no repositories to show")
		}
		fatalf("repo list: only -o json is supported by fakehelm")
	}
	if len(f.Repositories) == 0 {
		// Helm 4 prints an empty array and exits 0, so callers distinguish
		// "no repos" from "listing failed".
		emitJSON([]repoEntry{})
		return
	}
	// Helm's JSON keys happen to match the YAML ones; the marshalling tags on
	// repoEntry cover both.
	emitJSON(f.Repositories)
}

func repoUpdate(args []string) {
	names := positional(args)
	f := readRepositories()
	known := map[string]struct{}{}
	for _, r := range f.Repositories {
		known[r.Name] = struct{}{}
	}
	for _, n := range names {
		if _, ok := known[n]; !ok {
			fatalf("no repositories found matching '%s'. Verify that the repo exists", n)
		}
	}
	fmt.Println("Hang tight while we grab the latest from your chart repositories...")
	for _, n := range names {
		fmt.Printf("...Successfully got an update from the %q chart repository\n", n)
	}
	fmt.Println("Update Complete. ⎈Happy Helming!⎈")
}

func repoRemove(args []string) {
	names := positional(args)
	if len(names) == 0 {
		fatalf("repo remove: expected at least one name")
	}
	f := readRepositories()
	for _, n := range names {
		kept := make([]repoEntry, 0, len(f.Repositories))
		found := false
		for _, r := range f.Repositories {
			if r.Name == n {
				found = true
				continue
			}
			kept = append(kept, r)
		}
		if !found {
			fatalf("no repo named %q found", n)
		}
		f.Repositories = kept
	}
	writeRepositories(f)
	fmt.Printf("%q has been removed from your repositories\n", strings.Join(names, "\", \""))
}

// --- search ---

func cmdSearch(u Universe, args []string) {
	if len(args) == 0 || args[0] != "repo" {
		fatalf("search: only `search repo` is supported by fakehelm")
	}
	rest := args[1:]
	if outputFlag(rest) != "json" {
		fatalf("search repo: only -o json is supported by fakehelm")
	}
	allVersions := has(rest, "--versions")
	devel := has(rest, "--devel")

	pos := positional(rest)
	if len(pos) != 1 {
		fatalf("search repo: expected exactly one keyword, got %v", pos)
	}
	// Helm matches the keyword as a substring of the full "<repo>/<chart>"
	// name, which is why searching "repo/postgresql" also returns
	// "repo/postgresql-ha". helmdex filters those out by exact name and that
	// filter needs the noise to be present.
	term := pos[0]

	repos := []string{}
	for _, r := range readRepositories().Repositories {
		repos = append(repos, r.Name)
	}
	sort.Strings(repos)

	type item struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		AppVersion  string `json:"app_version"`
		Description string `json:"description"`
	}
	out := []item{}
	for _, rn := range repos {
		for _, c := range u.Charts {
			full := rn + "/" + c.Name
			if !strings.Contains(full, term) {
				continue
			}
			versions := c.sortedVersions(devel)
			if !allVersions && len(versions) > 1 {
				versions = versions[:1]
			}
			for _, v := range versions {
				out = append(out, item{
					Name:        full,
					Version:     v,
					AppVersion:  c.AppVersion,
					Description: c.Description,
				})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	emitJSON(out)
}

// --- show ---

func cmdShow(u Universe, args []string) {
	if len(args) == 0 {
		fatalf("show: subcommand required")
	}
	kind := args[0]
	switch kind {
	case "chart", "values", "readme":
	default:
		fatalf("show: unknown subcommand %q", kind)
	}
	rest := args[1:]
	pos := positional(rest)
	if len(pos) != 1 {
		fatalf("show %s: expected exactly one chart reference, got %v", kind, pos)
	}
	c, version := resolveChartRef(u, pos[0], flagValue(rest, "--version"))

	switch kind {
	case "chart":
		fmt.Print(c.chartYAMLFor(version))
	case "values":
		fmt.Print(c.valuesFor(version))
	case "readme":
		fmt.Print(c.readmeFor(version))
	}
}

// --- pull ---

func cmdPull(u Universe, args []string) {
	pos := positional(args)
	if len(pos) != 1 {
		fatalf("pull: expected exactly one chart reference, got %v", pos)
	}
	dest := flagValue(args, "--destination")
	if dest == "" {
		dest = "."
	}
	c, version := resolveChartRef(u, pos[0], flagValue(args, "--version"))
	if err := os.MkdirAll(dest, 0o755); err != nil {
		fatalf("mkdir %s: %v", dest, err)
	}
	writeChartArchive(filepath.Join(dest, fmt.Sprintf("%s-%s.tgz", c.Name, version)), c, version)
}

// --- dependency ---

// chartFile is the subset of Chart.yaml fakehelm needs to resolve dependencies.
type chartFile struct {
	Name         string         `yaml:"name"`
	Version      string         `yaml:"version"`
	Dependencies []chartFileDep `yaml:"dependencies"`
}

type chartFileDep struct {
	Name       string `yaml:"name"`
	Version    string `yaml:"version"`
	Repository string `yaml:"repository"`
	Alias      string `yaml:"alias,omitempty"`
}

type lockFile struct {
	Dependencies []lockFileDep `yaml:"dependencies"`
	Digest       string        `yaml:"digest"`
	Generated    string        `yaml:"generated"`
}

type lockFileDep struct {
	Name       string `yaml:"name"`
	Version    string `yaml:"version"`
	Repository string `yaml:"repository"`
	Digest     string `yaml:"digest,omitempty"`
}

func cmdDependency(u Universe, args []string) {
	if len(args) == 0 {
		fatalf("dependency: subcommand required")
	}
	action := args[0]
	switch action {
	case "update", "up", "build":
	default:
		fatalf("dependency: unknown subcommand %q", action)
	}

	dir := "."
	if pos := positional(args[1:]); len(pos) == 1 {
		dir = pos[0]
	}

	chart := readChartFile(filepath.Join(dir, "Chart.yaml"))
	resolved := resolveDependencies(u, chart.Dependencies)

	if action == "build" {
		lockPath := filepath.Join(dir, "Chart.lock")
		b, err := os.ReadFile(lockPath)
		if err != nil {
			fatalf("no lock file found at %s; run `helm dependency update` first", lockPath)
		}
		var existing lockFile
		if err := yaml.Unmarshal(b, &existing); err != nil {
			fatalf("parse %s: %v", lockPath, err)
		}
		// Real Helm refuses to build from a lock that no longer matches
		// Chart.yaml, and helmdex detects that message to fall back to an
		// update (see instances.RelockDependencies).
		if lockDigest(existing.Dependencies) != lockDigest(resolved) {
			fatalf("the lock file (Chart.lock) is out of sync with the dependencies file (Chart.yaml). Please update the dependencies")
		}
	}

	vendorDependencies(u, dir, resolved)
	writeLock(filepath.Join(dir, "Chart.lock"), resolved)
	fmt.Printf("Saving %d charts\n", len(resolved))
	fmt.Println("Deleting outdated charts")
}

func readChartFile(path string) chartFile {
	b, err := os.ReadFile(path)
	if err != nil {
		fatalf("read %s: %v", path, err)
	}
	var c chartFile
	if err := yaml.Unmarshal(b, &c); err != nil {
		fatalf("parse %s: %v", path, err)
	}
	return c
}

func resolveDependencies(u Universe, deps []chartFileDep) []lockFileDep {
	out := make([]lockFileDep, 0, len(deps))
	for _, d := range deps {
		// Helm resolves a classic dependency through its cached repository
		// index, so an unregistered repository URL is a hard failure.
		if !strings.HasPrefix(d.Repository, "oci://") && !strings.HasPrefix(d.Repository, "file://") &&
			!repoURLRegistered(d.Repository) {
			fatalf("no cached repository for %s found. (try 'helm repo update')", d.Repository)
		}
		c, ok := u.chart(d.Name)
		if !ok {
			fatalf("can't get a valid version for repository %s, chart %s", d.Repository, d.Name)
		}
		v, err := c.resolveVersion(d.Version)
		if err != nil {
			fatalf("can't get a valid version for repository %s, chart %s: %v", d.Repository, d.Name, err)
		}
		out = append(out, lockFileDep{
			Name:       d.Name,
			Version:    v,
			Repository: d.Repository,
			Digest:     "sha256:" + shortHash(d.Name+"@"+v),
		})
	}
	return out
}

// vendorDependencies writes each resolved dependency both into the chart's
// charts/ directory (what `helm dependency update` produces) and into the Helm
// repository cache (where helmdex looks for cached archives).
func vendorDependencies(u Universe, dir string, deps []lockFileDep) {
	chartsDir := filepath.Join(dir, "charts")
	if err := os.MkdirAll(chartsDir, 0o755); err != nil {
		fatalf("mkdir %s: %v", chartsDir, err)
	}
	cacheDir := strings.TrimSpace(os.Getenv("HELM_REPOSITORY_CACHE"))
	if cacheDir != "" {
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			fatalf("mkdir %s: %v", cacheDir, err)
		}
	}
	// "Deleting outdated charts": Helm leaves only the resolved set behind, so
	// a version bump does not accumulate archives.
	wanted := map[string]struct{}{}
	for _, d := range deps {
		wanted[fmt.Sprintf("%s-%s.tgz", d.Name, d.Version)] = struct{}{}
	}
	stale, err := filepath.Glob(filepath.Join(chartsDir, "*.tgz"))
	if err != nil {
		fatalf("scan %s: %v", chartsDir, err)
	}
	for _, p := range stale {
		if _, keep := wanted[filepath.Base(p)]; !keep {
			if err := os.Remove(p); err != nil {
				fatalf("remove outdated chart %s: %v", p, err)
			}
		}
	}

	for _, d := range deps {
		c, ok := u.chart(d.Name)
		if !ok {
			fatalf("chart %q vanished from the universe while vendoring", d.Name)
		}
		name := fmt.Sprintf("%s-%s.tgz", d.Name, d.Version)
		writeChartArchive(filepath.Join(chartsDir, name), c, d.Version)
		if cacheDir != "" {
			writeChartArchive(filepath.Join(cacheDir, name), c, d.Version)
		}
	}
}

func writeLock(path string, deps []lockFileDep) {
	// Helm writes dependencies, digest and generated — no apiVersion, and no
	// per-dependency digest. A fixed timestamp keeps the file byte-stable
	// across runs so tests can compare it directly.
	bare := make([]lockFileDep, 0, len(deps))
	for _, d := range deps {
		bare = append(bare, lockFileDep{Name: d.Name, Version: d.Version, Repository: d.Repository})
	}
	l := lockFile{
		Dependencies: bare,
		Digest:       "sha256:" + lockDigest(deps),
		Generated:    "2024-01-01T00:00:00Z",
	}
	b, err := yaml.Marshal(l)
	if err != nil {
		fatalf("marshal lock: %v", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		fatalf("write %s: %v", path, err)
	}
}

// lockDigest hashes the (name, version, repository) triples that decide whether
// a lock matches its Chart.yaml.
func lockDigest(deps []lockFileDep) string {
	keys := make([]string, 0, len(deps))
	for _, d := range deps {
		keys = append(keys, d.Name+"|"+d.Version+"|"+d.Repository)
	}
	sort.Strings(keys)
	return shortHash(strings.Join(keys, "\n"))
}

func writeChartArchive(path string, c Chart, version string) {
	f, err := os.Create(path)
	if err != nil {
		fatalf("create %s: %v", path, err)
	}
	defer f.Close() //nolint:errcheck // closed after flush below

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	files := []struct{ name, body string }{
		{c.Name + "/Chart.yaml", c.chartYAMLFor(version)},
		{c.Name + "/values.yaml", c.valuesFor(version)},
		{c.Name + "/README.md", c.readmeFor(version)},
		{c.Name + "/values.schema.json", c.schemaFor(version)},
	}
	for _, file := range files {
		hdr := &tar.Header{
			Name:    file.name,
			Mode:    0o644,
			Size:    int64(len(file.body)),
			ModTime: time.Unix(0, 0),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			fatalf("tar header %s: %v", file.name, err)
		}
		if _, err := tw.Write([]byte(file.body)); err != nil {
			fatalf("tar write %s: %v", file.name, err)
		}
	}
	if err := tw.Close(); err != nil {
		fatalf("close tar %s: %v", path, err)
	}
	if err := gz.Close(); err != nil {
		fatalf("close gzip %s: %v", path, err)
	}
}

// --- shared helpers ---

// resolveChartRef maps a chart reference (`<repo>/<chart>` or
// `oci://host/path/<chart>`) plus an optional version to a universe chart.
//
// A classic reference only resolves once its repository has been registered:
// real Helm answers "repo <name> not found" otherwise, and helmdex relies on
// having called `repo add` first. Serving the chart regardless would leave
// that whole layer untested.
func resolveChartRef(u Universe, ref, version string) (Chart, string) {
	repoName, chartName := splitRef(ref)
	if repoName != "" && !repoRegistered(repoName) {
		fatalf("repo %s not found", repoName)
	}
	if chartName == "" {
		fatalf("cannot derive a chart name from reference %q", ref)
	}
	c, ok := u.chart(chartName)
	if !ok {
		fatalf("chart %q not found", ref)
	}
	v, err := c.resolveVersion(version)
	if err != nil {
		fatalf("%v", err)
	}
	return c, v
}

// splitRef splits a chart reference into its repository part and chart name.
// For `oci://` refs the repository part is empty: only the last path segment
// identifies the chart.
func splitRef(ref string) (repoName, chartName string) {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "oci://") {
		trimmed := strings.TrimSuffix(strings.TrimPrefix(ref, "oci://"), "/")
		return "", trimmed[strings.LastIndex(trimmed, "/")+1:]
	}
	if i := strings.Index(ref, "/"); i >= 0 {
		return ref[:i], ref[i+1:]
	}
	return "", ref
}

// repoRegistered reports whether `helm repo add` has been run for this name.
func repoRegistered(name string) bool {
	for _, r := range readRepositories().Repositories {
		if r.Name == name {
			return true
		}
	}
	return false
}

// repoURLRegistered reports whether any registered repo serves this URL.
func repoURLRegistered(url string) bool {
	for _, r := range readRepositories().Repositories {
		if r.URL == url {
			return true
		}
	}
	return false
}

func has(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

// flagValue returns the value of `--flag value` or `--flag=value`.
func flagValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
		if v, ok := strings.CutPrefix(a, flag+"="); ok {
			return v
		}
	}
	return ""
}

func outputFlag(args []string) string {
	if v := flagValue(args, "-o"); v != "" {
		return v
	}
	return flagValue(args, "--output")
}

// valuedFlags are the flags whose following argument is a value, not a
// positional argument.
var valuedFlags = map[string]bool{
	"-o":            true,
	"--output":      true,
	"--version":     true,
	"--destination": true,
	"-d":            true,
	"--repo":        true,
	"--username":    true,
	"--password":    true,
}

func positional(args []string) []string {
	out := []string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			if valuedFlags[a] {
				i++
			}
			continue
		}
		out = append(out, a)
	}
	return out
}

func emitJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		fatalf("marshal json output: %v", err)
	}
	fmt.Println(string(b))
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// fatalf mirrors Helm's failure shape: message on stderr prefixed with
// "Error:", non-zero exit.
func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}
