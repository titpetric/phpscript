# Custom entrypoints

The server reaches PHP through a request: a URL resolves to a file, the request is decoded into superglobals, and the file runs top to bottom. That is the right shape for a site. It is the wrong shape for a Go program that wants one function.

`runner.Lookup` is the other shape. The host names a symbol and the signature it wants, and gets a Go function back.

```go
rt := runner.New(os.Stdout, runner.Options{RootFS: os.DirFS("app")})
stdlib.Register(rt)

handle, err := runner.Lookup[http.HandlerFunc](rt, "App\\Handler\\index")
if err != nil {
	log.Fatal(err)
}
mux.Handle("GET /", handle)
```

Nothing about the PHP side is special. The file declares a function:

```php
<?php
namespace App\Handler;

function index(\HTTP\ResponseWriter $w, \HTTP\Request $r) {
	$w->header()->set("Content-Type", "text/plain");
	$w->write("hello from " . $r->url->path);
}
```

An `http.ResponseWriter` and an `*http.Request` are Go values, so they arrive as themselves. The script reads and writes them through the same reflection bridge every host object uses; there is no marshalling step and no copy.

`HTTP\Request` is the name a script already has for `*net/http.Request`, the one `new HTTP\Request` builds for an outbound call, and `HTTP\ResponseWriter` is `http.ResponseWriter` under the same rule. `HTTP\ResponseWriter` names no constructor, because a response writer is handed to a handler rather than made by one.

