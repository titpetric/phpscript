# phpscript-http-bridge

A PHP program that is itself the HTTP server: it opens the socket, forks its workers and answers on them. `HTTP\Mux` is net/http's `ServeMux` with PHP callables as its handlers, `HTTP\Server::listen` forks the workers, and `server.php` is the whole application.

```php
$mux = new HTTP\Mux;
$mux->handle("GET /users/{id}", $this->showUser);
$http = new HTTP\Server($config["addr"], $mux, $config["workers"], $config["queue"]);
$http->listen();
```

`$this->showUser` is the method read without parentheses, which is a callable bound to the object. A handler takes `$w` and `$r` and nothing else: no superglobal is decoded, nothing carries over from the last request, and two requests answered at the same moment cannot see each other, each running on a runtime of its own.

## Running it

```sh
docker compose up -d --wait
curl http://127.0.0.1:8095/users/42
docker compose down
```

Or without docker, from the repository root:

```sh
go build -o bin/phpscript .
TESTSERVER_ADDR=127.0.0.1:8095 TESTSERVER_LIMIT=600 bin/phpscript run testdata/phpscript-http-bridge/server.php
```

`TESTSERVER_WORKERS` is how many requests are answered at once and `TESTSERVER_LIMIT` is how long the script runs before `set_time_limit` ends it. The limit is a deadline on the whole run and not on a request, so a server that outlives a sweep takes a limit past the sweep's whole duration.

The compose file mounts the binary. A benchmark measures the build under test, and an image carrying its own phpscript would report on the binary baked into that image.

## This is Path B

Nothing re-parses and nothing re-registers: the program is read once, each handler is resolved at `$mux->handle()`, and the workers are forked once at `listen()`. `testdata/phpscript-service` is Path A, where `phpscript server` builds a runtime per request.

A number from one never belongs in a table with a number from the other. [../../docs/agents/performance.md](../../docs/agents/performance.md) is the rule, [../../docs/use-cases/http-server.md](../../docs/use-cases/http-server.md) is the surface, and [../../docs/design.md](../../docs/design.md) is why both paths exist.

`BenchmarkTestServerRoute` in `tests/testserver_bench_test.go` reads this file by path, takes its class declaration, and mounts its router, so a change here reaches the allocation half of the harness directly. It fails when a route stops answering what it expects, so no 404 reaches the measurement.
