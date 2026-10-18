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

A PSR-7 response is immutable, so a handler returns the response it built where the bridge writes to the one its caller passed in.

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

Profiles and not two directories, because the application is one application: a change to `public/index.php` reaches both stacks, so neither one answers bytes the other does not. `VANILLA_PORT`, `VANILLA_NGINX_PORT` and `VANILLA_WORKERS` are the three settings; the worker count is `TESTSERVER_WORKERS` on the phpscript side.

Every container shares the host's network namespace. A published port crosses docker-proxy or a NAT hop, and the measurement is tens of microseconds, so the shared namespace puts the generator and the server the same distance apart on both sides of the comparison. It is also what lets nginx reach fpm on loopback without the two sharing a filesystem.

## Benchmarking it against phpscript

`scripts/bench-http.sh` runs both stacks as targets and brings them up and down itself:

```sh
BENCH_TARGETS="bridge vanilla-apache vanilla-nginx" scripts/bench-http.sh after
BENCH_TARGETS="goserver bridge service vanilla-apache vanilla-nginx" scripts/bench-http.sh after
```

It renices the containers' processes to match the other sides, records what each holds in memory with its last request answered, and checks `/slow` on every side. [../../docs/agents/performance.md](../../docs/agents/performance.md) is the protocol the numbers have to satisfy.

## Where the cost is

Three stages on both stacks, each a path in `public/` that stops where its name says. `static.txt` is the web server alone, `_bare.php` is the `/hello` handler with no autoloader and no framework under it, and `/hello` is the whole application. All three are files in this directory, so every row below is reproducible:

```sh
scripts/benchmark.php --targets php-vanilla-apache,php-vanilla-nginx --concurrency 1 \
	--endpoints static=/static.txt,bare=/_bare.php,hello=/hello?name=sprint
```

Six segments of 3s per row at concurrency 1, four workers a side, four cores:

| stage                           | apache p50 us | apache req/s | nginx p50 us | nginx req/s |
|---------------------------------|---------------|--------------|--------------|-------------|
| `static.txt`, the server alone  | 65.0          | 14170        | 42.0         | 22445       |
| `_bare.php`, plus php           | 102.0         | 8947         | 115.0        | 8028        |
| `/hello`, the whole application | 451.0         | 2132         | 376.0        | 2515        |

opcache is on and `validate_timestamps` is off, so none of that is compilation, as the `_bare.php` row shows. Slim's per-request `App` construction is the single largest stage, and it is the whole of the distance between the second row and the third. The worker count is not in it either, concurrency 1 using one worker by definition.

The two stacks cross over between the rows: nginx serves a static file in well under what apache takes and answers the application faster, while apache answers `_bare.php` faster, because mod_php runs php inside the process that accepted the connection and the fastcgi hop to fpm costs more than the whole of a trivial script. A measurement taken on one row says nothing about the other.

nginx's margin on the application is mostly `fastcgi_keep_conn on`, which holds its connections to fpm open across requests:

| nginx, `POST /echo` under load | non-2xx | of    | `/hello` p50 us |
|--------------------------------|---------|-------|-----------------|
| without the directive          | 125     | 8540  | 503.0           |
| with it                        | 0       | 14655 | 370.0           |

## The JIT costs and buys nothing here

php ships `opcache.jit=disable` and this measurement is why. `VANILLA_JIT` selects the mode, two blocks per arm, counterbalanced, the same six segments of 3s at concurrency 1:

| arm                        | apache `_bare.php` p50 us | apache `/hello` p50 us | nginx `_bare.php` p50 us | nginx `/hello` p50 us |
|----------------------------|---------------------------|------------------------|--------------------------|-----------------------|
| `disable`, php's default   | 103.0, 102.0              | 451.0, 449.0           | 115.0, 114.0             | 376.0, 372.0          |
| `tracing`                  | 102.0, 102.0              | 459.0, 461.0           | 114.0, 115.0             | 384.0, 384.0          |
| `function`                 | 102.0, 101.0              | 473.0, 472.0           | 114.0, 115.0             | 402.0, 405.0          |

`_bare.php` does not move. `/hello` gets slower, and the mode that compiles more is the one that is worse.

What the JIT buffer holds says why, read through `opcache_get_status()["jit"]` in the apache container:

