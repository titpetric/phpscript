# Performance sprint

Cut the per-request cost of `testdata/testserver.php` and prove the cut as a measured delta.

Read this when optimising the HTTP path. It runs under the contract in [README.md](README.md); that document is not restated here.

## The target

`testdata/testserver.php` is a PHP program that is itself the HTTP server. `testdata/testserver.go` is the same routes on `net/http.ServeMux`, with the same encoder and the same bodies, and no VM. The two were written to be put through one load generator so the difference could be read off.

Two things drive them, and both arrived with this sprint's first pull request. `scripts/bench-http.sh` puts both sides through one load generator and reports a latency distribution and a request rate per route; `BenchmarkTestServerRoute` in `tests/testserver_bench_test.go` answers one request per iteration through the router the file builds, and reports `allocs/op`. There is still no fixture and no venom suite over either.

The sequencing those two arrived under holds for everything after them: the baseline is published before the first change, and every later number is read against that baseline, never against whatever is in the tree.

## Path B, not Path A

There are two unrelated HTTP paths in this repository and `testserver.php` uses the second. Measuring one and attributing the number to the other is the likeliest mistake in this sprint.

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

`scripts/bench-http.sh` is the sweep and `wrk --latency` is the generator inside it. `hey` was named here first and lost the job on its own output: it prints every latency as four decimal places of a second, which is coarser than any handler here answers in, so every percentile it reported came back as the same number. wrk prints no 95th percentile, reporting 50, 75, 90 and 99, so the 90th is collected in its place.

A latency column end to end is never finer than the generator, and a per-request allocation cannot be read off one at all. `BenchmarkTestServerRoute` is the other half of the harness for that reason: it drives the same router through `httptest` with the socket left out, so `ns/op`, `B/op` and `allocs/op` come off the same request path at a resolution the sweep does not have. The sweep is what says the socket and the wire did not undo it.

Concurrency 1 for the delta, as the contract's single-threaded rule requires. A second table at concurrency 4 for queueing behaviour, never as the headline.

A segment is one wrk run and a route gets several of them, because the contract's drift guard needs two ends to compare: one long run reports one distribution with no way to tell when inside it the box was busy.

`testdata/testserver.go` is the floor column. It is `//go:build ignore`, so it runs as `go run testdata/testserver.go`. It is read as the floor, not as a target to reach: a VM is the whole of what the comparison measures.

Routes in, and why:

| Route             | What it prices                                                                      |
|-------------------|-------------------------------------------------------------------------------------|
| `GET /hello`      | A query parameter and a concat. The smallest path that still enters the interpreter |
| `GET /users/{id}` | A path value, a two-key array, and `JSON\Encoder` writing straight to the socket    |
| `POST /echo`      | Body parse on first `form_value`, five-key array, encode                            |
| `GET /{$}`        | Eight echoes. The output path with no data structures                               |

Routes out, and why:

- `GET /slow` is twenty `usleep(100000)` calls by construction. It prices the deadline machinery. It gets a correctness check that `connection_aborted` still fires after a change, and no latency row.
- `GET /info` is `phpinfo()`. Nobody serves that shape.

Two asymmetries the harness has to respect. The PHP side answers four requests at once behind a queue of 64, and the queue depth is hardcoded in `config()` where the worker count is not, so no environment variable reaches it. The Go side has no queue at all; its semaphore blocks instead. The comparison is therefore only honest up to `TESTSERVER_WORKERS` concurrent requests.

`TESTSERVER_ADDR` defaults to `127.0.0.1:8099` and takes `127.0.0.1:0` to let the kernel pick. `TESTSERVER_WORKERS` defaults to 4. `TESTSERVER_LIMIT` is a duration and defaults to ten seconds.

The first thing that will go wrong is that limit. Running out of time is a fatal: the shutdown callback fires, the server stops listening, and the process disappears part-way through a run that was still measuring. Set `TESTSERVER_LIMIT` past the length of the whole sweep before starting it.

## The known allocation sites

The verified worklist. Each row is a per-request allocation on Path B, with what it is for, because several of them are buying something.

| Site                 | What allocates                                                                                                                                                    | What it is for                                                                                                                                                                                                                         |
|----------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `stdlib/http/mux.go` | An `answerTracker` wrapper per request                                                                                                                            | Recording whether the handler wrote, so a failure after a partial write is not answered with a 500 on top of it. It forwards and does not buffer                                                                                       |
| `runner/fork.go`     | A `poolRun` and a `make(chan struct{})` per request                                                                                                               | The handshake that lets the submitting goroutine wait for the worker                                                                                                                                                                   |
| `runner/deadline.go` | `EnterRequest` saves three fields, starts `watchEnd(ctx)` and re-arms the deadline                                                                                | Per-request client tracking and the execution limit                                                                                                                                                                                    |
| `runner/runtime.go`  | `resetExecution` sets `outStack` to nil, so `runner/output.go` regrows the backing array next request; plus a `strings.NewReader("")` and several `clear()` calls | Releasing the last request's values now rather than at the next request                                                                                                                                                                |
| `runner/callable.go` | A statics map on every closure `Invoke`                                                                                                                           | Two goroutines running one declaration are two calls, so the statics cannot be shared. `testserver.php` registers bound methods and takes `invokeMethod` instead, which does not pay this - a middleware wrapper puts it straight back |
| `runner/helpers.go`  | `reflect.Value.Call` per Go method a handler reaches: the receiver's method value, the argument frame, the result slice                                           | Reaching a host type's methods at all. Resolution is cached per receiver type and spelled name already; what is left is reflect's own per-call allocation, and it is the largest share of a handler that touches `$w` and `$r`         |

