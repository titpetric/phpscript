# php-vanilla

`testdata/testserver.php` as a vanilla PHP application: php 8.5 and apache with mod_php, routed by Slim, in a container that comes up and goes down in one command. The API is the same one, route for route, and every route answers the same bytes, so the same load generator drives it and the difference reads off directly.

What it has that the phpscript side does not is a bootstrap per request. Apache hands the request to mod_php, the autoloader runs, Slim builds its router and its middleware stack, the handler answers, and everything built is torn down again. The phpscript side parses once, resolves its handlers at `$mux->handle()`, and forks its workers at `listen()`. That difference is the whole of what the comparison measures.

## It is the same program

`public/index.php` holds the same `Server` class. `config()` reads the same environment, `mount()` registers the same six handlers against the same patterns, and every handler body is the one in `testserver.php`:

```php
public function showUser(\HTTP\ResponseWriter $w, \HTTP\Request $r): void
{
    $w->header()->set('Content-Type', 'application/json');
    (new \JSON\Encoder($w))->encode(['id' => $r->path_value('id'), 'path' => $r->url->path]);
}
```

`$this->showUser(...)` in `mount()` is php's first-class callable syntax, which is what reading `$this->showUser` without parentheses answers on the phpscript side.

`src/` is what the handlers are written against, and it is the whole of what this directory adds:

| File                        | What it is                                                                |
|-----------------------------|---------------------------------------------------------------------------|
| `src/HTTP/Mux.php`          | net/http's ServeMux as Slim's router, and the one piece that is routing    |
| `src/HTTP/ResponseWriter.php` | net/http's ResponseWriter over a PSR-7 response                         |
| `src/HTTP/Request.php`      | the request, with `$method`, `$url`, `path_value`, `form_value`            |
| `src/HTTP/Url.php`          | net/url's URL, with `$path`, `$rawquery` and `query()`                     |
| `src/HTTP/Header.php`       | net/http's Header, canonicalised names and `set`/`add`/`get`               |
| `src/HTTP/Values.php`       | net/url's Values, which is what `query()` answers                          |
| `src/HTTP/Writer.php`       | io.Writer, so the encoder does not know it writes to a response            |
| `src/JSON/Encoder.php`      | JSON\Encoder: the writer it wraps, a newline per value, Go's escapes       |

Three things in `Mux` are the routing, and nothing else in the program knows about them. `handle()` takes net/http's `"METHOD /path"` pattern and reads the method off it, dropping the `{$}` that anchors an exact match where Slim matches exactly anyway. The handler is adapted to what Slim calls, which reorders the two arguments and carries the path values net/http keeps on the request itself. The output buffer is open for the length of the call, so a handler's `echo` reaches the response rather than the process's own output.

`run()` is the one method that could not carry over. Apache is the server, the process is already inside one request, and `run()` is what answers it: there is nothing to listen on, nothing to wait for, and the shutdown that stops it listening is apache's.

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
| hello | 34.0      | 47.0       | 505.0          | 27283    | 20080     | 1862          |
| users | 36.0      | 52.0       | 516.0          | 25304    | 18151     | 1786          |
| echo  | 43.0      | 56.0       | 525.0          | 21619    | 16690     | 1800          |
| index | 34.0      | 42.0       | 486.0          | 27233    | 22273     | 1942          |

Apache's own floor, serving a static file out of the same container, against the two PHP columns on the same box:

| measurement                         | p50 us | req/s |
|-------------------------------------|--------|-------|
| apache, static file                 | 64.0   | 13659 |
| apache, php and Slim through /hello | 486.0  | 1981  |
| phpscript, /hello                   | 47.0   | 20080 |

The web server alone costs more per request than phpscript's whole answer. What the static floor leaves is the PHP bootstrap: the autoloader, Slim's app and middleware stack, and the teardown, once per request.

Memory with the sweep answered, from the same run:

| side    | rss kb | what it counts                              |
|---------|--------|---------------------------------------------|
| go      | 13008  | one process, workers as threads inside it   |
| php     | 25888  | one process, workers as threads inside it   |
| vanilla | 24914  | container cgroup, apache and its 4 children |

The two measurements are not interchangeable. A host process reports its own RSS; the container reports a cgroup counting apache's parent and every child it forked.

## Where it differs from testserver.php

Four differences, and every one of them is php or apache rather than a choice:

- `/slow` writes and flushes a space per tick. `connection_aborted()` reports a write that failed, so a loop writing nothing never notices the client leaving. The phpscript side reads the request's context, needs no write, and replies with the final line alone. Both sides pass the sweep's abort check.
- `JSON\Encoder` passes `JSON_UNESCAPED_SLASHES | JSON_HEX_AMP`, which is what php needs to write Go's bytes: php escapes a forward slash where Go does not, and Go escapes an ampersand where php does not. A raw query string carrying two parameters reaches the second one. Two differences are left and neither has a flag that closes it: php escapes non-ASCII to `\uXXXX` where Go writes UTF-8, and Go escapes `<` and `>` to lowercase `<` where php's `JSON_HEX_TAG` writes `<`. No route here produces either.
- `run()` neither listens nor waits, apache owning both, and `config()` reads the worker count back for the manifest alone: `conf/mpm.conf` is where apache is told it.
- The reply headers differ by a few bytes either way. `expose_php` is off so no `X-Powered-By` goes out, and apache sends a `Server` header where net/http does not.

The error paths match. An unrouted path answers `404 page not found` and a wrong method answers `Method Not Allowed` with an `Allow` header, both with net/http's content type and its `nosniff`, because Slim has no handler for either exception and would answer 500 to both.

## The image

`php:8.5-apache`, which ships opcache compiled in, so `conf/opcache.ini` configures it and nothing builds it. No extension is installed: the API needs core and json alone. A route that reads a database wants one `docker-php-ext-install pdo_sqlite` line in the Dockerfile and nothing else.

`conf/` is what the image replaces in apache's config: a fixed worker count in `mpm.conf`, the front controller rewrite in `vhost.conf`, one `Listen` in `ports.conf`, and no access log, because one line per request through a pipe is work the phpscript side is not doing and a sweep writes millions of them.

`composer.lock` is committed and the image installs from it, so a rebuild resolves nothing and two runs measure the same Slim. The autoloader is built `--classmap-authoritative`, so the eight classes in `src/` cost a map lookup each and no directory walk. `/hello` measured 483 us at the p50 with the whole program in one file and 486 us with it split across them, on the same box minutes apart.
