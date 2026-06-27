# PHP routing

`phpscript route [directory]` scans a PHP source tree for `// @route` comments and
registers those files as `net/http` `ServeMux` handlers. If no directory is
provided, the current directory is used.

```sh
phpscript route ./tests/fixtures/route
# or
phpscript route /workdir/route
```

Routes are declared in PHP comments:

```php
<?php
// @route GET /kv/{key}
$shm = new SharedMemory;
echo $shm->get($_PATH["key"]);
```

A route without a method registers both GET and POST. Explicit methods such as
PUT or DELETE must be written in the annotation.

## Shared host state

Each HTTP request gets a fresh PHP VM. Host applications can bind Go values into
each VM to provide shared process state or services. The example fixture under
`tests/fixtures/route` uses `SharedMemory`, a Go struct registered as the PHP class
`SharedMemory`, to provide a small key/value store and counters:

- `POST /kv/{key}` writes `$_POST["value"]` into shared memory.
- `GET /kv/{key}` reads the value back.
- `GET /stats/{counter}` reads request counters maintained with `incr()`.

Example session:

```sh
curl -d value=bar http://localhost:8080/kv/foo
# ok
curl http://localhost:8080/kv/foo
# bar
curl http://localhost:8080/stats/requests
# 2
```

A Go host wires the shared object by customizing each route runtime:

```go
shm := tests.NewSharedMemory()
mux := http.NewServeMux()
_, err := route.NewService(os.DirFS("tests/fixtures/route"), mux,
	route.WithRuntimeFunc(func(rt *runner.Runtime) {
		rt.SetContext(tests.SharedMemoryContext(context.Background(), shm))
		rt.RegisterConstructor("SharedMemory", tests.NewSharedMemoryBinding)
	}),
)
```

The command-line `phpscript route` entrypoint only loads the PHP files. Embedding
`route.Service` from Go is how applications add capabilities such as shared
memory, database handles, metrics, or other request-wide services.
