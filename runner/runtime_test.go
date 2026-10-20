package runner

import (
	"context"
	"errors"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner/coverage"
	"github.com/titpetric/phpscript/telemetry"
)

// telemetryRecorder is a runtime observer recording into a trace of its own, so
// the tests can assert on what the interpreter reported without a server.
type telemetryRecorder struct {
	states    []telemetry.State
	filenames []string
	traces    []string
	spans     []*telemetry.Span
	included  []int
	trace     *telemetry.Trace
}

func newTelemetryRecorder(t *testing.T) *telemetryRecorder {
	t.Helper()

	traceOptions := telemetry.NewOptions("phpscript")

	// oida records only when the caller sets Enabled; a test that reads
	// its traces back sets it.
	traceOptions.Enabled = true

	tracer, err := telemetry.New(traceOptions)
	if err != nil {
		t.Fatal(err)
	}
	_, trace, err := tracer.StartTrace(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	return &telemetryRecorder{trace: trace}
}

func (r *telemetryRecorder) Trace(ctx context.Context, message string, kind ...telemetry.Kind) *telemetry.Span {
	r.traces = append(r.traces, message)
	span := telemetry.StartSpan(telemetry.WithTrace(ctx, r.trace), message, kind...)
	r.spans = append(r.spans, span)
	return span
}

func (r *telemetryRecorder) UpdateStatus(_ context.Context, state telemetry.State) {
	r.states = append(r.states, state)
}

func (r *telemetryRecorder) UpdateFilename(_ context.Context, filename string) {
	r.filenames = append(r.filenames, filename)
}

func (r *telemetryRecorder) UpdateIncludedFiles(_ context.Context, count int) {
	r.included = append(r.included, count)
}

func TestRuntimeObserverReceivesExecutionPhases(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	recorder := newTelemetryRecorder(t)
	rt := New(&out, Options{})
	rt.Observe(recorder)
	program, err := rt.Load(`<?php $a = 1; echo $a; ?>`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}
	want := []telemetry.State{telemetry.StateStarting, telemetry.StateReading, telemetry.StateProcessing, telemetry.StateWriting}
	if !reflect.DeepEqual(recorder.states, want) {
		t.Fatalf("states = %q, want %q", recorder.states, want)
	}
}

func TestRuntimeObserverReceivesParseError(t *testing.T) {
	t.Parallel()
	recorder := newTelemetryRecorder(t)
	rt := New(nil, Options{})
	rt.Observe(recorder)
	if _, err := rt.Load(`<?php echo ;`); err == nil {
		t.Fatal("expected parse error")
	}
	if got := recorder.states[len(recorder.states)-1]; got != telemetry.StateError {
		t.Fatalf("last state = %q, want E", got)
	}
}

func TestRuntimeObserverReceivesExecutionErrorSpan(t *testing.T) {
	t.Parallel()
	recorder := newTelemetryRecorder(t)
	rt := New(nil, Options{RootFS: fstest.MapFS{
		"main.php": {Data: []byte(`<?php $value = 1; $value["key"] = 2;`)},
	}})
	rt.Observe(recorder)
	program, err := rt.LoadFile("main.php")
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err == nil {
		t.Fatal("expected execution error")
	}
	if got := recorder.states[len(recorder.states)-1]; got != telemetry.StateError {
		t.Fatalf("last state = %q, want E", got)
	}
	if len(recorder.spans) != 1 {
		t.Fatalf("error spans = %+v", recorder.spans)
	}
	// The message is the recorded error, not part of the span name: names stay
	// stable so one kind of failure reads as one kind of failure.
	span := recorder.spans[0]
	if span.Name != "php error" || span.ErrorText != "assign: target is not an array" || span.Filename != "/main.php" || span.Line != 1 {
		t.Fatalf("error span = %+v", span)
	}
}

func TestRuntimeContextCarriesActiveSourceLine(t *testing.T) {
	t.Parallel()
	traceOptions := telemetry.NewOptions("phpscript")
	// oida records only when the caller sets Enabled; a test that reads
	// its traces back sets it.
	traceOptions.Enabled = true
	tracer, err := telemetry.New(traceOptions)
	if err != nil {
		t.Fatal(err)
	}
	ctx, trace, err := tracer.StartTrace(context.Background(), "request")
	if err != nil {
		t.Fatal(err)
	}

	rt := New(nil, Options{RootFS: fstest.MapFS{
		"main.php": {Data: []byte("<?php\n\ninspect_context();")},
	}})
	rt.SetContext(ctx)
	rt.RegisterFunc("inspect_context", func(ctx context.Context) {
		telemetry.StartSpan(ctx, "inspect")
	})
	program, err := rt.LoadFile("main.php")
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}

	span := findSpan(trace, "inspect")
	if span == nil || span.Filename != "/main.php" || span.Line != 3 {
		t.Fatalf("spans = %+v", trace.Clone().Spans)
	}
}

