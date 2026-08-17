package startup

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/telemetry"
)

func TestModuleRunsAnnotatedPHPFilesInPathOrder(t *testing.T) {
	root := fstest.MapFS{
		"20-second.php": {Data: []byte("<?php\n# @startup:\necho \"second\";")},
		"10-first.php":  {Data: []byte("<?php\n// @startup\necho \"first,\";")},
		"ignored.php":   {Data: []byte("<?php\necho \"ignored\";")},
		"annotated.txt": {Data: []byte("// @startup")},
	}
	var out bytes.Buffer
	module := NewModule(root, "", &out, runner.Options{}, false)

	if err := module.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "first,second" {
		t.Fatalf("output = %q, want %q", got, "first,second")
	}
}

func TestModuleProvidesProjectFilesystem(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/startup.php", []byte(`<?php
/* @startup */
$file = fopen("started.txt", "w");
fwrite($file, "ready");
fclose($file);
`), 0o600); err != nil {
		t.Fatal(err)
	}
	module := NewModule(os.DirFS(dir), dir, nil, runner.Options{}, false)

	if err := module.Start(context.Background()); err != nil {
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

func TestModuleReturnsAnnotatedScriptError(t *testing.T) {
	root := fstest.MapFS{
		"broken.php": {Data: []byte("<?php\n// @startup\nmissing_function();")},
	}
	module := NewModule(root, "", &bytes.Buffer{}, runner.Options{}, false)

	err := module.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "startup broken.php") {
		t.Fatalf("error = %v, want startup file error", err)
	}
}

func TestModuleRecordsStartupSuccessAndFailure(t *testing.T) {
	for _, test := range []struct {
		name      string
		source    string
		wantError bool
	}{
		{name: "success", source: "<?php\n// @startup\necho 'ready';"},
		{name: "failure", source: "<?php\n// @startup\nmissing_function();", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder, err := telemetry.NewModule(telemetry.NewOptions())
			if err != nil {
				t.Fatal(err)
			}
			root := fstest.MapFS{"startup.php": {Data: []byte(test.source)}}
			module := NewModule(root, "", &bytes.Buffer{}, runner.Options{}, false, recorder)

			err = module.Start(context.Background())
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, wantError = %t", err, test.wantError)
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
			if gotError := trace.Spans[0].Error != ""; gotError != test.wantError {
				t.Fatalf("span error = %q, wantError = %t", trace.Spans[0].Error, test.wantError)
			}
			if gotError := trace.Error != ""; gotError != test.wantError {
				t.Fatalf("trace error = %q, wantError = %t", trace.Error, test.wantError)
			}
		})
	}
}

func TestAnnotatedRequiresCommentAnnotation(t *testing.T) {
	if Annotated([]byte(`<?php echo "// @startup";`)) {
		t.Fatal("annotation inside PHP string was detected")
	}
	if !Annotated([]byte("<?php\n/**\n * @startup\n */")) {
		t.Fatal("docblock annotation was not detected")
	}
}
