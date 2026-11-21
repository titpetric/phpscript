# Performance sprint

Cut the per-request cost of the HTTP path and prove the cut as a measured delta.

Read this when optimising the HTTP path. [README.md](README.md) holds the contract and this document does not restate it.

## The target

One API written five ways, one directory each in `testdata/`, driven by one load generator. The difference then reads off directly. Every side shares the host's network namespace, answers on the same worker count, and runs at `nice -n -20`. Only the stack differs.

| Directory                        | What it runs                                           | Model  |
|----------------------------------|--------------------------------------------------------|--------|
| `testdata/goserver`              | the same routes on `net/http.ServeMux`, no VM          | floor  |
| `testdata/phpscript-http-bridge` | a PHP program that is itself the HTTP server           | Path B |
| `testdata/phpscript-service`     | route annotations under `phpscript server`             | Path A |
| `testdata/php-vanilla`           | Slim on apache with mod_php, and on nginx with php-fpm | php    |

`testdata/phpscript-service` is the shape a phpscript application is actually written in: one file per endpoint, `// @route METHOD /path` above it as the whole routing table, `$_GET` and `$_POST` for the request, `$_REQUEST` for a path value, `echo` for the body.

The table shows all five. Each route ran six segments of 3s at concurrency 1, four workers a side, four cores. Every row came from one sweep. Path A and Path B share one table, since the subject is the spread between models. A sprint's before and after tables are per runtime, as the sweep writes them.

| Runtime           | `/hello` p50 us | `/users` p50 us | `/echo` p50 us | `/hello` req/s |
|-------------------|-----------------|-----------------|----------------|----------------|
| `goserver`        | 35.0            | 37.0            | 44.0           | 26719          |
| `bridge`, Path B  | 47.0            | 50.0            | 55.0           | 19441          |
| `service`, Path A | 70.0            | 75.0            | 83.0           | 13089          |
| `vanilla-nginx`   | 373.0           | 378.0           | 381.0          | 2525           |
| `vanilla-apache`  | 472.0           | 474.0           | 489.0          | 2017           |

Compare rows inside one table, never across tables. `scripts/bench-http.sh` and `scripts/benchmark.php` answer a few percent apart on the same configuration. A server's lifetime differs in what else it served. A figure copied between tables comes from another run.

Two things drive them. Both arrived with this sprint's first pull request. `scripts/bench-http.sh` puts both sides through a single load generator. It reports a latency distribution, with a request rate per route. `BenchmarkTestServerRoute` in `tests/testserver_bench_test.go` answers one request per iteration through the router the file builds, reporting `allocs/op`. No fixture or venom suite covers either.

That sequencing holds for everything later: publish the baseline before the first change. Read later numbers against the baseline, never against the tree.

## Path B, not Path A

This repository holds two unrelated HTTP paths, one directory each. `testdata/phpscript-service` is Path A. `testdata/phpscript-http-bridge` is Path B. Measuring one path and attributing the number to the other is the likeliest mistake here.

|                    | Path A, `phpscript server`                                          | Path B, `HTTP\Mux` plus `HTTP\Server`              |
|--------------------|---------------------------------------------------------------------|----------------------------------------------------|
| Runtime            | A `runner.SessionPool` per site, reset between requests             | Workers forked once at `listen()`                  |
| Registration       | `stdlib.Register` once per pooled runtime, kept across its requests | Once, at startup                                   |
| Handler resolution | Per request                                                         | Once, at `$mux->handle()`, through `rt.AsCallable` |
| Parse              | Served from the include cache; `precompile` moves it to startup     | Once                                               |
| Response           | Buffered whole in a `bytes.Buffer`                                  | Streamed                                           |
| What runs it       | `cmd/phpscript/server`                                              | `stdlib/ps/http` plus `runner.Pool`                |

Path A and Path B numbers never share one table. Every table names its path. The command that produced a number says which path it measured.

Path B is already structurally right. It has no re-parse or re-register. The gap must come out of per-call work, not out of a cache.

## The harness

`scripts/bench-http.sh` is the sweep. `wrk --latency` is the generator it drives. `hey` fails on output alone: it prints every latency as four decimal places of a second. That is coarser than any handler here answers in. Each percentile it reports comes back as the same number. wrk prints no 95th percentile, reporting 50, 75, 90 and 99. The sweep collects the 90th instead.

