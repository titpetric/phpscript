package startup

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/status"
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
			serverStatus := status.NewModule(status.NewOptions())
			root := fstest.MapFS{"startup.php": {Data: []byte(test.source)}}
			module := NewModule(root, "", &bytes.Buffer{}, runner.Options{}, false, serverStatus)

			err := module.Start(context.Background())
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, wantError = %t", err, test.wantError)
			}
			snapshot := serverStatus.Snapshot()
			if snapshot.Total != 1 || snapshot.Active != 0 || len(snapshot.Log) != 1 {
				t.Fatalf("unexpected status snapshot: %+v", snapshot)
			}
			entry := snapshot.Log[0]
			if entry.Request != "@startup startup.php" || entry.Method != "STARTUP" || entry.Filename != "startup.php" {
				t.Fatalf("unexpected startup entry: %+v", entry)
			}
			if len(entry.Spans) < 2 || entry.Spans[0].Message != "@startup startup.php" || !entry.Spans[0].Open {
				t.Fatalf("unexpected startup spans: %+v", entry.Spans)
			}
			if gotError := entry.Spans[0].Error != ""; gotError != test.wantError {
				t.Fatalf("span error = %q, wantError = %t", entry.Spans[0].Error, test.wantError)
			}
		})
	}
}

func TestAnnotatedRequiresCommentAnnotation(t *testing.T) {
	if annotated([]byte(`<?php echo "// @startup";`)) {
		t.Fatal("annotation inside PHP string was detected")
	}
	if !annotated([]byte("<?php\n/**\n * @startup\n */")) {
		t.Fatal("docblock annotation was not detected")
	}
}
