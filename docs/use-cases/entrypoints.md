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

`HTTP\Request` is the name a script already has for `*net/http.Request`, the one `new HTTP\Request` builds for an outbound call, and `HTTP\ResponseWriter` is `http.ResponseWriter` under the same rule. Neither hint is enforced - phpscript parses a parameter type and never checks it - and `HTTP\ResponseWriter` names no constructor, because a response writer is handed to a handler rather than made by one. Writing the hints still says what the function expects, which is the whole job a type hint has here.

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

## Limits

A looked-up symbol runs on the interpreter even when the runtime was built with `NewFlatStack`, because a flat-declared function lives in the bytecode program's own table rather than the runtime's.

`Options.Include` is not run per invocation, so a composer autoloader is not installed by a lookup. A function that needs one requires it in its own body, or the host runs the prelude on the runtime before looking anything up.

A `static $x` inside a looked-up function keeps its value across invocations on the same runtime, which is PHP's within-request semantics applied to the runtime's lifetime.