// TestRuntimeContextLineAfterNestedCall covers a binding called in the same
// statement as a PHP call that ran before it, the shape of
// `$this->conn->db()->get(...)`: the call returns into that statement, so the
// binding's span belongs to its line and not to the last line the callee ran.
func TestRuntimeContextLineAfterNestedCall(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		source string
		line   int
	}{
		{
			name: "function result",
			source: `<?php
function pick() {
	return 1;
}
inspect_context(pick());
`,
			line: 5,
		},
		{
			name: "method result",
			source: `<?php
class Conn {
	function db() {
		return 1;
	}
}
$conn = new Conn;
inspect_context($conn->db());
`,
			line: 8,
		},
		{
			name: "closure result",
			source: `<?php
$pick = function () {
	return 1;
};
inspect_context($pick());
`,
			line: 5,
		},
		{
			name: "inside a method",
			source: `<?php
class Store {
	function conn() {
		return 1;
	}
	function read() {
		return inspect_context($this->conn());
	}
}
$store = new Store;
$store->read();
`,
			line: 7,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			traceOptions := telemetry.NewOptions("phpscript")
			traceOptions.Enabled = true
			tracer, err := telemetry.New(traceOptions)
			if err != nil {
				t.Fatal(err)
			}
			ctx, trace, err := tracer.StartTrace(context.Background(), "request")
			if err != nil {
				t.Fatal(err)
			}

			rt := New(nil, Options{RootFS: fstest.MapFS{
				"main.php": {Data: []byte(test.source)},
			}})
			rt.SetContext(ctx)
			rt.RegisterFunc("inspect_context", func(ctx context.Context, _ any) {
				telemetry.StartSpan(ctx, "inspect").End()
			})
			program, err := rt.LoadFile("main.php")
			if err != nil {
				t.Fatal(err)
			}
			if err := rt.Run(program); err != nil {
				t.Fatal(err)
			}

			span := findSpan(trace, "inspect")
			if span == nil || span.Filename != "/main.php" || span.Line != test.line {
				t.Fatalf("inspect span = %+v, want /main.php:%d", span, test.line)
			}
		})
	}
}

// TestRuntimeObserverMethodLineAfterNestedCall covers a PHP method whose
// argument is another call: both spans belong to the statement making them.
func TestRuntimeObserverMethodLineAfterNestedCall(t *testing.T) {
	t.Parallel()
	recorder := newTelemetryRecorder(t)
	rt := New(nil, Options{RootFS: fstest.MapFS{
		"main.php": {Data: []byte(`<?php
class Pair {
	function inner() {
		return 1;
	}
	function outer($value) {
		return $value;
	}
}
$pair = new Pair;
$pair->outer($pair->inner());
`)},
	}})
	rt.Observe(recorder)
	program, err := rt.LoadFile("main.php")
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"pair.inner", "pair.outer"} {
		var found *telemetry.Span
		for _, span := range recorder.spans {
			if span.Name == name {
				found = span
			}
		}
		if found == nil || found.Line != 11 {
			t.Errorf("%s span = %+v, want line 11", name, found)
		}
	}
}

// TestRuntimeObserverErrorLineAfterNestedCall covers an error the statement
// raises after a call in it returned: the error span names the statement.
func TestRuntimeObserverErrorLineAfterNestedCall(t *testing.T) {
	t.Parallel()
	recorder := newTelemetryRecorder(t)
	rt := New(nil, Options{RootFS: fstest.MapFS{
		"main.php": {Data: []byte(`<?php
function pick() {
	return "key";
}
$value = 1;
$value[pick()] = 2;
`)},
	}})
	rt.Observe(recorder)
	program, err := rt.LoadFile("main.php")
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err == nil {
		t.Fatal("expected execution error")
	}
	var found *telemetry.Span
	for _, span := range recorder.spans {
		if span.Name == "php error" {
			found = span
		}
	}
	if found == nil || found.Line != 6 {
		t.Fatalf("error span = %+v, want line 6", found)
	}
}

func TestRuntimeObserverReceivesEntrypointFilename(t *testing.T) {
	t.Parallel()
	recorder := newTelemetryRecorder(t)
	rt := New(nil, Options{RootFS: fstest.MapFS{
		"routes/hello.php": {Data: []byte(`<?php echo "hello";`)},
	}})
	rt.Observe(recorder)
	if _, err := rt.LoadFile("routes/hello.php"); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.LoadFile("routes/hello.php"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"routes/hello.php"}; !reflect.DeepEqual(recorder.filenames, want) {
		t.Fatalf("filenames = %q, want %q", recorder.filenames, want)
	}
}

func TestRuntimeObserverReceivesIncludedFileCount(t *testing.T) {
	t.Parallel()
	recorder := newTelemetryRecorder(t)
	rt := New(nil, Options{RootFS: fstest.MapFS{
		"main.php":   {Data: []byte(`<?php include "one.php"; include "two.php";`)},
		"one.php":    {Data: []byte(`<?php include "nested.tpl";`)},
		"nested.tpl": {Data: []byte(`<?php $nested = 1;`)},
		"two.php":    {Data: []byte(`<?php $two = 2;`)},
	}})
	rt.Observe(recorder)
	program, err := rt.LoadFile("main.php")
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}
	if want := []string{"main.php"}; !reflect.DeepEqual(recorder.filenames, want) {
		t.Fatalf("filenames = %q, want %q", recorder.filenames, want)
	}
	if want := []int{1, 2, 3}; !reflect.DeepEqual(recorder.included, want) {
		t.Fatalf("included = %v, want %v", recorder.included, want)
	}

	// One span per include, measured from the include to its return, rather
	// than an opening and a closing marker to pair up.
	if want := []string{"include one.php", "include nested.tpl", "include two.php"}; !reflect.DeepEqual(recorder.traces, want) {
		t.Fatalf("traces = %q, want %q", recorder.traces, want)
	}
	if len(recorder.spans) != 3 {
		t.Fatalf("include spans = %+v", recorder.spans)
	}
	for i, span := range recorder.spans {
		if !span.Ended() || span.Duration <= 0 {
			t.Fatalf("include span %d = %+v", i, span)
		}
	}

	// A .tpl include is template work; the PHP includes are interpreter work.
	if got := recorder.spans[1].Kind; got != telemetry.KindTemplate {
		t.Fatalf("template include kind = %q", got)
	}
	for _, i := range []int{0, 2} {
		if got := recorder.spans[i].Kind; got != telemetry.KindInternal {
			t.Fatalf("PHP include %d kind = %q", i, got)
		}
	}

	// The runtime records the nested include below the one that ran it.
	if parent, child := recorder.spans[0], recorder.spans[1]; child.ParentID != parent.ID {
		t.Fatalf("nested include parent = %d, want %d", child.ParentID, parent.ID)
	}

	wantFiles := []string{"/main.php", "/one.php", "/main.php"}
	for i, filename := range wantFiles {
		if recorder.spans[i].Filename != filename {
			t.Fatalf("span %d filename = %q, want %q", i, recorder.spans[i].Filename, filename)
		}
	}
}

