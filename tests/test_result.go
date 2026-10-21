package tests

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/titpetric/phpscript/runner"
)

// TestResult carries execution outcome for a single fixture.
type TestResult struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	Path          string `json:"path"`
	Passed        bool   `json:"passed"`
	DurationMs    int64  `json:"duration_ms"`
	FailureReason string `json:"failure_reason,omitempty"`
	GotOutput     string `json:"got_output,omitempty"`
	WantOutput    string `json:"want_output,omitempty"`
	Error         string `json:"error,omitempty"`
	Runner        Runner `json:"runner,omitempty"`
	Skipped       bool   `json:"skipped,omitempty"`
}

// RunFixture executes a single fixture against the default runtime.
func RunFixture(ctx context.Context, f *Fixture) *TestResult {
	return RunFixtureOn(ctx, f, RunnerRuntime)
}

// RunFixtureOn executes a single fixture against one runner. The same expected
// output holds for every runner; the runner only decides how the source runs.
func RunFixtureOn(ctx context.Context, f *Fixture, r Runner) *TestResult {
	start := time.Now()
	res := &TestResult{
		Name:        f.Name,
		Description: f.Description,
		Path:        f.Path,
		Runner:      r,
	}

	if ctx.Value(tenantKey) == nil {
		ctx = context.WithValue(ctx, tenantKey, "acme")
	}

	var (
		out      string
		runErr   error
		headers  http.Header
		hostFail string
	)

	switch r {
	case RunnerFlatstack:
		var reqCtx runner.Context
		out, reqCtx, runErr = executeFlatstack(ctx, f)
		if errors.Is(runErr, ErrFlatstackUnsupported) {
			res.Skipped = true
			res.FailureReason = runErr.Error()
			res.Error = runErr.Error()
			res.DurationMs = time.Since(start).Milliseconds()
			return res
		}
		headers = reqCtx.ResponseHeaders()
	case RunnerPHP:
		php, hostErr := executePHP(ctx, f)
		if hostErr != nil {
			res.Skipped = errors.Is(hostErr, ErrRunnerUnavailable)
			res.FailureReason = hostErr.Error()
			res.Error = hostErr.Error()
			res.DurationMs = time.Since(start).Milliseconds()
			return res
		}
		out = php.Stdout
		runErr = phpFatal(php.Stderr)
		hostFail = php.diagnostics()
	default:
		var reqCtx runner.Context
		out, reqCtx, runErr = executeFixturePHP(ctx, f)
		headers = reqCtx.ResponseHeaders()
	}

	res.GotOutput = out
	res.WantOutput = strings.TrimSuffix(f.Expected, "\n")
	res.DurationMs = time.Since(start).Milliseconds()

	if f.Error != "" {
		if runErr == nil {
			res.FailureReason = fmt.Sprintf("expected uncaught error containing %q, but execution succeeded with output %q", f.Error, out)
			return res
		}
		if !errorChainContains(runErr, f.Error) {
			res.FailureReason = fmt.Sprintf("uncaught error %q does not contain expected substring %q", runErr.Error(), f.Error)
			res.Error = runErr.Error()
			return res
		}
	} else if runErr != nil {
		res.FailureReason = fmt.Sprintf("unexpected uncaught error: %v", runErr)
		res.Error = runErr.Error()
		return res
	}

	// The host stages response headers, so only an in-process runner has any
	// to check.
	if headers != nil {
		for k, wantVal := range f.Response.Headers {
			gotVal := headers.Get(k)
			if gotVal != wantVal {
				res.FailureReason = fmt.Sprintf("header mismatch for %s: got %q, want %q", k, gotVal, wantVal)
				return res
			}
		}
	}

	gotTrim := strings.TrimSuffix(out, "\n")
	if gotTrim != res.WantOutput {
		res.FailureReason = fmt.Sprintf("output mismatch:\n  got:  %q\n  want: %q", gotTrim, res.WantOutput)
		if hostFail != "" {
			res.FailureReason += "\n  " + hostFail
		}
		return res
	}

	res.Passed = true
	return res
}
