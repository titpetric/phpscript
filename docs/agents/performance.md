# Performance sprint

Cut the per-request cost of `testdata/testserver.php` and prove the cut as a measured delta.

Read this when optimising the HTTP path. It runs under the contract in [README.md](README.md); that document is not restated here.

## The target

`testdata/testserver.php` is a PHP program that is the HTTP server rather than something a server runs. `testdata/testserver.go` is the same routes on `net/http.ServeMux`, with the same encoder and the same bodies, and no VM. The two were written to be put through one load generator so the difference could be read off.

Nothing drives either one today: no benchmark, no fixture, no venom suite, no atkins job. The target is a thirty percent gap against the Go twin, and it has no baseline.

That decides the sequencing. The first pull request of this sprint is the harness and the published before numbers, and nothing else. Every later number in the sprint is read against that, not against anything in the tree now.

## Path B, not Path A

There are two unrelated HTTP paths in this repo and `testserver.php` uses the second. Measuring one and attributing the number to the other is the likeliest mistake in this sprint.

|                    | Path A, `phpscript server`                                                           | Path B, `HTTP\Mux` plus `HTTP\Server`              |
|--------------------|--------------------------------------------------------------------------------------|----------------------------------------------------|
| Runtime            | A new `runner.Runtime` per request                                                   | Workers forked once at `listen()`                  |
| Registration       | The whole of `stdlib.Register` per request: 322 functions, 41 classes, 152 constants | Once, at startup                                   |
| Handler resolution | Per request                                                                          | Once, at `$mux->handle()`, through `rt.AsCallable` |
| Parse              | Served from the include cache; `precompile` moves it to startup                      | Once                                               |
| Response           | Buffered whole in a `bytes.Buffer`                                                   | Streamed                                           |
| What runs it       | `cmd/phpscript/server`                                                               | `stdlib/http` plus `runner.Pool`                   |

A Path A number and a Path B number never appear in one table, and every table says which path it is. How to tell which you measured: the command that produced it.

Path B is already structurally right - nothing re-parses, nothing re-registers - so the gap has to come out of the per-call work, not out of adding a cache.

## The harness

`hey` is primary, because it prints a distribution. `wrk` is for the soak.

Concurrency 1 for the delta, which is what the contract's single-threaded rule requires. A second table at concurrency 4 for queueing behaviour, never as the headline.

`testdata/testserver.go` is the floor column. It is `//go:build ignore`, so it runs as `go run testdata/testserver.go`. It is read as the floor, not as a target to reach: a VM is the whole of what the comparison measures.

Routes in, and why:

| Route             | What it prices                                                                      |
|-------------------|-------------------------------------------------------------------------------------|
| `GET /hello`      | A query parameter and a concat. The smallest path that still enters the interpreter |
| `GET /users/{id}` | A path value, a two-key array, and `JSON\Encoder` writing straight to the socket    |
| `POST /echo`      | Body parse on first `form_value`, five-key array, encode                            |
| `GET /{$}`        | Eight echoes. The output path with no data structures                               |

Routes out, and why:

- `GET /slow` is twenty `usleep(100000)` calls by construction. It prices the deadline machinery, not the request path. It gets a correctness check that `connection_aborted` still fires after a change, and no latency row.
- `GET /info` is `phpinfo()`. Nobody serves that shape.

Two asymmetries the harness has to respect. The PHP side answers four requests at once behind a queue of 64, and the queue depth is hardcoded in `config()` where the worker count is not, so no environment variable reaches it. The Go side has no queue at all; its semaphore blocks instead. The comparison is therefore only honest up to `TESTSERVER_WORKERS` concurrent requests.

`TESTSERVER_ADDR` defaults to `127.0.0.1:8099` and takes `127.0.0.1:0` to let the kernel pick. `TESTSERVER_WORKERS` defaults to 4. `TESTSERVER_LIMIT` defaults to 10 seconds.

The first thing that will go wrong is that limit. Running out of time is a fatal: the shutdown callback fires, the server stops listening, and the process disappears part-way through a run that was still measuring. Set `TESTSERVER_LIMIT` past the length of the whole sweep before starting it.

## The known allocation sites

The verified worklist. Each row is a per-request allocation on Path B, with what it is for, because several of them are buying something.

