package runner

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

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

	// oida records only when it is asked to; a test that reads its

	// traces back is asking.

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
	traceOptions := telemetry.NewOptions("phpscript")
	// oida records only when it is asked to; a test that reads its
	// traces back is asking.
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

func TestRuntimeObserverReceivesEntrypointFilename(t *testing.T) {
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

	// The nested include is recorded below the one that ran it.
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
// position rather than on ordering.
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

// TestCoverageCounts covers the count semantics: a loop body is charged once
// per iteration, the loop header once, and an untaken branch stays at zero.
func TestCoverageCounts(t *testing.T) {
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
// with NewFlatStack executes interpreted while a collector is installed, so the
// counts exist and no coverage support is needed in the bytecode backend.
func TestCoverageForcesInterpreter(t *testing.T) {
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
