# Request routing

The phpscript runtime contains a routing helper that turns PHP files into
`net/http` handlers. It keeps the server shell and shared services in Go,
while PHP owns small endpoint behavior.

When you run `phpscript route [directory]`, it scans a PHP source tree
for `// @route` comments and registers those files as `net/http`
`ServeMux` handlers. If no directory is provided, the current directory
is used.

```sh
phpscript route ./tests/fixtures/route
```

`phpscript server [directory]` also registers annotated routes, while serving
only the application's `public/` directory directly. Under `server`, route
files belong outside `public/`; annotations under `public/` are ignored.

The route test fixture uses this source tree:

```text
tests/fixtures/route/
├── kv/
│   ├── get.php
│   └── post.php
└── stats.php
```

Routes are declared in PHP comments:

```php
<?php

// @route GET /kv/{key}

$shm = new SharedMemory;

echo $shm->get($_PATH["key"]);
```

A route without a method registers both GET and POST. Explicit methods such as
PUT or DELETE must be written in the annotation. There is no specific handling
available for HEAD or OPTIONS requests, you have to bind them explicitly.

PHP endpoint files handle:

- route declarations with `// @route METHOD /path/{param}`
- path params from `$_PATH`
- query and form data from `$_GET` and `$_POST`
- host-provided bindings such as `SharedMemory`
- response bodies with `echo`

## Shared host state

Each HTTP request gets a fresh PHP VM. Host applications can bind Go values into
each VM to provide shared process state or services. The example fixture under
`tests/fixtures/route` uses `SharedMemory`, a Go struct registered as the PHP class
`SharedMemory`, to provide a small key/value store and counters:

- `POST /kv/{key}` writes `$_POST["value"]` into shared memory.
- `GET /kv/{key}` reads the value back.
- `GET /stats/{counter}` reads request counters maintained with `incr()`.

The write endpoint looks like this:

```php
<?php

// @route POST /kv/{key}

$shm = new SharedMemory;
$shm->incr("requests");
$shm->incr("post");
$shm->set($_PATH["key"], $_POST["value"]);

echo "ok";
```

The read endpoint mirrors it:

```php
<?php

// @route GET /kv/{key}

$shm = new SharedMemory;
$shm->incr("requests");
$shm->incr("get");

echo $shm->get($_PATH["key"]);
```

And the stats endpoint proves that requests share the same Go-side state:

```php
<?php

// @route GET /stats/{counter}

$shm = new SharedMemory;

echo $shm->count($_PATH["counter"]);
```

Example session:

```sh
curl -d value=bar http://localhost:8080/kv/foo
# ok
curl http://localhost:8080/kv/foo
# bar
curl http://localhost:8080/stats/requests
# 2
```

The command-line `phpscript route` entrypoint only loads the PHP files.
Embedding `route.Service` from Go is how applications add capabilities
such as shared memory, database handles, metrics, or other request-wide
services.

```go
shm := NewSharedMemory()
mux := http.NewServeMux()

_, err := route.NewService(root, mux, route.WithRuntimeFunc(func(rt *runner.Runtime) {
	rt.SetContext(SharedMemoryContext(context.Background(), shm))
	rt.RegisterConstructor("SharedMemory", NewSharedMemoryBinding)
}))
```

Go owns the `http.ServeMux`, route registration, synchronization, durable
services, and bindings. Every request gets a fresh PHP runtime, but
`new SharedMemory` resolves to the same Go `shm` value.

## References

- [Route tests](../../tests/route_test.go)
- [SharedMemory binding](../../tests/route.go)
- [Route fixtures](../../tests/fixtures/route)
