# HTTP server bindings

The standard runtime provides the Go-backed `HTTP\Mux` and `HTTP\Server` classes. They are `net/http`'s `ServeMux` and `Server`, so a PHP program can be the server rather than something a server runs: it builds the router, listens, and owns the process until it stops.

That is one of two ways to serve. The other is `phpscript server`, which scans a source tree for `// @route` comments and runs a file per request; see [routing.md](routing.md). Use that one for an application laid out as pages and endpoints. Use this one when the program is the server: a service with a handful of routes, a test server, something that has to start and stop on its own terms.

`testdata/testserver.php` is a working example of everything below, and `testdata/testserver.go` is the same routes in Go beside it.

## A script that serves

```php
<?php

$mux = new HTTP\Mux();

$mux->handle("GET /hello", function (\HTTP\ResponseWriter $w, \HTTP\Request $r) {
    $w->header()->set("Content-Type", "text/plain; charset=utf-8");
    $w->write("hello " . $r->url->query()->get("name") . "\n");
});

$http = new HTTP\Server("127.0.0.1:8099", $mux, 4, 64);
$http->listen();

register_shutdown_function(function () use ($http) {
    $http->shutdown();
});

set_time_limit(60);
echo "serving on http://" . $http->addr() . "\n";
$http->wait();
```

`listen()` binds and returns the address, and does not block. `wait()` blocks until the script runs out of time. `shutdown()` stops accepting and lets what is in flight finish; `close()` drops it. An `$addr` of `127.0.0.1:0` binds a port the system picks, which `listen()` and `addr()` answer with, and is how a test takes a free one rather than hoping.

## What a handler is

A handler is a callable, in three spellings:

| Spelling                        | Written as                                          |
|---------------------------------|-----------------------------------------------------|
| A closure                       | `$mux->handle("GET /x", function ($w, $r) { ... })` |
| A method bound to its receiver  | `$mux->handle("GET /x", $this->index)`              |
| The name of a declared function | `$mux->handle("GET /x", "handle_index")`            |

`$this->fnName` reads the method without calling it, which is php's first-class callable syntax `$this->fnName(...)` in a shorter spelling. It is what a server written as a class registers:

```php
class Server {
    function mount() {
        $mux = new HTTP\Mux();
        $mux->handle('GET /{$}', $this->index);
        $mux->handle("GET /users/{id}", $this->showUser);
        return $mux;
    }

    function index($w, $r) { echo "the index\n"; }
    function showUser($w, $r) { echo "user " . $r->path_value("id") . "\n"; }
}
```

`array($object, "method")` is not accepted here. It stays a callable everywhere else - `call_user_func`, `usort` and the rest take it - and it is not going to be added: a handler is a function of its arguments, and wrapping one in an array to name a method is a spelling this does not want.

## Patterns

The pattern is `net/http`'s, so a method and a path, with `{name}` segments the handler reads back through `$r->path_value($name)`:

```php
$mux->handle('GET /{$}', $index);          // the root, and nothing else
$mux->handle("GET /users/{id}", $show);     // a path value
$mux->handle("POST /echo", $echo);          // one method
$mux->handle("/health", $health);           // any method
```

`GET /` is a `net/http` subtree and answers for every path nothing else claimed; `GET /{$}` anchors it to the root alone. Write `'{$}'` in single quotes: in a double-quoted string `{$` starts an interpolation.

## Answering

The handler is called with the response writer and the request, the two `net/http` values themselves:

```php
$w->header()->set("Content-Type", "application/json");
$w->write($body);
(new JSON\Encoder($w))->encode(array("id" => $r->path_value("id")));
echo "this reaches the response too\n";
```

`echo` reaches the response: the runtime answering the request writes there for the length of the call. `$r->url->query()->get($name)` is the first value under a name, `$r->form_value($name)` reads the query and the body together, and `$r->method`, `$r->url->path`, `$r->url->rawquery` and `$r->user_agent()` are the request as Go names it.

A handler that throws is one request's problem: it is reported to the runtime's error sink and answered with a 500 if nothing has gone out yet, and the server goes on.

## Workers and the queue

`HTTP\Server`'s third argument is how many requests are answered at once and its fourth is how deep the queue behind them is. Omitted, they are the number of cores and 1024.

```php
$http = new HTTP\Server($addr, $mux, 4, 64);
```