func TestRuntimeObserverReceivesMeasuredConstructor(t *testing.T) {
	t.Parallel()
	recorder := newTelemetryRecorder(t)
	rt := New(nil, Options{RootFS: fstest.MapFS{
		"main.php": {Data: []byte(`<?php class Example {} $value = new Example;`)},
	}})
	rt.Observe(recorder)
	program, err := rt.LoadFile("main.php")
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}
	if want := []string{"new Example"}; !reflect.DeepEqual(recorder.traces, want) {
		t.Fatalf("traces = %q, want %q", recorder.traces, want)
	}
	if len(recorder.spans) != 1 || recorder.spans[0].Duration <= 0 || recorder.spans[0].Filename != "/main.php" {
		t.Fatalf("constructor spans = %+v", recorder.spans)
	}
}

func TestRuntimeObserverReceivesMeasuredPHPMethod(t *testing.T) {
	t.Parallel()
	recorder := newTelemetryRecorder(t)
	rt := New(nil, Options{RootFS: fstest.MapFS{
		"main.php": {Data: []byte("<?php\ninclude \"Database.php\";\n$db = new Database;\necho $db->get();\n")},
		"Database.php": {Data: []byte(`<?php
class Database {
    function get() { return "result"; }
}
`)},
	}})
	rt.Observe(recorder)
	program, err := rt.LoadFile("main.php")
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}
	var methodSpan *telemetry.Span
	for _, span := range recorder.spans {
		if span.Name == "db.get" {
			methodSpan = span
			break
		}
	}
	if methodSpan == nil || methodSpan.Duration <= 0 || methodSpan.Filename != "/main.php" || methodSpan.Line != 4 {
		t.Fatalf("method spans = %+v", recorder.spans)
	}
}

// findSpan returns the first span of the trace with the given name.
func findSpan(trace *telemetry.Trace, name string) *telemetry.Span {
	for _, span := range trace.Clone().Spans {
		if span.Name == name {
			return span
		}
	}
	return nil
}

// coverageBlock finds the block starting at line in file, so a test asserts on
// position and not on ordering.
func coverageBlock(t *testing.T, blocks []coverage.Block, file string, line int) coverage.Block {
	t.Helper()
	for _, b := range blocks {
		if b.File == file && b.StartLine == line {
			return b
		}
	}
	t.Fatalf("no block for %s:%d in %+v", file, line, blocks)
	return coverage.Block{}
}

// TestCoverageCounts covers what the interpreter charges: a loop body once per
// iteration, the loop header once, and nothing for an untaken branch.
func TestCoverageCounts(t *testing.T) {
	t.Parallel()
	src := `<?php
$total = 0;
for ($i = 0; $i < 3; $i++) {
	$total = $total + $i;
}
if ($total > 100) {
	echo "big";
}
echo $total;
`
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	rt := New(&out, Options{})
	rt.UpdateFilename("main.php")
	cov := coverage.New()
	rt.SetCoverage(cov)
	if err := rt.Run(prog); err != nil {
		t.Fatal(err)
	}
	if out.String() != "3" {
		t.Fatalf("output = %q", out.String())
	}

	blocks := cov.Blocks()
	if got := coverageBlock(t, blocks, "main.php", 2).Count; got != 1 {
		t.Errorf("assignment count = %d, want 1", got)
	}
	if got := coverageBlock(t, blocks, "main.php", 3); got.Count != 1 || got.EndLine != 3 {
		t.Errorf("for header = %+v, want count 1 clamped to its own line", got)
	}
	if got := coverageBlock(t, blocks, "main.php", 4).Count; got != 3 {
		t.Errorf("loop body count = %d, want 3", got)
	}
	if got := coverageBlock(t, blocks, "main.php", 7).Count; got != 0 {
		t.Errorf("untaken branch count = %d, want 0", got)
	}
}

// TestCoverageIncludedFiles covers the zero-count baseline: an included file
// registers every statement, so a function body the fixture never calls shows
// up at zero under the include's own filename.
func TestCoverageIncludedFiles(t *testing.T) {
	t.Parallel()
	root := fstest.MapFS{
		"lib.php": &fstest.MapFile{Data: []byte(`<?php
function used()
{
	return "used";
}

function unused()
{
	return "unused";
}
`)},
	}
	prog, err := parser.Parse(`<?php
include "lib.php";
echo used();
`)
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	rt := New(&out, Options{RootFS: root})
	rt.UpdateFilename("main.php")
	cov := coverage.New()
	rt.SetCoverage(cov)
	if err := rt.Run(prog); err != nil {
		t.Fatal(err)
	}
	if out.String() != "used" {
		t.Fatalf("output = %q", out.String())
	}

	blocks := cov.Blocks()
	if got := coverageBlock(t, blocks, "lib.php", 4).Count; got != 1 {
		t.Errorf("used() body count = %d, want 1", got)
	}
	if got := coverageBlock(t, blocks, "lib.php", 9).Count; got != 0 {
		t.Errorf("unused() body count = %d, want 0", got)
	}
	for _, b := range blocks {
		if b.File == "lib.php" && b.StartLine == 2 {
			t.Errorf("function declaration registered as a block: %+v", b)
		}
	}
}

