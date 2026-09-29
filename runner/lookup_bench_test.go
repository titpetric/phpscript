package runner_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// benchLookupTree holds the two spellings of one handler: the function a lookup
// calls, and the whole-file entrypoint a request runs. Both answer the same
// string off the same request, so the two figures differ by what the host does
// around the PHP rather than by what the PHP does.
var benchLookupTree = fstest.MapFS{
	"handler.php": {Data: []byte(`<?php
namespace App\Handler;

function main(\HTTP\Request $r) {
	return $r->method . " " . $r->url->path;
}
`)},
	"entrypoint.php": {Data: []byte(`<?php
echo $_SERVER["REQUEST_METHOD"], " ", $_SERVER["REQUEST_URI"];
`)},
}

var lookupBenchmarkSink int

// BenchmarkLookup measures one invocation into the VM three ways, all of them
// parallel over one shared cache pair with a runtime per goroutine, which is
// the contract a host calling a looked-up symbol concurrently keeps.
//
// lookup is the typed closure. callable is rt.Callable, the untyped API that
// existed before it, so the figure says what the signature bridge costs on top
// of the call it wraps. request is the shape a host reaches PHP through today:
// a fresh runtime with the standard library registered on it, the entrypoint
// read back out of the include cache, and the file run top to bottom with the
// request decoded over it.
//
// Run with -cpu 1,2 to see the parallel scaling: nothing an invocation
// allocates is shared, so the only contention is the two caches' read locks.
func BenchmarkLookup(b *testing.B) {
	includes := runner.NewIncludeCache()
	exprs := runner.NewExprCache()
	if cached := (runner.Precompiler{Root: benchLookupTree, Includes: includes}).Run(); cached != len(benchLookupTree) {
		b.Fatalf("precompiled %d files, want %d", cached, len(benchLookupTree))
	}

	newRuntime := func(w io.Writer) *runner.Runtime {
		rt := runner.New(w, runner.Options{RootFS: benchLookupTree, Precompile: true})
		stdlib.Register(rt)
		rt.SetIncludeCache(includes)
		rt.SetExprCache(exprs)
		return rt
	}

	request := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	const want = "GET /users/42"

	b.Run("lookup", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			handle, err := runner.Lookup[func(*http.Request) string](newRuntime(io.Discard), "App\\Handler\\main")
			if err != nil {
				b.Error(err)
				return
			}
			for pb.Next() {
				lookupBenchmarkSink = len(handle(request))
			}
		})
	})

	b.Run("callable", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			rt := newRuntime(io.Discard)
			// Callable resolves against the function table, so the declaration
			// has to be there; a lookup installs it, and this is the same
			// install without the typed wrapper on top.
			if _, err := runner.Lookup[func(*http.Request) string](rt, "App\\Handler\\main"); err != nil {
				b.Error(err)
				return
			}
			call, ok := rt.Callable("App\\Handler\\main")
			if !ok {
				b.Error("App\\Handler\\main is not callable")
				return
			}
			for pb.Next() {
				value, err := call(request)
				if err != nil {
					b.Error(err)
					return
				}
				lookupBenchmarkSink = len(value.(string))
			}
		})
	})

	b.Run("request", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				var out responseBuffer
				rt := newRuntime(&out)
				rt.SetContext(request.Context())
				runner.FromRequest(request).Register(rt)

				program, err := rt.LoadFile("entrypoint.php")
				if err == nil {
					err = rt.Run(program)
				}
				if err != nil {
					b.Error(err)
					return
				}
				lookupBenchmarkSink = out.n
			}
		})
	})

	// The control is checked once rather than asserted per iteration, so the
	// comparison is not paying for the comparison.
	verify := func(name, got string) {
		if got != want {
			b.Fatalf("%s produced %q, want %q", name, got, want)
		}
	}
	handle, err := runner.Lookup[func(*http.Request) string](newRuntime(io.Discard), "App\\Handler\\main")
	if err != nil {
		b.Fatal(err)
	}
	verify("lookup", handle(request))

	var out responseBuffer
	entrypoint := newRuntime(&out)
	runner.FromRequest(request).Register(entrypoint)
	program, err := entrypoint.LoadFile("entrypoint.php")
	if err == nil {
		err = entrypoint.Run(program)
	}
	if err != nil {
		b.Fatal(err)
	}
	verify("request", out.text)
}

// responseBuffer counts what an entrypoint echoed without the comparison
// allocating a string per iteration; the text is kept for the one check.
type responseBuffer struct {
	n    int
	text string
}

func (r *responseBuffer) Write(p []byte) (int, error) {
	r.n += len(p)
	r.text += string(p)
	return len(p), nil
}