Neither hint is checked, here or anywhere: parameter types are a [known divergence](../README.md#known-divergences-from-php), not something a lookup opts out of. Write them for the reader.

## What an invocation carries

The arguments, and nothing else.

No request is decoded, so `$_GET`, `$_POST` and `$_SERVER` are absent rather than empty. No file body runs: the declaring program is hoisted for the functions and classes it declares, and its top-level statements are skipped. The scope the function body executes in holds its parameters and whatever it declares itself.

That is what makes the path cheap. `BenchmarkLookup` splits the cost up, on one pinned core:

```
BenchmarkLookup/bind         	 3142390	       381.0 ns/op	     128 B/op	       3 allocs/op
BenchmarkLookup/invoke       	  299434	      3986 ns/op	     320 B/op	      18 allocs/op
BenchmarkLookup/callable     	  388674	      3171 ns/op	     240 B/op	      14 allocs/op
BenchmarkLookup/runtime      	   10000	    132753 ns/op	   84364 B/op	     684 allocs/op
BenchmarkLookup/request      	    7159	    158615 ns/op	   96324 B/op	     850 allocs/op
```

`bind` is resolving the symbol and building the closure, paid per runtime. `invoke` is calling it, the only line that scales with traffic, and `callable` is the same call through the untyped `rt.Callable`, so the gap between them is what the signature bridge costs. `runtime` runs no PHP at all: it is a runtime built and the standard library registered onto it.

That last pair is the point. `request` less `runtime` is about 26us of script; the other 133us is the host getting ready to run it, four fifths of the figure, and it is what a lookup on a reused runtime skips. It is also why the request path stops scaling with cores while the lookup path keeps going - at ninety kilobytes a go, the collector becomes the limit before the CPU does.

A host that wants the superglobals still has them: build the runtime, `runner.FromRequest(r).Register(rt)`, and look up afterwards. The lookup does not take them away, it just does not add them.

## Finding the symbol

The symbol may be anywhere under the runtime's source root. A runtime that has loaded nothing parses its tree through a `Precompiler` on the first lookup, so there is no load step to remember; a host that already precompiled, or that has been serving requests, resolves against the cache it filled.

A free function in a namespaced file carries the namespace, so `App\Handler\index` is the whole name. That name resolves, and so does any trailing part of it:

```go
runner.Lookup[http.HandlerFunc](rt, "App\\Handler\\index") // exact
runner.Lookup[http.HandlerFunc](rt, "Handler\\index")      // by segment
runner.Lookup[http.HandlerFunc](rt, "index")               // by name
```

The last one is an error if two namespaces declare an `index`. `*runner.LookupError` carries the names that matched, so a host binding its handlers at startup is told which spelling to use rather than served whichever one the map happened to yield.

## Signatures

`T` is checked when it is bound, not when it is called. It must be a function type returning at most one value and an optional trailing `error`:

```go
runner.Lookup[func(*http.Request) bool](rt, "middleware")
runner.Lookup[func(string) (int, error)](rt, "count_words")
runner.Lookup[func(*testing.T)](rt, "test_users")
runner.Lookup[func(...string) string](rt, "join")
```

A returned value takes PHP's own coercions into the declared result: `1` fills a `bool` the way `if (1)` reads it, `"42"` fills an `int`, and an array fills a `[]string` or a `map[string]int`. An error has to go somewhere - a signature ending in `error` receives it, and one that does not hands it to the runtime's error sink, which is where a failed script's error goes.

## Concurrency

A `*runner.Runtime` serves one goroutine, and a looked-up function belongs to the runtime it was found on. A host calling one from several goroutines builds a runtime per goroutine and shares the two caches between them:

```go
includes, exprs := runner.NewIncludeCache(), runner.NewExprCache()
runner.Precompiler{Root: root, Includes: includes}.Run()

// per goroutine
rt := runner.New(io.Discard, runner.Options{RootFS: root, Precompile: true})
stdlib.Register(rt)
rt.SetIncludeCache(includes)
rt.SetExprCache(exprs)
handle, err := runner.Lookup[http.HandlerFunc](rt, "App\\Handler\\index")
```

That is the arrangement the HTTP server already keeps per request: the parse and the bytecode compile are paid once for the process, everything an invocation allocates is its own, and the only thing two goroutines share is a read lock.

Sharing one runtime instead is not a race to think carefully about, it is a crash. `rt.compiled`, `rt.goMethods` and `rt.frames` are written during evaluation with no lock, so two goroutines calling one closure take the process down with `fatal error: concurrent map writes` out of `setCompiledExpr`.

## Reusing a runtime

A runtime cannot serve two requests at once. It can serve them one after another, which is what a pool gives you, and it is where the 133us goes:

```go
pool := sync.Pool{New: func() any {
	rt := newRuntime()
	handle, err := runner.Lookup[http.HandlerFunc](rt, "App\\Handler\\index")
	if err != nil {
		panic(err)
	}
	return handle
}}

mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	handle := pool.Get().(http.HandlerFunc)
	defer pool.Put(handle)
	handle(w, r)
}))
```

`BenchmarkLookupHandler` runs the same PHP handler four ways, `go` being a native Go handler doing the same work as the floor:

```
                             │ sec/op   │ B/op     │ allocs/op │
LookupHandler/go-4              422.5n     1.008Ki      10
LookupHandler/pooled-4          2.441µ     1.619Ki      35
LookupHandler/pooled_reset-4    6.269µ     1.954Ki      44
LookupHandler/fresh-4           91.54µ    94.950Ki     872
```

A kilobyte of every row is the `httptest` recorder, which the native handler allocates too.

Pooling is 37x a fresh runtime per request, and it is 59x less garbage, which is the part that decides whether more cores help. What it costs is PHP's request isolation: script globals, `static $x` in a function or a class, constants the script defined and anything `register_shutdown_function` collected all survive into the next request on that runtime. A handler that is a pure function of its arguments does not care. One that is not will read the last request's state.

`pooled_reset` is the same pool with `rt.ResetSession(io.Discard, nil)` first, which drops all of it. That also drops the declarations, so the symbol is bound again per request - at a few hundred nanoseconds, which is why the line is still 15x a fresh runtime. If you are unsure which handler you have, use this one.

One thing a pool does not carry: the writer is fixed when the runtime is built. That does not arise above because the handler writes through `$w`, but a function that `echo`es needs `rt.PushOutput(w)` and a deferred `rt.PopOutput()` around the call, which is the same stack output buffering is built on.

## Limits

A looked-up symbol runs on the interpreter even when the runtime was built with `NewFlatStack`, because a flat-declared function lives in the bytecode program's own table rather than the runtime's.

`Options.Include` is not run per invocation, so a composer autoloader is not installed by a lookup. A function that needs one requires it in its own body, or the host runs the prelude on the runtime before looking anything up.

A `static $x` inside a looked-up function keeps its value across invocations on the same runtime, which is PHP's within-request semantics applied to the runtime's lifetime.