// TestCoverageForcesInterpreter holds the flatstack boundary: a runtime created
// with NewFlatStack executes interpreted while it holds a collector, so the
// counts exist and the bytecode backend needs no coverage support.
func TestCoverageForcesInterpreter(t *testing.T) {
	t.Parallel()
	prog, err := parser.Parse(`<?php
echo "covered";
`)
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	rt := NewFlatStack(&out, Options{})
	rt.UpdateFilename("main.php")
	cov := coverage.New()
	rt.SetCoverage(cov)
	if err := rt.Run(prog); err != nil {
		t.Fatal(err)
	}
	if out.String() != "covered" {
		t.Fatalf("output = %q", out.String())
	}
	if got := coverageBlock(t, cov.Blocks(), "main.php", 2).Count; got != 1 {
		t.Errorf("echo count = %d, want 1", got)
	}
}

// stubDatabase and stubMail stand in for the providers a host configures. Each
// embeds the interface and declares no method: these tests read which value the
// runtime answers with and never call through it, and implementing Open would
// put the sql driver in this package's test binary for nothing.
type stubDatabase struct{ model.DatabaseProvider }

type stubMail struct{ model.MailProvider }

// TestRuntimeAnswersForItsOptions covers the accessors a binding reads its
// configuration through. Each answers what the host passed and nothing it
// inferred: a runtime with no source root answers nil and no working
// directory, and no provider and no default, because the binding settles what
// a missing one falls back to.
func TestRuntimeAnswersForItsOptions(t *testing.T) {
	t.Parallel()
	bare := New(io.Discard, Options{})
	if got := bare.SAPI(); got != "" {
		t.Errorf("SAPI() = %q on a runtime that named none, want empty", got)
	}
	if got := bare.FS(); got != nil {
		t.Errorf("FS() = %v with no source root, want nil", got)
	}
	if got := bare.Database(); got != nil {
		t.Errorf("Database() = %v with none configured, want nil", got)
	}
	if got := bare.Mail(); got != nil {
		t.Errorf("Mail() = %v with none configured, want nil", got)
	}
	if got := bare.MemoryLimit(); got != 0 {
		t.Errorf("MemoryLimit() = %s with none configured, want no limit", got)
	}
	if bare.Precompiled() {
		t.Error("Precompiled() is true on a runtime that was not configured to")
	}
	if got := bare.IncludePath(); got != "." {
		t.Errorf("IncludePath() = %q, want the working directory", got)
	}

	root := fstest.MapFS{"app/index.php": &fstest.MapFile{Data: []byte(`<?php`)}}
	stdin := strings.NewReader("piped")
	database, mail := &stubDatabase{}, &stubMail{}
	rt := New(io.Discard, Options{
		SAPI:          "cgi-phpscript",
		RootFS:        root,
		Stdin:         stdin,
		WorkDir:       "app",
		MemoryLimit:   8 << 20,
		Precompile:    true,
		Database:      database,
		Mail:          mail,
		WritablePaths: []string{"var", "tmp"},
	})

	if got := rt.SAPI(); got != "cgi-phpscript" {
		t.Errorf("SAPI() = %q, want the configured one", got)
	}
	if got := rt.FS(); got == nil {
		t.Error("FS() = nil with a source root configured")
	}
	if got := rt.Stdin(); got != stdin {
		t.Errorf("Stdin() = %v, want the configured reader", got)
	}
	if got := rt.Database(); got != model.DatabaseProvider(database) {
		t.Errorf("Database() = %v, want the configured provider", got)
	}
	if got := rt.Mail(); got != model.MailProvider(mail) {
		t.Errorf("Mail() = %v, want the configured provider", got)
	}
	if got := rt.MemoryLimit().Bytes(); got != 8<<20 {
		t.Errorf("MemoryLimit() = %d, want %d", got, 8<<20)
	}
	if !rt.Precompiled() {
		t.Error("Precompiled() is false on a runtime configured to precompile")
	}

	// The working directory has two spellings: the fs.FS path a binding
	// resolves against, and the one a script reads out of getcwd().
	if got := rt.WorkDir(); got != "app" {
		t.Errorf("WorkDir() = %q, want the fs path", got)
	}
	if got := rt.WorkDirPath(); got != "/app" {
		t.Errorf("WorkDirPath() = %q, want the path a script reads", got)
	}

	// WritablePaths answers a copy, so a binding that sorts or filters what
	// it holds does not edit the host's configuration.
	paths := rt.WritablePaths()
	if len(paths) != 2 {
		t.Fatalf("WritablePaths() = %v, want both", paths)
	}
	paths[0] = "edited"
	if again := rt.WritablePaths(); again[0] != "var" {
		t.Errorf("WritablePaths() = %v after the caller edited its copy, want the configured list", again)
	}
}

// TestRuntime_SetWorkDir covers chdir(): per-runtime state, so a script moving
// it moves nothing another request sees and os.Chdir is never called. A
// relative path resolves against the current directory, an absolute one
// against the source root, and neither climbs out of it. A path naming no
// directory is the false php documents.
func TestRuntime_SetWorkDir(t *testing.T) {
	t.Parallel()
	rt := New(io.Discard, Options{RootFS: fstest.MapFS{
		"app/lib/helper.php": &fstest.MapFile{Data: []byte(`<?php`)},
		"app/index.php":      &fstest.MapFile{Data: []byte(`<?php`)},
	}})

	if !rt.SetWorkDir("app") {
		t.Fatal("SetWorkDir(app) refused a directory the root holds")
	}
	if got := rt.WorkDir(); got != "app" {
		t.Errorf("WorkDir() = %q, want app", got)
	}
	// A relative path resolves through the directory that is current now, so a
	// script's include and its file_get_contents name the same file.
	if got := rt.ResolvePath("index.php"); got != "app/index.php" {
		t.Errorf("ResolvePath(index.php) = %q, want app/index.php", got)
	}

	if !rt.SetWorkDir("lib") {
		t.Fatal("SetWorkDir(lib) refused a directory below the current one")
	}
	if got := rt.ResolvePath("helper.php"); got != "app/lib/helper.php" {
		t.Errorf("ResolvePath(helper.php) = %q, want app/lib/helper.php", got)
	}

	// SetWorkDir reads an absolute path from the source root and no host path.
	if !rt.SetWorkDir("/app") {
		t.Fatal("SetWorkDir(/app) refused the root-relative spelling")
	}
	if got := rt.WorkDirPath(); got != "/app" {
		t.Errorf("WorkDirPath() = %q, want /app", got)
	}

	// SetWorkDir refuses a path naming no directory and leaves the current one
	// alone, which is the false php documents.
	for _, dir := range []string{"nope", "index.php", "app/index.php"} {
		if rt.SetWorkDir(dir) {
			t.Errorf("SetWorkDir(%q) was accepted", dir)
		}
		if got := rt.WorkDir(); got != "app" {
			t.Fatalf("SetWorkDir(%q) moved the directory to %q", dir, got)
		}
	}

	// SetWorkDir clamps a climb and never refuses it: the path resolves to the
	// source root, so a script that walked up as far as it liked is standing at
	// the top of the tree the host gave it and never above one.
	if !rt.SetWorkDir("../../..") {
		t.Fatal("SetWorkDir refused to climb to the root")
	}
	if got := rt.WorkDir(); got != "." {
		t.Errorf("WorkDir() = %q after climbing, want the source root", got)
	}
	if got := rt.ResolvePath("app/index.php"); got != "app/index.php" {
		t.Errorf("ResolvePath = %q from the root, want app/index.php", got)
	}
}

