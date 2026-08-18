package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"helmdex/internal/testutil"
)

// cliRepo is a hermetic repo plus a way to run commands against it.
type cliRepo struct {
	testutil.Repo
	t       *testing.T
	HelmLog string
}

// newCLIRepo builds a throwaway repo with the fake Helm in place and makes it
// the working directory.
//
// Commands run without --repo, the way a user standing in their repo does.
// That also avoids a flag collision: `instance dep add` defines its own
// --repo (the chart repository URL), which shadows the root's --repo (the
// repo root) and would leave the root auto-detected from the cwd anyway.
func newCLIRepo(t *testing.T, opts testutil.RepoOpts) *cliRepo {
	t.Helper()
	helmLog := testutil.Hermetic(t)
	repo := testutil.NewRepo(t, opts)
	t.Chdir(repo.Root)
	return &cliRepo{Repo: repo, t: t, HelmLog: helmLog}
}

// run executes a helmdex command in the repo and returns its stdout.
func (r *cliRepo) run(args ...string) string {
	r.t.Helper()
	out, err := r.tryRun(args...)
	if err != nil {
		r.t.Fatalf("helmdex %s failed: %v\noutput:\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

// tryRun is run without the failure assertion, for error-path tests.
func (r *cliRepo) tryRun(args ...string) (string, error) {
	r.t.Helper()
	var stdout strings.Builder
	cmd := NewRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), err
}

// mustFail asserts a command fails and returns the error message.
func (r *cliRepo) mustFail(args ...string) string {
	r.t.Helper()
	out, err := r.tryRun(args...)
	if err == nil {
		r.t.Fatalf("helmdex %s unexpectedly succeeded:\n%s", strings.Join(args, " "), out)
	}
	return err.Error()
}

// runJSON executes a command and decodes its JSON stdout into v.
func (r *cliRepo) runJSON(v any, args ...string) {
	r.t.Helper()
	out := r.run(args...)
	if err := json.Unmarshal([]byte(out), v); err != nil {
		r.t.Fatalf("helmdex %s: decode json %q: %v", strings.Join(args, " "), out, err)
	}
}

// helmCalls returns the Helm invocations recorded so far.
func (r *cliRepo) helmCalls() []string {
	r.t.Helper()
	return testutil.FakeHelmCalls(r.t, r.HelmLog)
}
