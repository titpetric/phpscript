package runner

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

type statusRecorder struct {
	statuses  []Status
	filenames []string
	included  []int
}

func (r *statusRecorder) UpdateStatus(_ context.Context, status Status) {
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
	want := []Status{StatusStarting, StatusReading, StatusProcessing, StatusWriting}
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
	if got := recorder.statuses[len(recorder.statuses)-1]; got != StatusError {
		t.Fatalf("last status = %q, want E", got)
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
		"main.php": {Data: []byte(`<?php include "one.php"; include "two.php";`)},
		"one.php":  {Data: []byte(`<?php $one = 1;`)},
		"two.php":  {Data: []byte(`<?php $two = 2;`)},
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
	if want := []int{1, 2}; !reflect.DeepEqual(recorder.included, want) {
		t.Fatalf("included = %v, want %v", recorder.included, want)
	}
}