A latency column is never finer than the generator. None prices a per-request allocation. `BenchmarkTestServerRoute` is the other half of the harness. It drives the same router through `httptest` with the socket left out. `ns/op`, `B/op` and `allocs/op` come off one request path at a resolution the sweep lacks. The sweep confirms that the socket and the wire undid nothing.

Concurrency 1 measures the delta, as the contract's single-threaded rule requires. A second table at concurrency 4 covers queueing behaviour but never becomes the headline.

A segment covers one wrk run. Each route gets six. The contract's drift guard needs two ends to compare. One long run reports one distribution, and nothing records busy periods during it.

`testdata/goserver` is the floor column. The file holds `//go:build ignore` and runs as `go run testdata/goserver/main.go`. Read it as the floor, not as a target: the comparison measures the VM alone.

`BENCH_TARGETS` selects the sides. The default pair needs no docker:

```sh
BENCH_TARGETS="goserver bridge service vanilla-apache vanilla-nginx" scripts/bench-http.sh after
```

One table per side. Two sides use different execution models. `bridge` and `service` are the pair. The table header names the model. Each number states its meaning. The sweep starts and stops every side, including the compose stacks.

`testdata/php-vanilla` is the ceiling, served two ways from one copy of `public/index.php`. The web server is a column of the table. No part of the stack stays fixed across it. Its worker count is `VANILLA_WORKERS`, set to `TESTSERVER_WORKERS` by default. Every side answers the same number of requests at once. opcache runs with `validate_timestamps` off, Slim caches its routes and the autoloader is `--classmap-authoritative`. An application without those measures the compiler. The JIT is off, php's default. `VANILLA_JIT` sets the mode. The README prices all three of its modes and the buffer each fills. [../../testdata/php-vanilla/README.md](../../testdata/php-vanilla/README.md) prices each stage of its bootstrap and lists where it differs from the PHP the phpscript side runs.

Memory is the last section a sweep writes, taken with the last request answered and nothing torn down. The two measurements are not interchangeable. The note column distinguishes the two. A host process reports its RSS, phpscript and the twin answering every request on one process, with threads as workers. The container reports a cgroup over apache's parent and all forked children. Neither number is a per-request figure. `B/op` from `BenchmarkTestServerRoute` prices a request.

Routes in:

| Route             | What it prices                                                                      |
|-------------------|-------------------------------------------------------------------------------------|
| `GET /hello`      | A query parameter and a concat. The smallest path that still enters the interpreter |
| `GET /users/{id}` | A path value, a two-key array, and `JSON\Encoder` writing straight to the socket    |
| `POST /echo`      | Body parse on first `form_value`, five-key array, encode                            |
| `GET /{$}`        | Eight echoes. The output path with no data structures                               |

Routes out:

- `GET /slow` is twenty `usleep(100000)` calls by construction. It prices the deadline machinery. It gets a correctness check that `connection_aborted` still fires after a change. It gets no latency row.
- `GET /info` is `phpinfo()`. Nobody serves that shape.

The harness must respect two asymmetries. The PHP side answers four requests at once behind a queue of 64. `config()` hardcodes the queue depth. The worker count comes from elsewhere, without an environment variable override. The Go side has no queue. Its semaphore blocks. The comparison is honest only up to `TESTSERVER_WORKERS` concurrent requests.

`TESTSERVER_ADDR` defaults to `127.0.0.1:8099` and takes `127.0.0.1:0` to let the kernel pick. `TESTSERVER_WORKERS` defaults to 4. `TESTSERVER_LIMIT` is a duration and defaults to ten seconds.

The first thing that will go wrong is that limit. Running out of time is a fatal: the shutdown callback fires, the server stops listening, and the process disappears part-way through a run still measuring. Set `TESTSERVER_LIMIT` past the length of the whole sweep before starting it.

## The known allocation sites

The verified worklist, in the order a memory profile of `BenchmarkTestServerRoute` ranks it. Each row is a per-request allocation on Path B and states its gain. Not every row is waste.

Three rows are off the worklist and stay here, to stop re-proposals. The `answerTracker` comes from a free list (`trackers` in `stdlib/ps/http/mux.go`). `releaseOutput` keeps the output stack's backing array between requests. The per-statement deadline check reads the precomputed `deadlineArmed` field.

