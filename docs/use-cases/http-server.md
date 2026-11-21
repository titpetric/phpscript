# HTTP server bindings

The standard runtime registers the Go-backed `HTTP\Mux` and `HTTP\Server` classes. They wrap `net/http`'s `ServeMux` and `Server`. A PHP program can itself be the server: it builds the router, listens, and owns the process until it stops.

Two ways to serve exist. The other is `phpscript server`, which scans a source tree for `// @route` comments and runs a file per request: see [routing.md](routing.md). Choose that one for an application laid out as pages and endpoints. Choose this one when the program is the server: a service with a handful of routes, a test server, something that starts and stops at will.

`testdata/phpscript-http-bridge/server.php` is a working example of everything below, and `testdata/goserver/main.go` is the same routes in Go beside it. `testdata/phpscript-service` repeats the same API, its script holding no router at all, as route annotations under [`phpscript server`](../cli/server.md).

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

`listen()` binds and returns the address without blocking. `wait()` blocks until the script runs out of time. `shutdown()` stops accepting and lets in-flight work finish, while `close()` drops it. An `$addr` of `127.0.0.1:0` binds a port the system picks. `listen()` and `addr()` report that port. A test thus takes a free port without guessing.

## What a handler is

A handler is a callable, in five spellings:

| Spelling                        | Written as                                                      |
|---------------------------------|-----------------------------------------------------------------|
| A closure                       | `$mux->handle("GET /x", function ($w, $r) { ... })`             |
| A method bound to its receiver  | `$mux->handle("GET /x", $this->index)` or `$this->index(...)`   |
| An object declaring `__invoke`  | `$mux->handle("GET /x", new Index)` or `(new Index)(...)`       |
| The name of a declared function | `$mux->handle("GET /x", "handle_index")` or `handle_index(...)` |
| A static method                 | `$mux->handle("GET /x", "Site::index")` or `Site::index(...)`   |