A row that is buying something is not automatically a target. The question each one answers is whether the thing it buys is needed on every request or only on the requests that use it.

## The benchmarks that price a change

Which of the ninety-four answers which question. The commands are in [collect-go-benchmarks.md](collect-go-benchmarks.md); none appear here.

| Question                                                          | Benchmark                                                                                                                 |
|-------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------|
| What one request on Path B costs, per route                       | `BenchmarkTestServerRoute` in `tests/testserver_bench_test.go`, subtests hello, users, echo, index                        |
| What a request turnaround costs, with and without a recorder      | `BenchmarkRequestCycle`, `BenchmarkRequestCycleTraced`, `BenchmarkRequestCycleSvc` in `tests/request_bench_test.go`       |
| What an entrypoint pays while it is parsed per request, on Path A | `BenchmarkServeEntrypoint` in `cmd/phpscript/server`, four subtests over interpreter or flatstack and lazy or precompiled |
| What handler resolution costs across the four host shapes         | `BenchmarkLookupHandler`, subtests go, pooled, pooled_reset, fresh                                                        |
| Interpreter against bytecode on one source                        | The `engine=` subtests in `tests/flatstack/engine_compare_bench_test.go`, read with `benchstat -col /engine`              |
| What the function table costs per `Eval`                          | `BenchmarkScriptEnvFullStdlib` against `BenchmarkScriptEnvMinimal`                                                        |
| What a binding's return shape costs                               | The constructor, call and script families in `tests/bindings_test.go`                                                     |

An engine comparison means nothing unless the program is gated on `flatstack.Supports`. Without the gate a benchmark that falls back records the interpreter's result and its cost under the flatstack name. [../flatstack.md](../flatstack.md) carries the gate idiom and argues the atomic fallback it protects.

## The two benchmark jobs

`atkins bench` used to run every benchmark pinned to one core with `-count 3`. That is the pinned half only, with no exclusion, so its numbers for `BenchmarkLookup`, `BenchmarkLookupHandler` and `BenchmarkFlatstackParallelHostBridge` described a single goroutine.

It is now `atkins bench:pinned` and `atkins bench:parallel`, the two jobs the contract defines, at `-count 6` and `-count 10`, both writing `bench-go-$SIDE.txt` and both taking the measure lock; `atkins bench` runs the pair and summarises the file with benchstat. Neither is in the default pipeline, for the reason the comment above them states: a sweep costs minutes of pinned CPU and answers a performance question. The load sweep has no atkins job: `scripts/bench-http.sh <side>` is the whole interface and a job wrapping it would only hide the side.

## Techniques

The three tiers are in the contract. What this target invites, by name:

- Keep the output stack's backing array between requests, in place of nil-ing it.
- Arm the deadline lazily, so a request that sets no limit pays nothing for one.
- Pool the `answerTracker`, whose lifetime ends at the request boundary.
- Hold the resolved-handler table immutable and shared, which it almost is.
- Build the statics map on first static, not on every invoke.

The one that goes to the operator before it is written is `unsafe` over the output buffer. The failure mode is a response body that aliases a buffer the next request is writing, and the suite does not prove its absence.

A technique that lost is recorded with its number. `../flatstack.md` is the model: typed slots were measured and rejected, and the verdict is in the document so nobody spends the week again.

## The pull request

The contract owns the body order. Two things are specific to this sprint.

The summary statement names the path and the route, because a reader cannot otherwise tell what moved:

> Optimized the per-request output path by keeping the output stack's backing array between requests, in place of clearing it to nil. This nets a positive change of -X% in latency on `GET /hello` and a delta from X allocs/op to Y allocs/op (-Z%).

The table states the path in its caption and carries the Go floor as a row, so the remaining gap is a subtraction the reader can do. Microseconds, not the contract's milliseconds: every route here answers in a fraction of one, and a millisecond column to one decimal is a column of zeroes.

| Route          | p50 us | p99 us | req/s |
|----------------|-------:|-------:|------:|
| `/hello` (go)  |        |        |       |
| `/hello` (was) |        |        |       |
| `/hello` (now) |        |        |       |

The allocation table is the second one, in the shape [../allocation-performance.md](../allocation-performance.md) owns, and it carries no Go floor: `testdata/testserver.go` is `//go:build ignore` and its handlers cannot be imported, so the floor lives in the latency table only.

The delta is the claim. The absolute number is context.