| Site                 | What allocates                                                                                                                                                    | What it is for                                                                                                                                                                                                                         |
|----------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `stdlib/http/mux.go` | An `answerTracker` wrapper per request                                                                                                                            | Recording whether the handler wrote, so a failure after a partial write is not answered with a 500 on top of it. It forwards rather than buffers, deliberately                                                                         |
| `runner/fork.go`     | A `poolRun` and a `make(chan struct{})` per request                                                                                                               | The handshake that lets the submitting goroutine wait for the worker                                                                                                                                                                   |
| `runner/deadline.go` | `EnterRequest` saves three fields, starts `watchEnd(ctx)` and re-arms the deadline                                                                                | Per-request client tracking and the execution limit                                                                                                                                                                                    |
| `runner/runtime.go`  | `resetExecution` sets `outStack` to nil, so `runner/output.go` regrows the backing array next request; plus a `strings.NewReader("")` and several `clear()` calls | Releasing the last request's values now rather than at the next request                                                                                                                                                                |
| `runner/callable.go` | A statics map on every closure `Invoke`                                                                                                                           | Two goroutines running one declaration are two calls, so the statics cannot be shared. `testserver.php` registers bound methods and takes `invokeMethod` instead, which does not pay this - a middleware wrapper puts it straight back |
| `runner.baseEnv`     | The expression environment rebuilt on every `Eval`, one closure per registered function                                                                           | Priced by `BenchmarkScriptEnvFullStdlib` against `BenchmarkScriptEnvMinimal`                                                                                                                                                           |

A row that is buying something is not automatically a target. The question each one answers is whether the thing it buys is needed on every request or only on the requests that use it.

## The benchmarks that price a change

Which of the ninety-four answers which question. The commands are in [collect-go-benchmarks.md](collect-go-benchmarks.md); none appear here.

| Question                                                          | Benchmark                                                                                                                 |
|-------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------|
| What a request turnaround costs, with and without a recorder      | `BenchmarkRequestCycle`, `BenchmarkRequestCycleTraced`, `BenchmarkRequestCycleSvc` in `tests/request_bench_test.go`       |
| What an entrypoint pays while it is parsed per request, on Path A | `BenchmarkServeEntrypoint` in `cmd/phpscript/server`, four subtests over interpreter or flatstack and lazy or precompiled |
| What handler resolution costs across the four host shapes         | `BenchmarkLookupHandler`, subtests go, pooled, pooled_reset, fresh                                                        |
| Interpreter against bytecode on one source                        | The `engine=` subtests in `tests/flatstack/engine_compare_bench_test.go`, read with `benchstat -col /engine`              |
| What the function table costs per `Eval`                          | `BenchmarkScriptEnvFullStdlib` against `BenchmarkScriptEnvMinimal`                                                        |
| What a binding's return shape costs                               | The constructor, call and script families in `tests/bindings_test.go`                                                     |

An engine comparison means nothing unless the program is gated on `flatstack.Supports`. Without the gate a benchmark that falls back records the interpreter's result and its cost under the flatstack name. [../flatstack.md](../flatstack.md) carries the gate idiom and argues the atomic fallback it protects.

## The sweep that is wrong today

`atkins bench` runs every benchmark pinned to one core with `-count 3`. It is the pinned half only, with no exclusion, so its numbers for `BenchmarkLookup`, `BenchmarkLookupHandler` and `BenchmarkFlatstackParallelHostBridge` describe a single goroutine and are discarded.

Splitting it into the two jobs the contract defines, and raising `-count` to 6, is a deliverable of this sprint. It stays out of the default pipeline for the reason the comment above it gives: a sweep costs minutes of pinned CPU and answers a performance question, not a correctness one.

## Techniques

The three tiers are in the contract. What this target invites, by name:

- Reuse the output stack's backing array instead of nil-ing it between requests.
- Arm the deadline lazily, so a request that sets no limit pays nothing for one.
- Pool the answer tracker, whose lifetime ends at the request boundary.
- Hold the resolved-handler table immutable and shared, which it almost is.
- Build the statics map on first static, not on every invoke.

The one that goes to the operator before it is written is `unsafe` over the output buffer. The failure mode is a response body that aliases a buffer the next request is writing, and the suite does not prove its absence.

A technique that lost is recorded with its number. `../flatstack.md` is the model: typed slots were measured and rejected, and the verdict is in the document so nobody spends the week again.

## The pull request

The contract owns the body order. Two things are specific to this sprint.

The summary statement names the path and the route, because a reader cannot otherwise tell what moved:

> Optimized the per-request output path by reusing the output stack's backing array instead of clearing it to nil between requests. This nets a positive change of -X% in latency on `GET /hello` and a delta from X allocs/op to Y allocs/op (-Z%).

The table states the path in its caption and carries the Go floor as a row, so the remaining gap is visible rather than implied:

| Route          | p50 ms | p99 ms | req/s |
|----------------|-------:|-------:|------:|
| `/hello` (go)  |        |        |       |
| `/hello` (was) |        |        |       |
| `/hello` (now) |        |        |       |

The delta is the claim. The absolute number is context.