// TestRuntimeSymbolTables covers what the introspection functions read. Each
// answers a sorted snapshot, because php guarantees no order and a diagnostic
// that reads differently per run is one nothing can diff.
func TestRuntimeSymbolTables(t *testing.T) {
	t.Parallel()
	rt := New(io.Discard, Options{})
	rt.RegisterFunc("host_shim", func(args ...any) any { return nil })
	rt.RegisterConstructor("Storage", func() any { return nil })

	program, err := rt.Load(`<?php
interface Greets { public function hello(); }
class Site { public function hello() { return "hei"; } }
function declared_in_php() { return 1; }
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}

	internal, user := rt.DefinedFunctions()
	if !slices.Contains(internal, "host_shim") {
		t.Errorf("DefinedFunctions internal = %v, want the registered shim among them", internal)
	}
	if !slices.Contains(user, "declared_in_php") {
		t.Errorf("DefinedFunctions user = %v, want the php declaration among them", user)
	}
	if slices.Contains(internal, "declared_in_php") {
		t.Error("a php declaration is reported as internal")
	}
	if !slices.IsSorted(internal) || !slices.IsSorted(user) {
		t.Error("DefinedFunctions answered an unsorted snapshot")
	}

	classes := rt.DeclaredClasses()
	for _, want := range []string{"Site", "Storage", "stdClass"} {
		if !slices.Contains(classes, want) {
			t.Errorf("DeclaredClasses = %v, want %s among them", classes, want)
		}
	}
	if !slices.IsSorted(classes) {
		t.Error("DeclaredClasses answered an unsorted snapshot")
	}

	if _, ok := rt.LookupFunc("host_shim"); !ok {
		t.Error("LookupFunc did not find the registered shim")
	}
	if _, ok := rt.LookupFunc("nothing_registered_this"); ok {
		t.Error("LookupFunc found a name nothing registered")
	}
	if _, ok := rt.LookupConstructor("Storage"); !ok {
		t.Error("LookupConstructor did not find the registered constructor")
	}
	if _, ok := rt.LookupConstructor("Site"); ok {
		t.Error("LookupConstructor found a php class, which has no Go constructor")
	}

	// Method lookup is php's, which is case-insensitive, and answers only for
	// an interpreted class: a caller reflects over a host-backed one itself.
	if !rt.MethodExists("Site", "HELLO") {
		t.Error("MethodExists is case-sensitive, want php's case-insensitive lookup")
	}
	if rt.MethodExists("Site", "absent") || rt.MethodExists("Storage", "Get") {
		t.Error("MethodExists answered for a method the class does not declare")
	}

	declared, err := rt.InterfaceExists("Greets", false)
	if err != nil {
		t.Fatal(err)
	}
	if !declared {
		t.Error("InterfaceExists did not find a declared interface")
	}
	absent, err := rt.InterfaceExists("Absent", false)
	if err != nil {
		t.Fatal(err)
	}
	if absent {
		t.Error("InterfaceExists found an interface nothing declares")
	}
}

// TestRuntime_IncludeFile covers the include a host performs itself, the call
// composer's autoloader pulls a declaration file in with: the file evaluates
// in a fresh scope the way an `include` statement would, and the runtime
// records it among the files it included, so the count a script reads and the
// count it publishes agree.
func TestRuntime_IncludeFile(t *testing.T) {
	t.Parallel()
	recorder := newTelemetryRecorder(t)
	rt := New(io.Discard, Options{RootFS: fstest.MapFS{
		"lib/site.php": &fstest.MapFile{Data: []byte(`<?php class Site {} return "loaded";`)},
	}})
	rt.Observe(recorder)

	value, err := rt.IncludeFile("lib/site.php")
	if err != nil {
		t.Fatalf("IncludeFile: %v", err)
	}
	if value != "loaded" {
		t.Errorf("IncludeFile answered %v, want the file's return value", value)
	}
	if declared, err := rt.ClassExists("Site", false); err != nil || !declared {
		t.Error("the class the included file declares is not defined")
	}

	// The runtime writes the record from the source filesystem's root, the
	// spelling every path a script reads carries.
	included := rt.IncludedFiles()
	if !slices.Contains(included, "/lib/site.php") {
		t.Errorf("IncludedFiles = %v, want the file among them", included)
	}
	// The snapshot is a copy, so a caller walking it cannot edit the record.
	included[0] = "edited"
	if again := rt.IncludedFiles(); slices.Contains(again, "edited") {
		t.Errorf("IncludedFiles = %v after the caller edited its copy", again)
	}

	// UpdateIncludedFiles publishes the count to an observer; a host calls it
	// for the files it included outside the interpreter.
	rt.UpdateIncludedFiles(len(rt.IncludedFiles()))
	if len(recorder.included) == 0 {
		t.Fatal("no included-file count reached the observer")
	}
	if got := recorder.included[len(recorder.included)-1]; got != len(rt.IncludedFiles()) {
		t.Errorf("the observer was told %d files, want %d", got, len(rt.IncludedFiles()))
	}

	if _, err := rt.IncludeFile("lib/absent.php"); err == nil {
		t.Error("IncludeFile of a file the root does not hold reported no error")
	}
}

// TestRuntime_CompiledTree covers what the parsed tree costs, which phpinfo()
// reports and memory_limit does not charge for: every request shares the tree
// and it outlives each one. The heap figure is zero until a precompile pass
// measures one, because printing 0.00 MiB over a filled cache would read as a
// measurement and not the absence of one.
func TestRuntime_CompiledTree(t *testing.T) {
	t.Parallel()
	rt := New(io.Discard, Options{
		Precompile: true,
		RootFS: fstest.MapFS{
			"lib.php": &fstest.MapFile{Data: []byte(`<?php function helper($a) { return $a * 2; }`)},
		},
	})

	programs, expressions, heap := rt.CompiledTree()
	if programs != 0 || expressions != 0 {
		t.Fatalf("CompiledTree = %d programs, %d expressions on a runtime that loaded nothing", programs, expressions)
	}
	if heap != 0 {
		t.Errorf("CompiledTree heap = %d with no precompile pass behind it, want 0", heap)
	}

	if _, err := rt.LoadFile("lib.php"); err != nil {
		t.Fatal(err)
	}
	if programs, _, _ = rt.CompiledTree(); programs != 1 {
		t.Errorf("CompiledTree = %d programs after one load, want 1", programs)
	}
}

// TestRuntime_Reset covers what a pool does between two programs: ResetSession
// plus the caches keyed by AST node, which hold the previous tree alive. A
// runtime handed to the next program must not answer for the last one's
// classes, constants or parsed files.
func TestRuntime_Reset(t *testing.T) {
	t.Parallel()
	var first strings.Builder
	rt := New(&first, Options{
		Precompile: true,
		RootFS: fstest.MapFS{
			"lib.php": &fstest.MapFile{Data: []byte(`<?php class Site {}`)},
		},
	})
	// The constants a host froze are what a reset returns to, so the bindings
	// stay defined and whatever the program added does not. There is no
	// define() to use here: the bindings that register one import this
	// package.
	rt.SetConst("STDLIB_CONST", int64(1))
	rt.FreezeStdlib()
	rt.SetConst("LOADED", int64(2))

	program, err := rt.LoadFile("lib.php")
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}
	if programs, _, _ := rt.CompiledTree(); programs == 0 {
		t.Fatal("nothing was cached, so the clear below proves nothing")
	}

	var second strings.Builder
	rt.Reset(&second, nil)

	if programs, expressions, _ := rt.CompiledTree(); programs != 0 || expressions != 0 {
		t.Errorf("CompiledTree = %d programs, %d expressions after Reset, want both cleared", programs, expressions)
	}
	if declared, err := rt.ClassExists("Site", false); err != nil || declared {
		t.Error("a class the previous program declared survived Reset")
	}
	if _, defined := rt.Const("LOADED"); defined {
		t.Error("a constant added past the freeze survived Reset")
	}
	if _, defined := rt.Const("STDLIB_CONST"); !defined {
		t.Error("a frozen constant was cleared by Reset, so the bindings would be undefined")
	}

	// Output goes to the writer Reset holds, so the next program's text
	// does not join the previous one's.
	echo, err := rt.Load(`<?php echo "second";`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(echo); err != nil {
		t.Fatal(err)
	}
	if got := second.String(); got != "second" {
		t.Errorf("the second program wrote %q to the new writer, want %q", got, "second")
	}
	if strings.Contains(first.String(), "second") {
		t.Error("the second program wrote to the first program's writer")
	}
}

// TestRuntimeMemoryReporting covers the figures memory_get_usage() and
// memory_get_peak_usage() answer with. Both come off a walk of the live roots,
// so the usage is what is reachable now and the peak is the highest a walk has
// seen: an allocation made and released between two walks does not raise it,
// and php's allocator-level peak counts it.
func TestRuntimeMemoryReporting(t *testing.T) {
	t.Parallel()
	rt := New(io.Discard, Options{})

	base := rt.MemoryUsage()
	if base <= 0 {
		t.Fatalf("MemoryUsage() = %d, want the baseline a runtime starts at", base)
	}
	if got := rt.MemoryWalk(); got != base {
		t.Errorf("MemoryWalk() = %d against MemoryUsage()'s %d, want one figure", got, base)
	}
	if got := rt.MemoryPeak(); got < base {
		t.Errorf("MemoryPeak() = %d, below the %d in use", got, base)
	}

	// A host folds the request's own cost into the baseline, so the walk starts
	// from what the request already weighs.
	rt.AccountRequest(strings.Repeat("x", 4096))
	grown := rt.MemoryWalk()
	if grown <= base {
		t.Errorf("MemoryWalk() = %d after accounting a 4k value, want more than the %d baseline", grown, base)
	}

	// The peak is at least what the last walk saw.
	peak := rt.MemoryPeak()
	if peak < grown {
		t.Errorf("MemoryPeak() = %d, below the %d the last walk measured", peak, grown)
	}

	// A reset takes it back to the baseline, because the figure belonged to the
	// program that ran: a pooled runtime reporting the previous script's peak
	// would charge it to a page that never allocated any of it.
	rt.Reset(io.Discard, nil)
	if got := rt.MemoryPeak(); got >= peak {
		t.Errorf("MemoryPeak() = %d after a reset, want it back below the %d the last program reached", got, peak)
	}
}

// TestRuntime_Exit covers the value `exit` ends a script with: an error, so it
// unwinds the interpreter, and an ending and no failure, which a host reads
// through IsExit without naming the type.
func TestRuntime_Exit(t *testing.T) {
	t.Parallel()
	rt := New(io.Discard, Options{})

	err := rt.Exit(3)
	if err == nil {
		t.Fatal("Exit answered no error, so nothing would unwind")
	}
	exit, ok := IsExit(err)
	if !ok {
		t.Fatalf("IsExit(%v) = false, want the exit recognised", err)
	}
	if exit.Code != 3 {
		t.Errorf("exit code = %d, want 3", exit.Code)
	}
	if got, want := err.Error(), "exit(3)"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}

	// A zero is the ordinary ending and still an ExitError, so a host reading
	// the status does not have to tell the two apart.
	if exit, ok := IsExit(rt.Exit(0)); !ok || exit.Code != 0 {
		t.Error("exit(0) is not recognised as an exit")
	}
}

// TestRuntime_SetConst covers a constant registered from Go, which is visible
// in every scope the way define() is and unlike a global, which php confines to
// the global one.
func TestRuntime_SetConst(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	rt := New(&out, Options{})
	rt.SetConst("HOST_NAME", "phpscript")

	if value, defined := rt.Const("HOST_NAME"); !defined || value != "phpscript" {
		t.Fatalf("Const(HOST_NAME) = %v, %v, want the registered value", value, defined)
	}

	program, err := rt.Load(`<?php function inside() { echo HOST_NAME; } inside();`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "phpscript" {
		t.Errorf("output = %q, want the constant read inside a function", got)
	}

	// A second registration replaces the value, the call a host swapping one
	// between programs makes.
	rt.SetConst("HOST_NAME", "replaced")
	if value, _ := rt.Const("HOST_NAME"); value != "replaced" {
		t.Errorf("Const(HOST_NAME) = %v after a second SetConst, want the new value", value)
	}
}

// TestRuntime_RegisterShutdown covers the callbacks php runs at the end of a
// program, in registration order, so a registered flush reaches the output of
// a script that already finished. The run consumes each, so the next program
// on a pooled runtime does not run the previous one's.
func TestRuntime_RegisterShutdown(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	rt := New(&out, Options{})

	var ran []string
	rt.RegisterShutdown(func() any { ran = append(ran, "first"); return nil })
	rt.RegisterShutdown(func() any { ran = append(ran, "second"); return nil })

	program, err := rt.Load(`<?php echo "ran";`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := strings.Join(ran, ","); got != "first,second" {
		t.Errorf("the callbacks ran %q, want them in registration order", got)
	}
	if got := out.String(); got != "ran" {
		t.Errorf("output = %q, want the program's own text", got)
	}

	ran = nil
	if err := rt.Run(program); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(ran) != 0 {
		t.Errorf("the callbacks ran again on the next program: %v", ran)
	}

	// An error a callback reports reaches the host, because a shutdown that
	// could not finish is a run that did not.
	rt.RegisterShutdown(func() (any, error) { return nil, errors.New("the flush failed") })
	if err := rt.Run(program); err == nil {
		t.Error("a failing shutdown callback did not reach the host")
	}
}

// TestRuntime_Trace and TestRuntime_UpdateStatus cover the two publications a
// host observes a run through. The runtime exports both for the phases that
// happen outside PHP execution, where a serving host spends its time.
func TestRuntime_Trace(t *testing.T) {
	t.Parallel()
	recorder := newTelemetryRecorder(t)
	rt := New(io.Discard, Options{})
	rt.Observe(recorder)

	span := rt.Trace("host work", telemetry.KindInternal)
	if span == nil {
		t.Fatal("Trace answered no span from an observer that creates them")
	}
	if !slices.Contains(recorder.traces, "host work") {
		t.Errorf("the observer recorded %v, want the message among them", recorder.traces)
	}

	// A runtime nothing observes answers a nil span, and every method on one
	// does nothing, so a host traces unconditionally.
	if span := New(io.Discard, Options{}).Trace("unobserved"); span != nil {
		t.Errorf("Trace answered %v with no observer, want nil", span)
	}
}

func TestRuntime_UpdateStatus(t *testing.T) {
	t.Parallel()
	recorder := newTelemetryRecorder(t)
	rt := New(io.Discard, Options{})
	rt.Observe(recorder)

	before := len(recorder.states)
	rt.UpdateStatus(telemetry.StateWriting)
	if len(recorder.states) != before+1 {
		t.Fatalf("the observer received %d states, want one more than %d", len(recorder.states), before)
	}
	if got := recorder.states[len(recorder.states)-1]; got != telemetry.StateWriting {
		t.Errorf("the observer was told %v, want %v", got, telemetry.StateWriting)
	}

	// The same state twice is one phase, so a host calling it per statement
	// does not flood an observer with what it already knows.
	rt.UpdateStatus(telemetry.StateWriting)
	if len(recorder.states) != before+1 {
		t.Errorf("a repeated state was published again: %v", recorder.states)
	}
}

// TestFilenameObserver and TestIncludeObserver cover the two optional halves of
// the observer contract. Each is a separate interface so an observer that wants
// neither implements neither, and the runtime asks before publishing.
func TestFilenameObserver(t *testing.T) {
	t.Parallel()
	recorder := newTelemetryRecorder(t)
	var observer FilenameObserver = recorder

	rt := New(io.Discard, Options{})
	rt.Observe(recorder)
	rt.UpdateFilename("app/index.php")

	if !slices.Contains(recorder.filenames, "app/index.php") {
		t.Errorf("the observer recorded %v, want the entrypoint among them", recorder.filenames)
	}
	observer.UpdateFilename(context.Background(), "direct")
	if !slices.Contains(recorder.filenames, "direct") {
		t.Error("the interface method did not reach the recorder")
	}

	// The runtime publishes the first entrypoint only: an include is not a new
	// one, and the span named for the run's first file has to keep that name.
	rt.UpdateFilename("app/other.php")
	if slices.Contains(recorder.filenames, "app/other.php") {
		t.Errorf("a second entrypoint was published: %v", recorder.filenames)
	}

	// An observer implementing neither optional half is never called for one.
	plain := &statusOnlyObserver{}
	bare := New(io.Discard, Options{})
	bare.Observe(plain)
	bare.UpdateFilename("app/index.php")
	bare.UpdateIncludedFiles(1)
}

func TestIncludeObserver(t *testing.T) {
	t.Parallel()
	recorder := newTelemetryRecorder(t)
	var observer IncludeObserver = recorder

	rt := New(io.Discard, Options{})
	rt.Observe(recorder)
	rt.UpdateIncludedFiles(3)

	if !slices.Contains(recorder.included, 3) {
		t.Errorf("the observer recorded %v, want the count among them", recorder.included)
	}
	observer.UpdateIncludedFiles(context.Background(), 9)
	if !slices.Contains(recorder.included, 9) {
		t.Error("the interface method did not reach the recorder")
	}
}

// statusOnlyObserver implements the required half of the observer contract and
// neither optional one, the shape the runtime has to tolerate.
type statusOnlyObserver struct{}

func (*statusOnlyObserver) UpdateStatus(context.Context, telemetry.State) {}

func (*statusOnlyObserver) Trace(context.Context, string, ...telemetry.Kind) *telemetry.Span {
	return nil
}

// TestScriptEnvironment covers what getenv() reads. A nil environment is the
// process one, as a CLI run has it; a host that configured one passes it
// instead, and a virtual host always does, so a site sees the variables it
// declared and none of the operator's. ScriptEnvironment holds the
// infrastructure names back either way.
func TestScriptEnvironment(t *testing.T) {
	configured := ScriptEnvironment([]string{
		"APP_ENV=production",
		"EMPTY=",
		InfrastructurePrefix + "SECRET=held back",
		"=no name",
		"HAS=an=equals",
	})

	if got := configured["APP_ENV"]; got != "production" {
		t.Errorf("APP_ENV = %q, want production", got)
	}
	if value, present := configured["EMPTY"]; !present || value != "" {
		t.Errorf("EMPTY = %q, %v, want an empty value that is present", value, present)
	}
	if got := configured["HAS"]; got != "an=equals" {
		t.Errorf("HAS = %q, want everything past the first equals", got)
	}
	if _, present := configured[InfrastructurePrefix+"SECRET"]; present {
		t.Error("an infrastructure variable reached the script environment")
	}
	if _, present := configured[""]; present {
		t.Error("an entry with no name reached the script environment")
	}

	// A configured environment is the whole of what a script sees, so a
	// variable the process carries and the host did not declare is absent.
	t.Setenv("PHPSCRIPT_TEST_ONLY_IN_PROCESS", "1")
	if _, present := configured["PHPSCRIPT_TEST_ONLY_IN_PROCESS"]; present {
		t.Error("a process variable reached a configured environment")
	}
	if _, present := ScriptEnvironment(nil)["PHPSCRIPT_TEST_ONLY_IN_PROCESS"]; !present {
		t.Error("a nil environment did not read the process one")
	}
}

// TestScopeBuiltins covers the names installed per-environment and no entry in
// the function table. function_exists() and the linter both read that table, so
// a call to one of these would read as a call to something undefined if the
// list stayed unpublished.
func TestScopeBuiltins(t *testing.T) {
	t.Parallel()
	builtins := ScopeBuiltins()
	if len(builtins) == 0 {
		t.Fatal("ScopeBuiltins answered nothing")
	}
	if !slices.Contains(builtins, "func_get_args") {
		t.Errorf("ScopeBuiltins = %v, want func_get_args among them", builtins)
	}

	// Each name is resolvable in a scope and absent from the function table,
	// which is the split the list exists to bridge.
	rt := New(io.Discard, Options{})
	for _, name := range builtins {
		if _, ok := scopeBuiltin(name, NewScope()); !ok {
			t.Errorf("%s is listed and does not resolve in a scope", name)
		}
		if _, ok := rt.LookupFunc(name); ok {
			t.Errorf("%s is in the function table, so it needs no listing", name)
		}
	}
}

// TestRuntime_PutEnv covers the copy-on-write over the frozen environment:
// a write never lands in the snapshot FreezeStdlib took, and a reset restores
// the snapshot whole.
func TestRuntime_PutEnv(t *testing.T) {
	rt := New(io.Discard, Options{Env: []string{"KEPT=host"}})
	rt.FreezeStdlib()

	rt.PutEnv("ADDED", "x")
	if rt.Env["ADDED"] != "x" || rt.Env["KEPT"] != "host" {
		t.Fatalf("after PutEnv, Env = %v", rt.Env)
	}
	rt.UnsetEnv("KEPT")
	if _, ok := rt.Env["KEPT"]; ok {
		t.Fatal("UnsetEnv left the variable in place")
	}

	rt.ResetSession(io.Discard, nil)
	if _, ok := rt.Env["ADDED"]; ok {
		t.Fatal("a PutEnv write survived the reset")
	}
	if rt.Env["KEPT"] != "host" {
		t.Fatal("the reset did not restore the frozen environment")
	}
}

// TestRuntime_OnReset covers the reset hooks: each registered one runs once
// per reset, so a binding's own state leaves with the program's.
func TestRuntime_OnReset(t *testing.T) {
	rt := New(io.Discard, Options{})
	var calls int
	rt.OnReset(func() { calls++ })
	rt.ResetSession(io.Discard, nil)
	if calls != 1 {
		t.Fatalf("the reset ran the hook %d times, want 1", calls)
	}
}
