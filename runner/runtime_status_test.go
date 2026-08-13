package runner

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/model"
)

type statusRecorder struct {
	statuses  []model.Status
	filenames []string
	traces    []string
	spans     []*model.RequestSpan
	included  []int
	request   model.Request
}

func (r *statusRecorder) Trace(ctx context.Context, message string, flags ...model.Flag) *model.RequestSpan {
	r.traces = append(r.traces, message)
	span := model.StartSpan(model.WithRequest(ctx, &r.request), message, flags...)
	r.spans = append(r.spans, span)
	return span
}

func (r *statusRecorder) UpdateStatus(_ context.Context, status model.Status) {
	r.statuses = append(r.statuses, status)
}

func (r *statusRecorder) UpdateFilename(_ context.Context, filename string) {
	r.filenames = append(r.filenames, filename)
}

func (r *statusRecorder) UpdateIncludedFiles(_ context.Context, count int) {
	r.included = append(r.included, count)
}

func TestRuntimeObserverReceivesExecutionPhases(t *testing.T) {
	var out strings.Builder
	recorder := &statusRecorder{}
	rt := New(&out, Options{})
	rt.Observe(recorder)
	program, err := rt.Load(`<?php $a = 1; echo $a; ?>`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}
	want := []model.Status{model.StatusStarting, model.StatusReading, model.StatusProcessing, model.StatusWriting}
	if !reflect.DeepEqual(recorder.statuses, want) {
		t.Fatalf("statuses = %q, want %q", recorder.statuses, want)
	}
}

func TestRuntimeObserverReceivesParseError(t *testing.T) {
	recorder := &statusRecorder{}
	rt := New(nil, Options{})
	rt.Observe(recorder)
	if _, err := rt.Load(`<?php echo ;`); err == nil {
		t.Fatal("expected parse error")
	}
	if got := recorder.statuses[len(recorder.statuses)-1]; got != model.StatusError {
		t.Fatalf("last status = %q, want E", got)
	}
}

func TestRuntimeObserverReceivesExecutionErrorSpan(t *testing.T) {
	recorder := &statusRecorder{}
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
	if got := recorder.statuses[len(recorder.statuses)-1]; got != model.StatusError {
		t.Fatalf("last status = %q, want E", got)
	}
	if len(recorder.spans) != 1 || recorder.spans[0].Message != "Error: <code>assign: target is not an array</code>" || recorder.spans[0].Error != "assign: target is not an array" || recorder.spans[0].Filename != "main.php" || recorder.spans[0].Line != 1 {
		t.Fatalf("error spans = %+v", recorder.spans)
	}
}

func TestRuntimeContextCarriesActiveSourceLine(t *testing.T) {
	request := &model.Request{}
	rt := New(nil, Options{RootFS: fstest.MapFS{
		"main.php": {Data: []byte("<?php\n\ninspect_context();")},
	}})
	rt.SetContext(model.WithRequest(context.Background(), request))
	rt.RegisterFunc("inspect_context", func(ctx context.Context) {
		model.StartSpan(ctx, "inspect")
	})
	program, err := rt.LoadFile("main.php")
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}
	if len(request.Spans) != 1 || request.Spans[0].Filename != "main.php" || request.Spans[0].Line != 3 {
		t.Fatalf("spans = %+v", request.Spans)
	}
}

func TestRuntimeObserverReceivesEntrypointFilename(t *testing.T) {
	recorder := &statusRecorder{}
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
	recorder := &statusRecorder{}
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
	if want := []string{"include one.php", "include nested.tpl", "include nested.tpl", "include one.php", "include two.php", "include two.php"}; !reflect.DeepEqual(recorder.traces, want) {
		t.Fatalf("traces = %q, want %q", recorder.traces, want)
	}
	if len(recorder.spans) != 6 {
		t.Fatalf("include spans = %+v", recorder.spans)
	}
	for _, i := range []int{0, 1, 4} {
		if span := recorder.spans[i]; !span.Open || span.Duration <= 0 {
			t.Fatalf("opening include span = %+v", span)
		}
	}
	for _, i := range []int{2, 3, 5} {
		if span := recorder.spans[i]; !span.Close || span.Duration != 0 {
			t.Fatalf("closing include span = %+v", span)
		}
	}
	if recorder.spans[1].Type != model.SpanType.Template || recorder.spans[2].Type != model.SpanType.Template {
		t.Fatalf("template include spans = %+v / %+v", recorder.spans[1], recorder.spans[2])
	}
	for _, i := range []int{0, 3, 4, 5} {
		if recorder.spans[i].Type != model.SpanType.Internal {
			t.Fatalf("PHP include span = %+v", recorder.spans[i])
		}
	}
	wantFiles := []string{"main.php", "one.php", "one.php", "main.php", "main.php", "main.php"}
	for i, filename := range wantFiles {
		if recorder.spans[i].Filename != filename {
			t.Fatalf("span %d filename = %q, want %q", i, recorder.spans[i].Filename, filename)
		}
	}
}

func TestRuntimeObserverReceivesMeasuredConstructor(t *testing.T) {
	recorder := &statusRecorder{}
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
	if len(recorder.spans) != 1 || recorder.spans[0].Duration <= 0 || recorder.spans[0].Filename != "main.php" {
		t.Fatalf("constructor spans = %+v", recorder.spans)
	}
}

func TestRuntimeObserverReceivesMeasuredPHPMethod(t *testing.T) {
	recorder := &statusRecorder{}
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
	var methodSpan *model.RequestSpan
	for _, span := range recorder.spans {
		if span.Message == "db.get" {
			methodSpan = span
			break
		}
	}
	if methodSpan == nil || methodSpan.Duration <= 0 || methodSpan.Filename != "main.php" || methodSpan.Line != 4 {
		t.Fatalf("method spans = %+v", recorder.spans)
	}
}
