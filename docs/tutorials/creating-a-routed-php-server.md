# Creating a routed PHP server

This tutorial mirrors the route test suite in `tests/route_test.go`. It builds a
small key/value HTTP API from PHP endpoint files, while keeping shared process
state in Go.

## Source tree

The PHP route source tree lives in `tests/fixtures/route`:

```text
tests/fixtures/route/
├── kv/
│   ├── get.php
│   └── post.php
└── stats.php
```

Each PHP file declares one route with a `// @route` comment. The Go route loader
scans this tree recursively and registers matching `net/http` routes.

## What lives in Go

Go owns the server shell and process-wide capabilities:

- `http.ServeMux` and route registration through `route.NewService`.
- The embedded filesystem used by tests, via `fixturesFS` and `fs.Sub`.
- `SharedMemory`, the synchronized in-memory key/value and counter store.
- The PHP class binding: `rt.RegisterConstructor("SharedMemory", ...)`.
- The request test harness using `httptest`.

The test creates one shared Go object and injects it into every request VM:

```go
shm := NewSharedMemory()
mux := http.NewServeMux()
_, err := route.NewService(root, mux, route.WithRuntimeFunc(func(rt *runner.Runtime) {
	rt.SetContext(SharedMemoryContext(context.Background(), shm))
	rt.RegisterConstructor("SharedMemory", NewSharedMemoryBinding)
}))
```

Every HTTP request gets a fresh PHP runtime, but `new SharedMemory` resolves to
the same Go `shm` value. That is what makes it behave like shared memory.

## What PHP handles

PHP owns endpoint behavior:

- Route declarations with `// @route METHOD /path/{param}`.
- Reading path params from `$_PATH`.
- Reading form data from `$_POST`.
- Calling host-provided methods on `SharedMemory`.
- Producing the response body with `echo`.

`tests/fixtures/route/kv/post.php` writes a value:

```php
<?php
// @route POST /kv/{key}
$shm = new SharedMemory;
$shm->incr("requests");
$shm->incr("post");
$shm->set($_PATH["key"], $_POST["value"]);
echo "ok";
```

`tests/fixtures/route/kv/get.php` reads it:

```php
<?php
// @route GET /kv/{key}
$shm = new SharedMemory;
$shm->incr("requests");
$shm->incr("get");
echo $shm->get($_PATH["key"]);
```

`tests/fixtures/route/stats.php` exposes counters:

```php
<?php
// @route GET /stats/{counter}
$shm = new SharedMemory;
echo $shm->count($_PATH["counter"]);
```

## The test flow

`ExampleSharedMemory` exercises the actual endpoints through the mux:

1. `POST /kv/color` with `value=blue` returns `ok`.
2. `GET /kv/color` returns `blue`.
3. `GET /stats/requests` returns `2`, proving both endpoint requests incremented
   the same Go-side shared counter.

Expected example output:

```text
ok
blue
2
```

This split keeps infrastructure, synchronization, and durable services in Go,
while PHP remains the small request scripting layer for endpoint behavior.