Each worker holds a runtime of its own, forked from the one running the script. Workers are the parallelism, the queue is the backpressure: a request that finds every worker busy waits its turn rather than starting a runtime of its own, and a queue that fills means the caller waits, which is the signal that it should. A handler doing no IO can use about as many workers as there are cores; one that waits on a database wants more.

## What a handler can reach

A handler starts on a clean stack holding `$w` and `$r` and nothing else. No superglobals are decoded, nothing carries over from the last request, and two requests answered at the same moment cannot see each other.

What a handler carries with it is what it captured: a closure's `use (...)` values and the `$this` it binds, or the receiver a bound method was read off. Those are shared by every request answering through that handler, because each request runs on a runtime of its own and the capture is not copied. Read them. Writing to one from a handler is two requests writing to one value, and the engine cannot make that safe without copying the object, which is not what a shared receiver is for.

Autoloaders come along too, so a class no request had named yet is loaded on the worker that first names it, once per worker. What does not come along is anything the script wrote: globals, `static $x`, constants the script defined, the list of included files, the output it produced. A handler reaches what it captured, what its receiver holds, and what was declared before `listen()`.

## Re-entry: calling back into PHP

A handler often calls something the script built before the server started. Middleware is the plain case:

```php
function with_log($next) {
    return function ($w, $r) use ($next) {
        error_log($r->method . " " . $r->url->path);
        $next($w, $r);
    };
}

$mux->handle("GET /users/{id}", with_log($this->showUser));
```

`$next` is a callable value built on the runtime that ran the script, and the request is answered on a worker. The value crosses; the execution does not follow it. Calling it runs the declaration on the worker, so what it echoes reaches this request's response, `connection_aborted()` inside it reports this client, and two requests through one wrapper never meet. The same holds for a callable read out of a shared object or an array, and for one handed to a binding that calls back - `usort`, `array_map`, `preg_replace_callback`, `call_user_func`.

This is what makes a runtime per worker sound. A `runner.Runtime` is one goroutine's execution - its frames, its output stack, its statics, its request - and reaching into another one is not a slow path but a concurrent map write. A whole runtime per worker, indexed by the handle the worker holds, is the arrangement; [../design.md](../design.md) records it under program re-entry, and `runner/fork.go` is the code.

## The client leaving

`connection_aborted()` reports whether the client this handler is answering has gone. `ignore_user_abort(true)` is what keeps the handler running long enough to ask, because without it the disconnect ends the handler where it next looks:

```php
$mux->handle("GET /slow", function ($w, $r) {
    ignore_user_abort(true);
    $ticks = 0;
    while ($ticks < 20) {
        usleep(100000);
        $ticks++;
        if (connection_aborted()) {
            // The work is allowed to finish; what is skipped is the part
            // nobody is left to read.
            error_log("aborted after " . $ticks . " ticks");
            return;
        }
    }
    $w->write("waited " . $ticks . " ticks\n");
});
```

`sleep()` and `usleep()` end early when the client goes away or the script runs out of time, because the wait is on the runtime context rather than on the clock alone.

## Stopping

`set_time_limit($seconds)` is a deadline on the context the interpreter checks and every binding is handed. `wait()` is a binding, so the script runs out of time while it is parked there, and running out of time is a fatal, as it is in php: nothing after it runs, and `register_shutdown_function` callbacks do run, with the clock off. That is why stopping the server belongs in one:

```php
register_shutdown_function(function () use ($http) {
    $http->shutdown();
    echo "server stopped\n";
});
```

`phpscript run` ends the script on SIGINT and SIGTERM, and reloads it on SIGHUP: the generation running is ended by cancelling the context it was given, its shutdown callbacks run, and the file is read again.

## Limits

| Limit                                                 | What happens                                                                                                                                                                                                               |
|-------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| A closure as a handler with `flatstack.enabled: true` | Refused where the route is written: the bytecode engine compiles a closure literal to a call on the VM that built it rather than to a callable value. `$this->fnName` and the name of a declared function both work there. |
| A handler writing to what it captured                 | Two requests writing to one value. The engine does not copy a receiver, so read-only is the contract.                                                                                                                      |
| `array($object, "method")` as a handler               | Refused where the route is written. Every other callable spelling takes it.                                                                                                                                                |
| A runtime per worker                                  | Forking installs the whole standard library, so a pool is paid at `listen()` rather than per request, and the worker count is memory as well as parallelism.                                                               |
