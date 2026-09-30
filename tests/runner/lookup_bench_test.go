package runner_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
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

// BenchmarkLookup decomposes what reaching PHP from Go costs, every part of it
// parallel over one shared cache pair with a runtime per goroutine, which is
// the contract a host calling a looked-up symbol concurrently keeps.
//
// The parts are separate because they are paid at different times, and reading
// one for another is the mistake the split exists to prevent:
//
//   - bind resolves the symbol and builds the closure. Paid per runtime.
//   - invoke calls the closure. Paid per call, and the only figure that scales
//     with traffic.
//   - callable is the same call through rt.Callable, the untyped API that
//     existed before this one, so the difference is what the signature bridge
//     costs rather than what the call does.
//   - runtime builds a runtime and registers the standard library onto it, and
//     runs no PHP at all.
//   - request is the shape a host reaches PHP through today: that runtime, the
//     request decoded into superglobals over it, the entrypoint read back out
//     of the include cache and the file run top to bottom.
//
// request less runtime is what the script cost; the rest is the host getting
// ready to run it, which is the part a lookup on a reused runtime skips.
//
// Run with -cpu 1,2,4 for the parallel scaling. Nothing an invocation
// allocates is shared, so the only contention is the two caches' read locks,
// while request allocates ninety kilobytes a go and stops scaling where the
// collector becomes the limit.
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

	b.Run("bind", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			rt := newRuntime(io.Discard)
			// Resolved once outside the loop, so the figure is what binding the
			// second handler of a tree costs rather than what installing the
			// first one does.
			if _, err := runner.Lookup[func(*http.Request) string](rt, "App\\Handler\\main"); err != nil {
				b.Error(err)
				return
			}
			for pb.Next() {
				// The error is checked, which is what keeps the call from
				// being optimised away; the closure itself is the product and
				// nothing here needs it.
				if _, err := runner.Lookup[func(*http.Request) string](rt, "App\\Handler\\main"); err != nil {
					b.Error(err)
					return
				}
			}
		})
	})

	b.Run("invoke", func(b *testing.B) {
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

	b.Run("runtime", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				// No PHP at all: the runtime a request builds and the standard
				// library registered onto it, which is the share of the figure
				// below that has nothing to do with the script.
				lookupBenchmarkSink = len(newRuntime(io.Discard).Env)
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
	verify("invoke", handle(request))

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

// BenchmarkLookupTreeSize holds the symbol index to constant time. Resolving a
// name is a walk of every statement of every program in the tree, and a host
// serving concurrently binds per runtime and therefore per goroutine, which is
// often per request: paying the size of the application there is the cost this
// guards against coming back.
func BenchmarkLookupTreeSize(b *testing.B) {
	for _, files := range []int{2, 50, 200, 800} {
		b.Run(fmt.Sprintf("files=%d", files), func(b *testing.B) {
			tree := fstest.MapFS{}
			for i := range files {
				tree[fmt.Sprintf("pkg%03d/handlers.php", i)] = &fstest.MapFile{Data: fmt.Appendf(nil, `<?php
namespace App\Pkg%03d;

function handle($r) { return "pkg%03d"; }
function helper_a() { return 1; }
function helper_b() { return 2; }
`, i, i)}
			}

			includes := runner.NewIncludeCache()
			runner.Precompiler{Root: tree, Includes: includes}.Run()
			rt := runner.New(io.Discard, runner.Options{RootFS: tree, Precompile: true})
			rt.SetIncludeCache(includes)

			// The last package, so a resolution that scanned would scan the
			// whole tree before reaching it.
			name := fmt.Sprintf(`App\Pkg%03d\handle`, files-1)
			if _, err := runner.Lookup[func(*http.Request) string](rt, name); err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()
			var bound func(*http.Request) string
			for b.Loop() {
				handle, err := runner.Lookup[func(*http.Request) string](rt, name)
				if err != nil {
					b.Fatal(err)
				}
				bound = handle
			}
			if bound == nil {
				b.Fatal("nothing was bound")
			}
		})
	}
}

// lookupHandlerTree is one PHP handler answering through the net/http values it
// was handed, so the shapes below differ by what the host does around it.
var lookupHandlerTree = fstest.MapFS{"h.php": {Data: []byte(`<?php
namespace App;
function index($w, $r) {
	$w->header()->set("Content-Type", "text/plain");
	$w->write("hello from " . $r->url->path);
}
`)}}

func BenchmarkLookupHandler(b *testing.B) {
	includes := runner.NewIncludeCache()
	exprs := runner.NewExprCache()
	runner.Precompiler{Root: lookupHandlerTree, Includes: includes}.Run()

	newRuntime := func() *runner.Runtime {
		rt := runner.New(io.Discard, runner.Options{RootFS: lookupHandlerTree, Precompile: true})
		stdlib.Register(rt)
		rt.SetIncludeCache(includes)
		rt.SetExprCache(exprs)
		return rt
	}

	goHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello from " + r.URL.Path))
	})

	fresh := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h, err := runner.Lookup[http.HandlerFunc](newRuntime(), `App\index`)
		if err != nil {
			b.Error(err)
			return
		}
		h(w, r)
	})

	pool := sync.Pool{New: func() any {
		rt := newRuntime()
		h, err := runner.Lookup[http.HandlerFunc](rt, `App\index`)
		if err != nil {
			panic(err)
		}
		return h
	}}
	pooled := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := pool.Get().(http.HandlerFunc)
		defer pool.Put(h)
		h(w, r)
	})

	// Same pool, but each request starts from a clean session: script globals,
	// class and function statics, constants and declarations are dropped. The
	// declaration goes with them, so the symbol is bound again per request.
	rtPool := sync.Pool{New: func() any { return newRuntime() }}
	reset := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rt := rtPool.Get().(*runner.Runtime)
		defer rtPool.Put(rt)
		rt.ResetSession(io.Discard, nil)
		h, err := runner.Lookup[http.HandlerFunc](rt, `App\index`)
		if err != nil {
			b.Error(err)
			return
		}
		h(w, r)
	})

	request := httptest.NewRequest(http.MethodGet, "/greet", nil)
	for _, bm := range []struct {
		name string
		h    http.Handler
	}{{"go", goHandler}, {"pooled", pooled}, {"pooled_reset", reset}, {"fresh", fresh}} {
		b.Run(bm.name, func(b *testing.B) {
			check := httptest.NewRecorder()
			bm.h.ServeHTTP(check, request)
			if got := check.Body.String(); got != "hello from /greet" {
				b.Fatalf("body = %q", got)
			}
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					bm.h.ServeHTTP(httptest.NewRecorder(), request)
				}
			})
		})
	}
}
