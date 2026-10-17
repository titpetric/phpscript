# php-vanilla

`testdata/testserver.php` written as a vanilla PHP application: php 8.5 and apache with mod_php, routed by Slim, in a container that comes up and goes down in one command. The API is the same one, route for route, and every route answers the same bytes, so the same load generator drives it and the difference reads off directly.

What it has that the phpscript side does not is a bootstrap per request. Apache hands the request to mod_php, the autoloader runs, Slim builds its router and its middleware stack, the handler answers, and everything built is torn down again. The phpscript side parses once, resolves its handlers at `$mux->handle()`, and forks its workers at `listen()`. That difference is the whole of what the comparison measures.

## Running it

```sh
docker compose up -d --wait
curl http://127.0.0.1:8098/users/42
docker compose down
```

`VANILLA_PORT` is the port and `VANILLA_WORKERS` is how many requests it answers at once, which is `TESTSERVER_WORKERS` on the phpscript side. Both reach apache's config through the environment, so changing either needs no rebuild:

```sh
VANILLA_PORT=9000 VANILLA_WORKERS=8 docker compose up -d --wait
```

The container shares the host's network namespace. A published port goes through docker-proxy or a NAT hop, and the measurement here is tens of microseconds, so sharing the namespace is what puts the generator and the server the same distance apart on both sides of the comparison.

## Benchmarking it against phpscript

`scripts/bench-http.sh` runs it as the `vanilla` target and brings the stack up and down itself:

```sh
BENCH_TARGETS="php vanilla" scripts/bench-http.sh after
BENCH_TARGETS="go php vanilla" scripts/bench-http.sh after
```

The sweep reniced the container's processes to match the other two sides, records what each side holds in memory with its last request answered, and checks `/slow` on every side. [../../docs/agents/performance.md](../../docs/agents/performance.md) is the protocol the numbers have to satisfy.

## What a run reports

Three segments of 2s per route, `VANILLA_WORKERS=4` against `TESTSERVER_WORKERS=4`, both at `nice -n -20`, on four cores. `go` is `testdata/testserver.go`, `php` is `testdata/testserver.php` on phpscript, `vanilla` is this directory.

| route | go p50 us | php p50 us | vanilla p50 us | go req/s | php req/s | vanilla req/s |
|-------|-----------|------------|----------------|----------|-----------|---------------|
| hello | 35.0      | 47.0       | 483.0          | 26431    | 19554     | 1939          |
| users | 37.0      | 50.0       | 480.0          | 24933    | 18025     | 1941          |
| echo  | 43.0      | 54.0       | 496.0          | 21576    | 16954     | 1876          |
| index | 35.0      | 42.0       | 459.0          | 25117    | 21993     | 2050          |

Apache's own floor is 64 us at the p50 and 13659 req/s, measured by serving a static file out of the same container. The web server alone costs more per request than phpscript's whole answer, and the remaining 420 us is the PHP bootstrap.

Memory with the sweep answered, from the same run:

| side    | rss kb | what it counts                             |
|---------|--------|--------------------------------------------|
| go      | 13228  | one process, workers as threads inside it  |
| php     | 25424  | one process, workers as threads inside it  |
| vanilla | 20183  | container cgroup, apache and its 4 children |

The two measurements are not interchangeable. A host process reports its own RSS; the container reports a cgroup counting apache's parent and every child it forked.

## Where it differs from testserver.php

- `json_encode` is passed `JSON_UNESCAPED_SLASHES`, because php escapes a forward slash in a string and Go's encoder does not. Without it a path reads `"\/users\/42"` here and `"/users/42"` there, and the reply sizes stop matching. php and Go still disagree on two things no route here produces: php escapes non-ASCII to `\uXXXX` where Go writes UTF-8, and Go escapes `<`, `>` and `&` where php does not.
- `/slow` flushes a space per tick. `connection_aborted()` reports a write that failed, so a loop that writes nothing never notices the client leaving; the phpscript side reads the request's context and needs no write, which is why its reply is the final line alone.
- `/` is registered as `'/'`. Slim matches a route exactly and has no subtree patterns, so nothing corresponds to the `{$}` that anchors the pattern on the net/http side.
- The reply headers differ by a few bytes either way. `expose_php` is off so no `X-Powered-By` goes out, but apache sends a `Server` header and net/http does not.

## The image

`php:8.5-apache`, which ships opcache compiled in, so `conf/opcache.ini` configures it and nothing builds it. No extension is installed: the API needs core and json alone. A route that reads a database wants one `docker-php-ext-install pdo_sqlite` line in the Dockerfile and nothing else.

`conf/` is what the image replaces in apache's config: a fixed worker count in `mpm.conf`, the front controller rewrite in `vhost.conf`, one `Listen` in `ports.conf`, and no access log, because one line per request through a pipe is work the phpscript side is not doing and a sweep writes millions of them.

`composer.lock` is committed and the image installs from it, so a rebuild resolves nothing and two runs measure the same Slim.
