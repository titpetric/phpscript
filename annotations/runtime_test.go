package annotations_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/annotations"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/runner/coverage"
)

var testRuntimeFileSystem = fstest.MapFS{
	"index.php": {Data: []byte(`<?php
// @route GET /
echo site_name();
`)},
	"branch.php": {Data: []byte(`<?php
// @route GET /branch
$total = 0;
foreach ([1, 2, 3] as $value) {
	$total += $value;
}
echo $total;
`)},
}

// TestRuntimeFunc covers the hook a host customizes an endpoint's runtime
// with. It runs after the standard library is installed, so a hook sees the
// bindings a script already has and a later hook replaces what an earlier one
// registered. One runtime is built per request, so the hook runs once per
// request and nothing a script did leaks into the next one.
func TestRuntimeFunc(t *testing.T) {
	t.Parallel()
	var calls int
	var sawStandardLibrary bool

	first := annotations.WithRuntimeFunc(func(rt *runner.Runtime) {
		calls++
		_, sawStandardLibrary = rt.LookupFunc("strlen")
		rt.RegisterFunc("site_name", func() string { return "first" })
	})
	second := annotations.WithRuntimeFunc(func(rt *runner.Runtime) {
		rt.RegisterFunc("site_name", func() string { return "example.com" })
	})

	mux := http.NewServeMux()
	if err := annotations.NewRoute(testRuntimeFileSystem, first, second).RegisterMux(mux); err != nil {
		t.Fatal(err)
	}

	for request := 1; request <= 2; request++ {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
		}
		if got := response.Body.String(); got != "example.com" {
			t.Fatalf("body = %q, want the value the later hook registered", got)
		}
		if calls != request {
			t.Fatalf("the hook ran %d times over %d requests, want one per request", calls, request)
		}
	}

	if !sawStandardLibrary {
		t.Error("strlen was not installed when the hook ran, so a hook cannot read what a script has")
	}
}

// TestWithCoverage covers the statement counts a server keeps for its
// application code. A collector is per run because it keys statements by the
// AST node the parser produced, and the aggregator keys them by profile line,
// so counts of two requests add on one entry per statement range.
func TestWithCoverage(t *testing.T) {
	t.Parallel()
	aggregator := coverage.NewAggregator()
	mux := http.NewServeMux()
	route := annotations.NewRoute(testRuntimeFileSystem, annotations.WithCoverage(aggregator))
	if err := route.RegisterMux(mux); err != nil {
		t.Fatal(err)
	}

	if !aggregator.Empty() {
		t.Fatal("the aggregator holds counts before a request ran")
	}

	for range 2 {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/branch", nil))
		if response.Code != http.StatusOK || response.Body.String() != "6" {
			t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
		}
	}

	if aggregator.Empty() {
		t.Fatal("no statement was counted, so the endpoint ran uninstrumented")
	}
	if files := aggregator.Files(); len(files) != 1 || files[0] != "branch.php" {
		t.Fatalf("files = %v, want the endpoint that was requested", files)
	}

	blocks := aggregator.Blocks()
	if len(blocks) == 0 {
		t.Fatal("no block was recorded")
	}
	for _, block := range blocks {
		if block.Count < 2 {
			t.Errorf("block %+v was counted %d times over two requests, want the counts added", block, block.Count)
		}
	}
}
