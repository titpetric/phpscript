# Custom entrypoints

The server reaches PHP through a request: a URL resolves to a file, the request fills the superglobals and the file runs top to bottom. That is the right shape for a site. A Go program calling one function needs another.

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

The PHP side is ordinary. The file declares a function:

```php
<?php
namespace App\Handler;

function index(\HTTP\ResponseWriter $w, \HTTP\Request $r) {
	$w->header()->set("Content-Type", "text/plain");
	$w->write("hello from " . $r->url->path);
}
```

An `http.ResponseWriter` and an `*http.Request` are Go values and they arrive as themselves. The script reads and writes them through the same reflection bridge every host object uses. There is no marshalling step. The bridge copies neither value.

`HTTP\Request` names `*net/http.Request`, the name a script already knows, which `new HTTP\Request` builds for an outbound call. `HTTP\ResponseWriter` follows the same rule for `http.ResponseWriter`. `HTTP\ResponseWriter` names no constructor: a handler receives a response writer and constructs none.

The runtime checks neither hint, here or anywhere: parameter types are a [known divergence](../README.md#known-divergences-from-php), not something a lookup opts out of. Write them for the reader.

## What an invocation receives

The arguments alone.

The runtime decodes no request. `$_GET`, `$_POST` and `$_SERVER` remain undefined. No file body runs: the runtime hoists the declaring program for the functions and classes it declares, and skips its top-level statements. The scope the function body executes in holds its parameters and whatever it declares itself.

`BenchmarkLookup` splits the cost up, on one pinned core:

```
BenchmarkLookup/bind         	 3142390	       381.0 ns/op	     128 B/op	       3 allocs/op
BenchmarkLookup/invoke       	  299434	      3986 ns/op	     320 B/op	      18 allocs/op
BenchmarkLookup/callable     	  388674	      3171 ns/op	     240 B/op	      14 allocs/op
BenchmarkLookup/runtime      	   10000	    132753 ns/op	   84364 B/op	     684 allocs/op
BenchmarkLookup/request      	    7159	    158615 ns/op	   96324 B/op	     850 allocs/op
```

`bind` resolves the symbol and builds the closure, paid per runtime. `invoke` calls it. That line alone scales with traffic. `callable` makes the same call through the untyped `rt.Callable`. The gap between the two shows the signature bridge's cost. `runtime` runs no PHP at all: the benchmark builds a runtime and registers the standard library on it.

In that last pair, `request` less `runtime` is the script. The rest is host setup for that call. A lookup on a reused runtime skips the setup. That setup cost is also why the request path stops scaling with cores while the lookup path keeps going - at ninety kilobytes a go, the collector becomes the limit before the CPU does.

A host needing the superglobals still has them: build the runtime, `runner.FromRequest(r).Register(rt)`, and look up afterwards. The lookup adds nothing and removes nothing.

## Finding the symbol

The symbol may be anywhere inside the runtime's source root. A runtime that has loaded nothing parses its tree on the first lookup through `Precompiler`. No load step needs remembering. A host that already precompiled, or that has been serving requests, resolves against the cache it filled.

A free function in a namespaced file keeps the namespace in its name and `App\Handler\index` is the whole name. That name resolves. A trailing part of it also resolves:

```go
runner.Lookup[http.HandlerFunc](rt, "App\\Handler\\index") // exact
runner.Lookup[http.HandlerFunc](rt, "Handler\\index")      // by segment
runner.Lookup[http.HandlerFunc](rt, "index")               // by name
```

The last one is an error if two namespaces declare an `index`. `*runner.LookupError` lists the names that matched. A host binding its handlers at startup reads which spelling to use, and serves no handler on a guess.

## Signatures

`runner.Lookup` checks `T` when it binds the symbol, not when the host calls the result. `T` must be a function type returning at most one value and an optional trailing `error`:

```go
runner.Lookup[func(*http.Request) bool](rt, "middleware")
runner.Lookup[func(string) (int, error)](rt, "count_words")
runner.Lookup[func(*testing.T)](rt, "test_users")
runner.Lookup[func(...string) string](rt, "join")
```

A returned value takes PHP's own coercions into the declared result: `1` fills a `bool` the way `if (1)` reads it, `"42"` fills an `int` and an array fills a `[]string` or a `map[string]int`. An error must go somewhere. A signature ending in `error` receives the error. A signature that does not passes it to the runtime's error sink, the same place a failed script's error goes.

## Concurrency

A `*runner.Runtime` serves one goroutine. A looked-up function stays with its runtime. A host calling one from more than one goroutine builds a runtime per goroutine and shares the two caches across goroutines:

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

That is the arrangement the HTTP server already keeps per request: the process pays for the parse and the bytecode compile once. Everything an invocation allocates belongs to it. Two goroutines share only a read lock.

Sharing one runtime instead is a crash. Evaluation writes `rt.compiled`, `rt.goMethods` and `rt.frames` without a lock. Two goroutines calling one closure take the process down with `fatal error: concurrent map writes` out of `setCompiledExpr`.

## Reusing a runtime

A runtime cannot serve two requests at once. It can serve them in turn, which is a pool. The table below splits the cost:

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

Pooling is 37x a fresh runtime per request and 59x less garbage, which is the part that decides whether more cores help. The cost is PHP's request isolation: script globals, `static $x` in a function or a class, constants the script defined and anything `register_shutdown_function` collected all survive into the next request on that runtime. A handler that is a pure function of its arguments does not care. One that is not will read the last request's state.

`pooled_reset` is the same pool with `rt.ResetSession(io.Discard, nil)` first, which drops it all. That also drops the declarations and the lookup binds the symbol again per request - at a few hundred nanoseconds and the line is still 15x a fresh runtime. If you are unsure which handler you have, use this one.

A pool holds no writer: `runner.New` fixes it for the runtime's life. That does not arise above because the handler writes through `$w`, but a function that `echo`es needs `rt.PushOutput(w)` plus a deferred `rt.PopOutput()` around the call, which is the same stack output buffering uses.

## Limits

A looked-up symbol runs on the interpreter even in a runtime built with `NewFlatStack`, since a flat-declared function belongs to the bytecode program's table.

`Options.Include` is not run per invocation. A lookup installs no composer autoloader. A function that needs one requires it in the function body, or the host runs the prelude on the runtime before looking anything up.

A `static $x` inside a looked-up function keeps its value across invocations on the same runtime, which is PHP's within-request semantics applied to the runtime's lifetime.