| Site                         | What allocates                                                                                                                                                         | What it buys                                                                                                                                                                                                                                     |
|------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `runner/expr/direct.go`      | `baseCall` boxes every call's arguments into a `[]any` per invocation                                                                                                  | The variadic signature re-entrant calls and registered functions require. [../allocation-performance.md](../allocation-performance.md) names it as the closure engine's one remaining call allocation                                            |
| `runner/helpers.go`          | `reflect.Value.Call` per Go method a handler reaches: the receiver through `reflect.methodReceiver`, the argument frame through `reflect.unsafe_New`, the result slice | Reaching a host type's methods at all. `runner` caches resolution per receiver type and spelled name already; reflect's own per-call allocation is what remains, and it makes up the largest share of a handler that touches `$w` and `$r`       |
| `runner/runner.go`           | `execOne` writes every echo through `io.WriteString`, which clones the string to `[]byte` whenever the writer underneath has no `WriteString`                          | The `answerTracker` forwards `Write` only and it hides the `io.StringWriter` that `net/http`'s response writer implements and every echo pays one clone. Forwarding `WriteString` through the tracker removes it; the harness trap below applies |
| `runner/context.go`          | `Context.Register` seeds the superglobals per request: the `mapmap` layers, the scratch arrays, the scope entries                                                      | `$_GET`, `$_SERVER` and the rest, decoded lazily but registered eagerly. It dominates `BenchmarkRequestCycle`'s profile where the testserver path, which decodes none of them, pays only the registration                                        |
| `stdlib/ps/core/json.go`     | `JSON\Encoder::encode` builds a `json.Encoder` and a `jsonObject` wrapper per array it walks                                                                           | Streaming a PHP array to the socket with `encoding/json` semantics. Both wrappers have request lifetime                                                                                                                                          |
| `runner/pool.go`             | A `poolRun` and a `make(chan struct{})` per request                                                                                                                    | The handshake that lets the submitting goroutine wait for the worker. [../allocation-performance.md](../allocation-performance.md) refuses recycling and records the reasoning                                                                   |
| `runner/runtime_deadline.go` | `EnterRequest` allocates the restore closure over the three fields it saves                                                                                            | Per-request client tracking, undone when the request leaves                                                                                                                                                                                      |
| `runner/callable.go`         | A statics map on every closure `Invoke`                                                                                                                                | Two goroutines running one declaration are two calls and they cannot share one statics map. `testserver.php` registers bound methods and takes `invokeMethod` instead, which does not pay this - a middleware wrapper puts it straight back      |

A row that buys something is not automatically a target. Each row answers whether every request needs that gain or only the requests that use the gain. `net/textproto.MIMEHeader.Set`, `net/url.parseQuery` and `net/http.copyValues` also show per request and belong to the standard library. A handler that sets a header and reads a query also pays them in Go. The twin's profile proves it.

## Profiles

A profile names the site a benchmark only prices. Collect both kinds from the benchmark that owns the question, at least `BenchmarkTestServerRoute`, with `-cpuprofile cpu-<label>.pprof -memprofile mem-<label>.pprof` added to the contract's pinned command. The artifact names are already gitignored.

Read four tables per benchmark. `go tool pprof -top` flat shows where the cost sits. `-top -cum` shows the chain that reaches it. Read the memory profile twice: `-sample_index=alloc_objects`, then `alloc_space`. A small struct allocated per request ranks high in objects and low in bytes. `-peek <symbol>` answers who calls an ambiguous entry such as `io.WriteString`.

Two traps the first collection walked into:

- The harness is inside the profile. `newTestServerHandler` parses the file and runs `stdlib.Register` once per benchmark invocation. Registration frames in a Path B profile belong to the setup. The contract's `-benchtime 1s` keeps that share small. On a Path A profile, the registration frames are the finding.
- `benchServerWriter` discards without a `WriteString` method, the shape the `answerTracker` forwards today. The benchmark then prices the output path's clone truthfully. A change that teaches the tracker `WriteString` must teach the benchmark writer the same method in the same commit. Otherwise the gain never reaches the number.

## The benchmarks that price a change

Which of the ninety-four answers which question. The commands are in [collect-go-benchmarks.md](collect-go-benchmarks.md). None appear here.

