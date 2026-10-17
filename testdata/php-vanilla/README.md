# php-vanilla

`testdata/phpscript-http-bridge/server.php` on Slim, served two ways from one copy of `public/index.php`: php 8.5 and apache with mod_php, and php 8.5 and nginx with php-fpm. The API is the same one, route for route, and every route answers the same bytes, so the same load generator drives it and the difference reads off directly.

What it has that the phpscript side does not is a bootstrap per request. The autoloader runs, Slim builds its App, its router and its middleware stack, the handler answers, and everything built is torn down again. The phpscript side parses once, resolves its handlers at `$mux->handle()`, and forks its workers at `listen()`. That difference is what the column measures.

## It is the same program with the signatures changed

The `Server` class is the same class: `config()` reads the environment, `mount()` registers the same six handlers against the same patterns, each handler is a method, `$this->showUser(...)` is the method as a bound callable, and `run()` is `main()`. What a handler is written against is what differs, and Slim and PSR-7 force four substitutions:

| In the bridge                         | Here                                           |
|---------------------------------------|------------------------------------------------|
| `$w->header()->set($n, $v)`           | `$response = $response->withHeader($n, $v)`    |
| `$w->write($s)` and `echo $s`         | `$response->getBody()->write($s)`              |
| `(new JSON\Encoder($w))->encode($a)`  | `json_encode($a, JSON_FLAGS) . "\n"`           |
| `$r->path_value("id")`                | `$args["id"]`                                  |

A PSR-7 response is immutable, so a handler returns the response it built where the bridge writes to the one it was handed.

## Running it

```sh
docker compose --profile apache up -d --wait
curl http://127.0.0.1:8098/users/42
docker compose --profile apache down
```

```sh
docker compose --profile nginx up -d --wait
curl http://127.0.0.1:8099/users/42
docker compose --profile nginx down
```

Profiles and not two directories, because the application is one application: a change to `public/index.php` reaches both stacks and neither can drift from the other. `VANILLA_PORT`, `VANILLA_NGINX_PORT` and `VANILLA_WORKERS` are the three settings; the worker count is `TESTSERVER_WORKERS` on the phpscript side.

Every container shares the host's network namespace. A published port goes through docker-proxy or a NAT hop, and the measurement is tens of microseconds, so sharing the namespace is what puts the generator and the server the same distance apart on both sides of the comparison. It is also what lets nginx reach fpm on loopback without the two sharing a filesystem.

## Benchmarking it against phpscript

`scripts/bench-http.sh` runs both stacks as targets and brings them up and down itself:

```sh
BENCH_TARGETS="bridge vanilla-apache vanilla-nginx" scripts/bench-http.sh after
BENCH_TARGETS="goserver bridge service vanilla-apache vanilla-nginx" scripts/bench-http.sh after
```

It reniced the containers' processes to match the other sides, records what each holds in memory with its last request answered, and checks `/slow` on every side. [../../docs/agents/performance.md](../../docs/agents/performance.md) is the protocol the numbers have to satisfy.

## Where the cost is

Each stage of the bootstrap, measured through the same apache container by serving a file that stops at that stage. Three segments of 3s at concurrency 1, four cores.

| stage                                        | p50 us | req/s |
|----------------------------------------------|--------|-------|
| `static.txt`, apache alone                   | 64.0   | 13870 |
| `_bare.php`, plus mod_php                    | 101.0  | 9051  |
| `_autoload.php`, plus the composer autoloader | 131.0  | 6902  |
| `_slim.php`, plus Slim's App and one route   | 377.0  | 2542  |
| `/hello`, the whole application              | 473.0  | 1954  |

opcache is on and `validate_timestamps` is off, so none of that is compilation: a bare php file answers in 101 us. Slim's per-request App construction is the single largest stage. The worker count is not in it either, concurrency 1 using one worker by definition.

The two web servers on the same application, four segments of 2s at concurrency 1:

| stack               | static p50 us | static req/s | /hello p50 us | /hello req/s |
|---------------------|---------------|--------------|---------------|--------------|
| nginx with php-fpm  | 42.0          | 23053        | 370.0         | 2503         |
| apache with mod_php | 66.0          | 12773        | 480.0         | 1920         |

nginx wins the static floor outright and the php routes by a fifth. Most of that margin is `fastcgi_keep_conn on`: nginx holding its connections to fpm took `/hello` from 503 us to 370 us, on top of answering every request instead of 1.5% of them with a 502.

## Where it differs from the bridge

Four differences, and every one of them is php or the web server rather than a choice:

- `/slow` writes and flushes a space per tick. `connection_aborted()` reports a write that failed, so a loop writing nothing never notices the client leaving. The phpscript side reads the request's context, needs no write, and replies with the final line alone. Both sides pass the sweep's abort check.
- `json_encode` is passed `JSON_UNESCAPED_SLASHES | JSON_HEX_AMP`, which is what php needs to write Go's bytes: php escapes a forward slash where Go does not, and Go escapes an ampersand where php does not. A raw query string carrying two parameters reaches the second one. Two differences are left and neither has a flag that closes it: php escapes non-ASCII to `\uXXXX` where Go writes UTF-8, and Go escapes `<` and `>` to lowercase `<` where php's `JSON_HEX_TAG` writes `<`. No route here produces either.
- `run()` neither listens nor waits, the web server owning both, and `config()` reads the worker count back for the record alone: `conf/apache/mpm.conf` and `conf/nginx/fpm-pool.conf` are where each stack is told it.
- The reply headers differ by a few bytes either way. `expose_php` is off so no `X-Powered-By` goes out, and both web servers send a `Server` header where net/http does not.

The error paths match. An unrouted path answers `404 page not found` and a wrong method answers `Method Not Allowed` with an `Allow` header, both with net/http's content type and its `nosniff`, because Slim has no handler for either exception and would answer 500 to both.

## The images

`conf/` is what each image replaces in its server's config, and `conf/opcache.ini` and `conf/php.ini` are shared by both. No php extension is installed: the API needs core and json alone, and php 8.5 ships opcache compiled in. A route that reads a database wants one `docker-php-ext-install pdo_sqlite` line and nothing else.

Neither stack writes an access log. One line per request through a pipe is work the phpscript side is not doing and a sweep writes millions of them.

`composer.lock` is committed and the images install from it, so a rebuild resolves nothing and two runs measure the same Slim. The autoloader is built `--classmap-authoritative`.

`fastcgi_keep_conn on` in the nginx config is not optional. The upstream block keeps connections to fpm, and without it nginx closes its end after each request while fpm holds its own open, so a reused connection fails with `upstream prematurely closed connection while reading response header`. That was roughly 1.5% of requests under load, answered 502 and counted in the sweep's `non2xx` column.

php-fpm expands no environment variable in its pool config, so `VANILLA_WORKERS` is substituted into it at startup. Baking it at build time would leave a stale image answering with a worker count nobody asked for whenever the variable changed without `--build`.
