package runner

import (
	"context"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/telemetry"
)

// telemetryRecorder is a runtime observer recording into a separate trace and
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
// `$this->conn->db()->get(...)`: the call returns into that statement and the
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

// includeTree is the tree the include tests run: an entrypoint including two
// files, one of which includes a template.
var includeTree = fstest.MapFS{
	"main.php":   {Data: []byte(`<?php include "one.php"; include "two.php";`)},
	"one.php":    {Data: []byte(`<?php include "nested.tpl";`)},
	"nested.tpl": {Data: []byte(`<?php $nested = 1;`)},
	"two.php":    {Data: []byte(`<?php $two = 2;`)},
}

// TestRuntimeObserverReceivesIncludedFileCount covers the count, which every
// level publishes, and the span per include, which is the debug level's: the
// default collapses those into one span, covered below.
func TestRuntimeObserverReceivesIncludedFileCount(t *testing.T) {
	t.Parallel()
	recorder := newTelemetryRecorder(t)
	rt := New(nil, Options{RootFS: includeTree, TraceLevel: TraceLevelDebug})
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

	// One span per include, measured from the include to its return. No
	// opening and closing marker to pair up.
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

// TestRuntimeObserverCollapsesIncludes covers what the default level reports
// of a request's includes: one span at the end naming the files, where the
// debug level publishes a span per file. An application that autoloads a class
// per file includes dozens of them, and a name and a microsecond each turns
// the trace into a file listing.
func TestRuntimeObserverCollapsesIncludes(t *testing.T) {
	t.Parallel()
	recorder := newTelemetryRecorder(t)
	rt := New(nil, Options{RootFS: includeTree})
	rt.Observe(recorder)
	program, err := rt.LoadFile("main.php")
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}

	if want := []string{"includes"}; !reflect.DeepEqual(recorder.traces, want) {
		t.Fatalf("traces = %q, want %q", recorder.traces, want)
	}
	span := recorder.spans[len(recorder.spans)-1]
	if !span.Ended() {
		t.Errorf("the inventory span = %+v, want it ended where the run did", span)
	}
	if span.Debug {
		t.Error("the inventory span is marked as detail, which a minimum duration would drop")
	}
	if got := span.Attributes["count"]; got != 3 {
		t.Errorf("count = %v, want every file the run included", got)
	}
	want := []string{"/one.php", "/nested.tpl", "/two.php"}
	if got, ok := span.Attributes["files"].([]string); !ok || !reflect.DeepEqual(got, want) {
		t.Errorf("files = %v, want %q", span.Attributes["files"], want)
	}

	// The count still arrives per include, as the attribute on the request
	// span, which a reader sees without opening the inventory.
	if want := []int{1, 2, 3}; !reflect.DeepEqual(recorder.included, want) {
		t.Fatalf("included = %v, want %v", recorder.included, want)
	}
}

// TestRuntimeObserverTraceLevelMarksFrames covers the mark the default level
// puts on a frame, which is the whole of how a trace of an interpreter stays
// readable: the recorder keeps the slow frames and drops the rest,
// and the debug level asks it to keep them all.
func TestRuntimeObserverTraceLevelMarksFrames(t *testing.T) {
	t.Parallel()
	tests := []struct {
		level TraceLevel
		debug bool
	}{
		{level: "", debug: true},
		{level: TraceLevelInfo, debug: true},
		{level: TraceLevelDebug, debug: false},
	}
	for _, test := range tests {
		t.Run(string(test.level.Level()), func(t *testing.T) {
			t.Parallel()
			recorder := newTelemetryRecorder(t)
			rt := New(nil, Options{TraceLevel: test.level, RootFS: fstest.MapFS{
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
			if len(recorder.spans) != 1 {
				t.Fatalf("spans = %+v, want the constructor alone", recorder.spans)
			}
			if got := recorder.spans[0].Debug; got != test.debug {
				t.Errorf("the constructor span's debug mark = %v, want %v", got, test.debug)
			}
		})
	}
}

// TestRuntimeObserverTraceLevelOff covers the level that silences the
// interpreter: no frame, no include, no inventory. What a binding or a script
// measured is not the interpreter's own work and still records. The error of
// a run that failed records too.
func TestRuntimeObserverTraceLevelOff(t *testing.T) {
	t.Parallel()
	recorder := newTelemetryRecorder(t)
	rt := New(nil, Options{RootFS: includeTree, TraceLevel: TraceLevelOff})
	rt.SetContext(telemetry.WithTrace(context.Background(), recorder.trace))
	rt.Observe(recorder)
	rt.RegisterFunc("measure", func(ctx context.Context) {
		telemetry.StartSpan(ctx, "binding work").End()
	})
	program, err := rt.Load(`<?php include "one.php"; measure();`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}

	if len(recorder.traces) != 0 {
		t.Fatalf("traces = %q, want none of the interpreter's own", recorder.traces)
	}
	if findSpan(recorder.trace, "binding work") == nil {
		t.Error("the binding's span went with the interpreter's; the level is not the recorder's")
	}

	// The count is the request span's attribute and not an interpreter span;
	// the level does not silence it.
	if want := []int{1, 2}; !reflect.DeepEqual(recorder.included, want) {
		t.Fatalf("included = %v, want %v", recorder.included, want)
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

// TestRuntimeObserverStaticCallOpensNoSpan pins that a static call is not
// measured: a library calls static helpers by the hundred and each one used to
// add a span that buried the requests and includes the trace is read for.
func TestRuntimeObserverStaticCallOpensNoSpan(t *testing.T) {
	t.Parallel()
	recorder := newTelemetryRecorder(t)
	rt := New(nil, Options{RootFS: fstest.MapFS{
		"main.php": {Data: []byte("<?php\ninclude \"Util.php\";\n$u = new Util;\necho Util::twice(2), $u->thrice(2);\n")},
		"Util.php": {Data: []byte(`<?php
class Util {
    static function twice($n) { return $n * 2; }
    function thrice($n) { return $n * 3; }
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
	var instance bool
	for _, span := range recorder.spans {
		if span.Name == "Util::twice" {
			t.Errorf("static call opened span %q", span.Name)
		}
		if span.Name == "u.thrice" {
			instance = true
		}
	}
	if !instance {
		t.Errorf("the instance method span is gone: %+v", recorder.spans)
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
	// does nothing and a host traces unconditionally.
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

	// The same state twice is one phase and a host calling it per statement
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