`$this->fnName` reads the method without calling it. That is the [first-class callable syntax](../reference/functions/README.md#first-class-callable-syntax) `$this->fnName(...)` in a shorter spelling. Both spellings parse and answer the same bound method, but the shorter one diverges: php reads it as an undefined property. A server written as a class registers it:

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

An `__invoke` is that same binding under php's reserved name. A static method names no receiver at all: the runtime builds an empty instance per call, and `self::` resolves. Two requests share nothing.

`array($object, "method")` and `array("Class", "method")` are not accepted here. They stay callable everywhere else - `call_user_func`, `usort`, a Go binding's callback and the rest take them. This router keeps refusing them. A handler is a function of its arguments. Wrapping one in an array to name a method is a spelling this router leaves out, and `(...)` now writes the same thing. The refusal names the spelling and its replacement. A route registered with one fails on that line, ahead of the first request.

## Patterns

The pattern is `net/http`'s: a method, a path, with `{name}` segments the handler reads back through `$r->path_value($name)`:

```php
$mux->handle('GET /{$}', $index);          // the root, and nothing else
$mux->handle("GET /users/{id}", $show);     // a path value
$mux->handle("POST /echo", $echo);          // one method
$mux->handle("/health", $health);           // any method
```

`GET /` is a `net/http` subtree and answers for every path no other route claimed, while `GET /{$}` anchors it to the root alone. Write `'{$}'` in single quotes: in a double-quoted string `{$` starts an interpolation.

## Answering

The worker calls the handler with the response writer and the request, the two `net/http` values themselves:

```php
$w->header()->set("Content-Type", "application/json");
$w->write($body);
(new JSON\Encoder($w))->encode(array("id" => $r->path_value("id")));
echo "this reaches the response too\n";
```

`echo` reaches the response: the runtime answering the request writes there for the length of the call. `$r->url->query()->get($name)` is the first value for a name, `$r->form_value($name)` reads the query and the body together, and `$r->method`, `$r->url->path`, `$r->url->rawquery` and `$r->user_agent()` are the request as Go names it.

A handler that throws is a single request's problem: the runtime reports it to its error sink and answers with a 500 if nothing has gone out yet. The server goes on.

## Serving files

`HTTP\ServeFile($w, $r, $path)` answers one request with one file, writing the content type, the byte range and the conditional headers `net/http` writes for a file:

```php
$mux->handle("GET /files/{name...}", function ($w, $r) {
    HTTP\ServeFile($w, $r, "public/" . $r->path_value("name"));
});
```

A whole subtree needs no PHP handler. `new HTTP\FileServer($dir)` is a `net/http` handler, `$mux->handle` registers one in place of a callable, and `HTTP\StripPrefix($prefix, $handler)` cuts the mount off the path the way `net/http` does:

```php
$mux->handle("GET /assets/", HTTP\StripPrefix("/assets/", new HTTP\FileServer("public")));
```

That route runs no PHP per request and takes no worker: the mux registers the Go handler itself and that handler answers. The mux refuses a Go handler under a `use` middleware, on the line the route is on, because `use` calls a middleware with the handler and no script can call a Go value.

A path resolves against the source filesystem the script itself reads, and a name climbing out of the root resolves back under the root: `../../etc/passwd` names a file of the tree or no file at all. The tree is the script's own: `FileServer` rooted at the source tree publishes the sources, and the directory to name is the one holding what the server is for. `Runtime\PS\HTTP\Files` is the area, and a host that selects areas leaves it out to register no file serving at all.

`FileServer` lists a directory holding no `index.html`, as `net/http` does.

## Groups and middleware

`$mux->use($middleware)` wraps the handlers registered after it. `$mux->group($fn)` calls `$fn` with a mux that shares the parent's routes and starts with its middleware. A `use` inside the group applies to the group alone, isolating a module's routes from the rest of the router. A middleware takes the next handler, and returns a handler that wraps it. The shape matches `chi`:

```php
$mux->group(function (HTTP\Mux $api) {
    $api->use(function ($next) {
        return function ($w, $r) use ($next) {
            $w->header()->set("X-Api", "1");
            $next($w, $r);
        };
    });
    $api->handle("GET /api/users/{id}", $this->showUser);
});
```

The middleware runs once, as the mux registers the route. Workers answer through the handler it returns.

## Binding the service router

A `@startup` job reaches the router of the service with `Runtime\Router::Bind`. The callable receives an `HTTP\Mux` and the routes it registers join the router the service serves, ahead of the first request:

```php
<?php
// @startup

use Runtime\Router;

Router::Bind(function (HTTP\Mux $router) {
    $router->handle("GET /ping", fn ($w, $r) => $w->write("pong"));
});
```

The mux registers straight on the router of the service and keeps no routes, leaving it none to list. It belongs to the call. A `handle` on it after `Bind` has returned throws a `RuntimeException`. `Router::Bind` called from a request throws it too, because the router is not mutable once the service is up. Every pattern needs a method: `GET /ping`, never bare `/ping`, because the service router registers a method and path.

## The worker pool

`HTTP\Server`'s third argument is how many requests the server answers at once and its fourth is how deep the queue behind them is. Omitted, they are the number of cores and 1024.

```php
$http = new HTTP\Server($addr, $mux, 4, 64);
```

Each worker holds a private runtime, forked from the script's runtime. Workers are the parallelism and the queue is the backpressure. A request that finds every worker busy waits its turn and starts no new runtime. A filling queue means the caller waits, which is the signal that it should. A handler doing no IO can use about as many workers as there are cores, while one that waits on a database wants more.

## What a handler can reach

A handler starts on a clean stack holding only `$w` and `$r`. The runtime decodes no superglobal. No state persists between requests. Two requests answered at the same moment cannot see each other.

A handler holds whatever it captured: a closure's `use (...)` values, the `$this` it binds, or a bound method's receiver. Every request answering through the handler shares them. Each request runs on a private runtime without copying the capture. Read them. Writing to one from a handler means two requests writing to one value. The engine cannot make that safe without copying the object. A shared receiver is not the place for that.

Autoloaders come along as well. The worker that first names a class loads it once per worker. Script state does not come along: globals, `static $x`, constants the script defined, the list of included files, the output it produced. A handler reaches its captures, the receiver's fields, and all declarations ahead of `listen()`.

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

`$next` is a callable value built on the runtime that ran the script. A worker answers the request. The value crosses without the execution following it. Calling it runs the declaration on the worker. The echo reaches this request's response. `connection_aborted()` inside the declaration reports this client. Two requests through a wrapper never meet. The same holds for a callable read out of a shared object or an array, and for one handed to a binding that calls back - `usort`, `array_map`, `preg_replace_callback`, `call_user_func`.

This makes a runtime per worker sound. A `runner.Runtime` covers one goroutine's execution: its frames, its output stack, its statics, its request. Reaching into another one is a concurrent map write. A whole runtime per worker, indexed by the worker's handle, is the arrangement. [../design.md](../design.md) records it under program re-entry, and `runner/fork.go` is the code.

## The client leaving

`connection_aborted()` reports whether the client this handler is answering has gone. `ignore_user_abort(true)` keeps the handler running long enough to ask. Without that call, a disconnect ends the handler where it next looks:

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

`sleep()` and `usleep()` end early when the client goes away or the script runs out of time, because the wait is on the runtime context and not on the clock alone.

## Stopping

`set_time_limit($seconds)` sets a deadline on the context the interpreter checks and all bindings read. `wait()` is a binding. The script runs out of time parked there. Running out of time is a fatal, as in php: no statement after it runs, and `register_shutdown_function` callbacks do run, with the clock off. That is why stopping the server belongs in one:

```php
register_shutdown_function(function () use ($http) {
    $http->shutdown();
    echo "server stopped\n";
});
```

`phpscript run` ends the script on SIGINT and SIGTERM, and reloads it on SIGHUP: cancelling its context ends the running generation, its shutdown callbacks run, and `phpscript run` reads the file again.

## Limits

| Limit                                                 | What happens                                                                                                                                                                                                                      |
|-------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| A closure as a handler with `flatstack.enabled: true` | Refused on the line registering the route: the bytecode engine compiles a closure literal to a call on the VM that built it rather than to a callable value. `$this->fnName` and the name of a declared function both work there. |
| A handler writing to what it captured                 | Two requests writing to one value. The engine does not copy a receiver and read-only is the contract.                                                                                                                             |
| `array($object, "method")` as a handler               | Refused on the line registering the route. Every other callable spelling takes it.                                                                                                                                                |
| A runtime per worker                                  | Forking installs the whole standard library and the cost lands at `listen()` rather than per request, and the worker count is memory as well as parallelism.                                                                      |