| Question                                                          | Benchmark                                                                                                                 |
|-------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------|
| What one request on Path B costs, per route                       | `BenchmarkTestServerRoute` in `tests/testserver_bench_test.go`, subtests hello, users, echo, index                        |
| What a request turnaround costs, with and without a recorder      | `BenchmarkRequestCycle`, `BenchmarkRequestCycleTraced`, `BenchmarkRequestCycleSvc` in `tests/request_bench_test.go`       |
| What an entrypoint pays for its parse on every request, on Path A | `BenchmarkServeEntrypoint` in `cmd/phpscript/server`, four subtests over interpreter or flatstack and lazy or precompiled |
| What handler resolution costs across the four host shapes         | `BenchmarkLookupHandler`, subtests go, pooled, pooled_reset, fresh                                                        |
| Interpreter against bytecode on one source                        | The `engine=` subtests in `tests/flatstack/engine_compare_bench_test.go`, read with `benchstat -col /engine`              |
| What the function table costs per `Eval`                          | `BenchmarkScriptEnvFullStdlib` against `BenchmarkScriptEnvMinimal`                                                        |
| What a binding's return shape costs                               | The constructor, call and script families in `tests/bindings_test.go`                                                     |

An engine comparison means nothing unless `flatstack.Supports` gates the program. Without the gate, a benchmark that falls back records the interpreter's result as the flatstack name. [../flatstack.md](../flatstack.md) holds the gate idiom and argues the atomic fallback it protects.

## The two benchmark jobs

`atkins bench` used to run every benchmark pinned to one core with `-count 3`. That is the pinned half alone, with nothing excluded. Its numbers for `BenchmarkLookup`, `BenchmarkLookupHandler` and `BenchmarkFlatstackParallelHostBridge` described a single goroutine.

The contract now defines two jobs. `atkins bench:pinned` runs at `-count 6`. `atkins bench:parallel` runs at `-count 10`. Each writes `bench-go-$SIDE.txt` and takes the measure lock. `atkins bench` runs the pair and summarises the file with benchstat. The default pipeline runs neither job. The comment above them states the reason. A sweep costs minutes of pinned CPU and settles a performance question. The load sweep has no atkins job. `scripts/bench-http.sh <side>` is the whole interface. A job wrapping it would only hide the side.

## Techniques

The contract names three tiers. This target invites, by name:

- Forward `io.StringWriter` through the `answerTracker`. An echo stops cloning its string. The harness trap above names the second half of that change.
- A fixed-arity fast path or a pooled slice for `baseCall`'s argument boxing, sized by the arity the compiler already knows.
- A typed bridge in front of `reflect.Value.Call` for the handful of host types every handler touches: the response writer, the request, `url.Values`.
- Register superglobals as lazily as they decode. A handler that reads none pays none of the `mapmap` layers.
- Hold the resolved-handler table immutable and shared, which it almost is.
- Build the statics map on first static, not on every invoke.

`unsafe` over the output buffer needs the operator's consent first. The failure mode: a response body aliases a buffer the next request writes. The suite does not prove its absence.

Record a technique that lost with its number. `../flatstack.md` is the model: it refuses typed slots on a measurement, and the document records the verdict so nobody spends the week again.

## The pull request

The contract owns the body order. Two things are specific to this sprint.

The summary statement names the path and the route. Otherwise a reader cannot tell what moved:

> Optimized the per-request output path by keeping the output stack's backing array between requests, in place of clearing it to nil. This nets a positive change of -X% in latency on `GET /hello`. It moves from X allocs/op to Y allocs/op (-Z%).

The table states the path in its caption and keeps the Go floor as a row. The remaining gap is a subtraction the reader can do. Use microseconds, where the contract uses milliseconds. Every route here answers in a fraction of a second. Its millisecond column to one decimal is a column of zeroes.

| Route             | p50 us | p99 us | req/s |
|-------------------|-------:|-------:|------:|
| `/hello` (go)     |        |        |       |
| `/hello` (before) |        |        |       |
| `/hello` (now)    |        |        |       |

The allocation table is the second one, in the shape [../allocation-performance.md](../allocation-performance.md) owns. It holds no Go floor: `testdata/goserver/main.go` is `//go:build ignore` and nothing can import its handlers and the floor appears in the latency table only.

The delta is the claim. The absolute number is context.
