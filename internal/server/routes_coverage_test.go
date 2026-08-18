package server

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
)

// Every API route must be exercised by this package's tests. The expected set
// is parsed from routes() rather than duplicated here, so adding a route
// without a test fails the suite instead of silently widening the untested
// surface.

var (
	coveredMu sync.Mutex
	covered   = map[string]bool{}
)

// recordRoute notes which mux pattern would serve a request. It runs inside
// the test client, so coverage reflects requests the tests actually make.
func recordRoute(mux *http.ServeMux, r *http.Request) {
	_, pattern := mux.Handler(r)
	if pattern == "" || pattern == "/" {
		// The SPA fallback is not an API route.
		return
	}
	coveredMu.Lock()
	covered[pattern] = true
	coveredMu.Unlock()
}

var routePattern = regexp.MustCompile(`\.HandleFunc\("((?:GET|POST|PUT|DELETE|PATCH|HEAD) [^"]+)"`)

// declaredRoutes parses the patterns registered in routes().
func declaredRoutes() ([]string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("cannot locate the server package source")
	}
	// Every file of the package, not just server.go: a route registered from
	// anywhere else would otherwise be silently exempt from this check.
	dir := filepath.Dir(thisFile)
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, src := range files {
		if strings.HasSuffix(src, "_test.go") {
			continue
		}
		b, err := os.ReadFile(src)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", src, err)
		}
		for _, m := range routePattern.FindAllStringSubmatch(string(b), -1) {
			out = append(out, m[1])
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no routes found in %s", dir)
	}
	sort.Strings(out)
	return out, nil
}

func TestMain(m *testing.M) {
	code := m.Run()
	if code != 0 {
		os.Exit(code)
	}
	// A filtered run only exercises part of the suite, so the coverage claim
	// would be meaningless.
	if f := flag.Lookup("test.run"); f != nil && f.Value.String() != "" {
		os.Exit(code)
	}
	if err := assertAllRoutesCovered(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(code)
}

func assertAllRoutesCovered() error {
	declared, err := declaredRoutes()
	if err != nil {
		return err
	}
	coveredMu.Lock()
	defer coveredMu.Unlock()

	missing := []string{}
	for _, pattern := range declared {
		if !covered[pattern] {
			missing = append(missing, pattern)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("these API routes have no test:\n  %v", missing)
	}
	return nil
}
