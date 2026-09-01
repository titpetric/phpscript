package annotations_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/annotations"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/telemetry"
)

func TestStartupRunsAnnotatedPHPFilesInPathOrder(t *testing.T) {
	root := fstest.MapFS{
		"20-second.php": {Data: []byte("<?php\n# @startup:\necho \"second\";")},
		"10-first.php":  {Data: []byte("<?php\n// @startup\necho \"first,\";")},
		"ignored.php":   {Data: []byte("<?php\necho \"ignored\";")},
		"annotated.txt": {Data: []byte("// @startup")},
	}
	var out bytes.Buffer
	startup := annotations.NewStartup(root, annotations.WithOutput(&out))

	if err := startup.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "first,second" {
		t.Fatalf("output = %q, want %q", got, "first,second")
	}
}

func TestStartupProvidesProjectFilesystem(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/startup.php", []byte(`<?php
/* @startup */
$file = fopen("started.txt", "w");
fwrite($file, "ready");
fclose($file);
`), 0o600); err != nil {
		t.Fatal(err)
	}
	startup := annotations.NewStartup(os.DirFS(dir), annotations.WithRootDir(dir))

	if err := startup.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(dir + "/started.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "ready" {
		t.Fatalf("started.txt = %q, want ready", contents)
	}
}

// Start reports what failed and names the file it failed in. Whether that is
// fatal is the caller's decision, not this module's.
func TestStartupReturnsAnnotatedScriptError(t *testing.T) {
	root := fstest.MapFS{
		"broken.php": {Data: []byte("<?php\n// @startup\nmissing_function();")},
	}
	startup := annotations.NewStartup(root, annotations.WithOutput(&bytes.Buffer{}))

	err := startup.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "startup broken.php") {
		t.Fatalf("error = %v, want the failing file named", err)
	}
}

// The scanner walks in path order, so a failure in the first file must not stop
// the second from running: the jobs of one tree are independent, and every
// failure is joined into the returned error rather than only the first.
func TestStartupRunsEveryJobAndJoinsFailures(t *testing.T) {
	root := fstest.MapFS{
		"10-broken.php": {Data: []byte("<?php\n// @startup\nmissing_function();")},
		"20-after.php":  {Data: []byte("<?php\n// @startup\necho \"after\";")},
		"30-broken.php": {Data: []byte("<?php\n// @startup\nalso_missing();")},
	}
	var out bytes.Buffer
	startup := annotations.NewStartup(root, annotations.WithOutput(&out))

	err := startup.Start(context.Background())
	if err == nil {
		t.Fatal("error = nil, want both failures")
	}
	for _, want := range []string{"startup 10-broken.php", "startup 30-broken.php"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want it to name %s", err, want)
		}
	}
	if got := out.String(); got != "after" {
		t.Fatalf("output = %q, want the job between the failures to have run", got)
	}
}

func TestStartupRecordsSuccessAndFailure(t *testing.T) {
	for _, test := range []struct {
		name        string
		source      string
		wantFailure bool
	}{
		{name: "success", source: "<?php\n// @startup\necho 'ready';"},
		{name: "failure", source: "<?php\n// @startup\nmissing_function();", wantFailure: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			traceOptions := telemetry.NewOptions("phpscript")
			// oida records only when it is asked to; a test that reads its
			// traces back is asking.
			traceOptions.Enabled = true
			tracer, err := telemetry.New(traceOptions)
			if err != nil {
				t.Fatal(err)
			}
			recorder := telemetry.NewModule(tracer)
			root := fstest.MapFS{"startup.php": {Data: []byte(test.source)}}
			startup := annotations.NewStartup(root,
				annotations.WithOutput(&bytes.Buffer{}),
				annotations.WithObservers(recorder),
			)

			// The job is recorded on its own trace either way, and a failure
			// is also returned.
			if gotError := startup.Start(context.Background()) != nil; gotError != test.wantFailure {
				t.Fatalf("returned error = %t, wantFailure = %t", gotError, test.wantFailure)
			}
			snapshot := recorder.Snapshot()
			if snapshot.Total != 1 || snapshot.Active != 0 || len(snapshot.Log) != 1 {
				t.Fatalf("unexpected telemetry snapshot: %+v", snapshot)
			}

			// A startup file is not a request: it records as a background
			// trace named after the annotation that selected it.
			trace := snapshot.Log[0]
			if trace.Name != "@startup startup.php" || trace.HTTP != nil || telemetry.TraceHost(trace) != telemetry.BackgroundHost {
				t.Fatalf("unexpected startup trace: %+v", trace)
			}
			if len(trace.Spans) == 0 || trace.Spans[0].Name != "@startup startup.php" || trace.Spans[0].Filename != "startup.php" {
				t.Fatalf("unexpected startup spans: %+v", trace.Spans)
			}
			if gotError := trace.Spans[0].ErrorText != ""; gotError != test.wantFailure {
				t.Fatalf("span error = %q, wantFailure = %t", trace.Spans[0].ErrorText, test.wantFailure)
			}
			if gotError := trace.ErrorText != ""; gotError != test.wantFailure {
				t.Fatalf("trace error = %q, wantFailure = %t", trace.ErrorText, test.wantFailure)
			}
		})
	}
}

func TestStartupRunsWithRunnerOptions(t *testing.T) {
	root := fstest.MapFS{"startup.php": {Data: []byte("<?php\n// @startup\necho \"ready\";")}}
	var out bytes.Buffer
	startup := annotations.NewStartup(root,
		annotations.WithOutput(&out),
		annotations.WithRunnerOptions(runner.Options{}),
		annotations.WithFlatstack(false),
	)

	if err := startup.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "ready" {
		t.Fatalf("output = %q, want %q", got, "ready")
	}
}

func TestStartupRejectsMissingRootFilesystem(t *testing.T) {
	if err := annotations.NewStartup(nil).Start(context.Background()); err == nil {
		t.Fatal("nil root filesystem was accepted")
	}
}
