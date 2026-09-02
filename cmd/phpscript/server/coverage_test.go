package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/internal/flags"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/runner/coverage"
)

var coverFS = fstest.MapFS{
	"public/index.php": {Data: []byte("<?php\necho greet(\"world\");\n")},
	"boot.php":         {Data: []byte("<?php\nfunction greet($n) { return \"hello, \" . $n; }\nfunction unused() { return 1; }\n")},
}

// coveredServer serves coverFS with coverage on and answers count requests, so
// what the module reports is what a run of the site produced.
func coveredServer(t *testing.T, count int, coverFile string) *coverageModule {
	t.Helper()
	module := newCoverageModule(&flags.Options{Cover: coverage.ModeLine, CoverFile: coverFile}, coverFS)

	options := runner.Options{Include: "boot.php"}
	handler, err := newHandler(coverFS, "", DefaultDocumentRoot, options, false, false)
	if err != nil {
		t.Fatalf("newHandler: %v", err)
	}
	handler.coverage = module.aggregator

	for range count {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		if w.Code != http.StatusOK || w.Body.String() != "hello, world" {
			t.Fatalf("request: status %d body %q", w.Code, w.Body.String())
		}
	}
	return module
}

// TestCoverageModuleCountsAcrossRequests pins the aggregation: each request
// gets its own collector, and what the process holds is the sum. The prelude
// counts too, which is what makes an --include file part of the measurement.
func TestCoverageModuleCountsAcrossRequests(t *testing.T) {
	module := coveredServer(t, 3, "")

	blocks := module.blocks()
	counts := map[string]int{}
	for _, block := range blocks {
		counts[block.File] += block.Count
	}
	if counts["public/index.php"] != 3 {
		t.Errorf("index.php counted %d, want one per request", counts["public/index.php"])
	}
	if counts["boot.php"] != 3 {
		t.Errorf("boot.php counted %d, want the called function per request", counts["boot.php"])
	}
	// The columns come from the source text, so a profile spans the statement
	// rather than the indentation around it.
	for _, block := range blocks {
		if block.StartCol < 1 || block.EndCol <= block.StartCol {
			t.Errorf("block %+v has no resolved columns", block)
		}
	}
}

// TestCoverageModuleServe covers the endpoint, which is how a test flow reads
// coverage off a server it is not going to shut down.
func TestCoverageModuleServe(t *testing.T) {
	module := coveredServer(t, 1, "")

	for _, tc := range []struct {
		mode   string
		status int
		want   []string
	}{
		{mode: "", status: http.StatusOK, want: []string{"mode: count", "public/index.php:2."}},
		{mode: "line", status: http.StatusOK, want: []string{"mode: count", "boot.php:2."}},
		{mode: "file", status: http.StatusOK, want: []string{"boot.php:1:", "index.php", "total:"}},
		{mode: "func", status: http.StatusOK, want: []string{"greet", "unused", "{main}", "total:"}},
		{mode: "statements", status: http.StatusBadRequest, want: []string{"unknown mode"}},
	} {
		t.Run("mode="+tc.mode, func(t *testing.T) {
			w := httptest.NewRecorder()
			module.serve(w, httptest.NewRequest(http.MethodGet, CoveragePath+"?mode="+tc.mode, nil))
			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d", w.Code, tc.status)
			}
			for _, want := range tc.want {
				if !strings.Contains(w.Body.String(), want) {
					t.Errorf("body is missing %q:\n%s", want, w.Body.String())
				}
			}
		})
	}
}

// TestCoverageModuleStop covers the flush: a graceful shutdown is the only
// moment a server knows it is finished, and {time} names one profile per run.
func TestCoverageModuleStop(t *testing.T) {
	dir := t.TempDir()
	module := coveredServer(t, 2, filepath.Join(dir, "cover", "site.{time}.cov"))

	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	written, err := filepath.Glob(filepath.Join(dir, "cover", "site.*.cov"))
	if err != nil || len(written) != 1 {
		t.Fatalf("written = %v err = %v, want one profile", written, err)
	}
	data, err := os.ReadFile(written[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "mode: count\n") || !strings.Contains(string(data), "boot.php:") {
		t.Errorf("profile =\n%s", data)
	}
}

// TestCoverageModuleStopWithoutRequests pins that a server nothing reached
// writes no profile. An empty one reads as a measurement that found nothing.
func TestCoverageModuleStopWithoutRequests(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "site.cov")
	module := newCoverageModule(&flags.Options{Cover: coverage.ModeLine, CoverFile: name})

	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if _, err := os.Stat(name); !os.IsNotExist(err) {
		t.Errorf("stat = %v, want no profile", err)
	}
}

// TestCoverageModuleSource pins that a file no site root answers for is left in
// the profile at column 1 rather than dropped out of it.
func TestCoverageModuleSource(t *testing.T) {
	module := newCoverageModule(&flags.Options{}, coverFS)
	if got := module.source("boot.php"); len(got) < 2 || !strings.Contains(got[1], "function greet") {
		t.Errorf("source = %v, want the file's lines", got)
	}
	if got := module.source("absent.php"); got != nil {
		t.Errorf("source of a missing file = %v, want nil", got)
	}
}