| mode       | enabled | buffer used, cold | buffer used, after 200 requests to `/hello` |
|------------|---------|-------------------|---------------------------------------------|
| `tracing`  | true    | 2293              | 2293                                        |
| `function` | true    | 937248            | 938725                                      |

In `tracing` the buffer stops moving: the JIT watches for a hot loop or a hot function, `opcache.jit_hot_loop` is 61 iterations and `opcache.jit_hot_func` is 127 calls, and a request path has no loop that runs 61 times. It is a few hundred opcodes of glue over calls into C, run once each, so there is nothing for a trace to cover. In `function` mode the compiler runs anyway and emits most of a megabyte of machine code that the measurement then pays for in instruction cache and in compilation at startup, with the same C calls doing the same work underneath.

Every php web request path has that shape, and php's default is off for it. A JIT earns its buffer where php itself is the thing running hot: a numeric loop, an image filter, a parser written in php. None of those is an HTTP handler, and the application here is the extreme case, a handler that reads one superglobal and writes one line.

## Where it differs from the bridge

Four differences, and php or the web server forces each one:

- `/slow` writes and flushes a space per tick. `connection_aborted()` reports a write that failed, so a loop writing nothing never notices the client leaving. The phpscript side reads the request's context, needs no write, and replies with the final line alone. Both sides pass the sweep's abort check.
- `json_encode` is passed `JSON_UNESCAPED_SLASHES | JSON_HEX_AMP`, the two flags php needs to write Go's bytes: php escapes a forward slash where Go does not, and Go escapes an ampersand where php does not. A raw query string carrying two parameters reaches the second one. Two differences are left and neither has a flag that closes it: php escapes non-ASCII to `\uXXXX` where Go writes UTF-8, and Go escapes `<` and `>` to lowercase `<` where php's `JSON_HEX_TAG` writes `<`. No route here produces either.
- `run()` neither listens nor waits, the web server owning both, and `config()` reads the worker count back for the record alone: `conf/apache/mpm.conf` and `conf/nginx/fpm-pool.conf` are where each stack is told it.
- The reply headers differ by a few bytes either way. `expose_php` is off so no `X-Powered-By` goes out, and both web servers send a `Server` header where net/http does not.

The error paths match. An unrouted path answers `404 page not found` and a wrong method answers `Method Not Allowed` with an `Allow` header, both with net/http's content type and its `nosniff`, because Slim has no handler for either exception and would answer 500 to both.

## The images

`conf/` is what each image replaces in its server's configuration, and `conf/opcache.ini` and `conf/php.ini` are shared by both. No php extension is installed: the API needs core and json alone, and php 8.5 ships opcache compiled in. A route that reads a database takes one `docker-php-ext-install pdo_sqlite` line and nothing else.

`conf/jit.ini.template` is written at container startup, by the same `envsubst` that fills in the fpm pool, where every other configuration file is copied in at build time. `opcache.jit` cannot be raised once the module has started, so a `php_admin_value` in the pool or the vhost is read too late and php answers `Cannot change opcache.jit setting at run-time`. A file in `conf.d` written before php starts is the only place the setting takes, so `VANILLA_JIT` needs a restart and no rebuild.

`KeepAlive` with `MaxKeepAliveRequests 0` on apache and `keepalive_requests` on both of nginx's ends, so one connection carries a whole segment. The defaults close a connection after 100 requests on apache and 1000 on nginx, and a generator holding one connection then pays a handshake the other sides never pay. `realpath_cache_size` is raised in `conf/php.ini` for the same reason `validate_timestamps` is off: the files do not change while the server is up.

Neither stack writes an access log. One line per request through a pipe is work the phpscript side is not doing and a sweep writes millions of them.

`composer.lock` is committed and the images install from it, so a rebuild resolves nothing and two runs measure the same Slim. The autoloader is built `--classmap-authoritative`.

`fastcgi_keep_conn on` in the nginx configuration keeps the upstream block's connections to fpm. Drop the directive and nginx closes its end after each request while fpm holds its own open, so a reused connection fails with `upstream prematurely closed connection while reading response header` and the 502 lands in the sweep's `non2xx` column. The table above prices both ends of it.

php-fpm expands no environment variable in its pool configuration, so `VANILLA_WORKERS` is substituted into it at startup. Baked in at build time, an image reused without `--build` answers on the worker count of its last build while the sweep's manifest records the variable, and the two disagree silently.
