# Server status middleware

`stdlib/status.ServerStatus` is a lighttpd-style request scoreboard for
phpscript services. It combines HTTP middleware with a PHP runtime observer to
show which requests are active, what the runtime is doing, recently completed
request costs, aggregate traffic, memory pressure, and garbage-collection
activity.

The bundled `phpscript server` enables it automatically.

## Endpoints

| Path | View |
|---|---|
| `/debug/server-status` | Live processes and the 100-request log |
| `/debug/server-status/live` | Live processes |
| `/debug/server-status/log` | Completed request log |
| `/debug/server-status/stats` | Aggregated request statistics |
| `/debug/server-status/detail/{id}` | Completed request spans |

The navigation links open distinct endpoints rather than switching client-side
tabs. The overview combines live processes with the full rolling request log;
the live view contains only requests currently in flight. An entry is
removed as soon as its handler returns, including for keep-alive connections.
The log is newest-first, retains the latest 100 completed requests, and offers
20, 50, and 100 row views. Statistics group that rolling window by hostname,
method and URI, and PHP entrypoint, then display the top 20 groups with their
normalized share of all requests in the window.

## Integration

Install one `ServerStatus` value as both HTTP middleware and runtime observer:

```go
status := status.NewServerStatus()
mux.Use(status.Middleware)

rt.SetContext(r.Context())
rt.Observe(status)
```

With the standard library, wrap the application mux:

```go
status := status.NewServerStatus()
mux := http.NewServeMux()
server := &http.Server{Handler: status.Middleware(mux)}
```

`ServerStatus` also implements `http.Handler`. It can be mounted explicitly at
`ServerStatusPath`, `ServerStatusLivePath`, `ServerStatusLogPath`, and
`ServerStatusStatsPath`, but wrapping is preferred: the middleware then exposes
all status routes and observes every application request.

Each request receives a generated ULID in its `Request-Id` request and response
headers. The ID is also stored in the request context and is available through
`status.RequestID(ctx)`. The runtime observer calls these hooks while the
request is active:

- `UpdateStatus` records runtime state transitions.
- `UpdateFilename` records only the first PHP file loaded as the entrypoint.
- `UpdateIncludedFiles` records the number of additional included files.

Included files do not replace the entrypoint filename. The HTML and text views
display `/` when no additional files were included and otherwise display only
the count.

## Request data

The process and log tables combine the HTTP method and request URI and include
the hostname, PHP entrypoint, included-file count, protocol status, duration,
response bytes, remote address, heap delta, allocated bytes and objects, GC
cycles, and GC pause observed during the request. The log preserves this data
after the active process entry is removed.

Each completed request links to a detail page containing its spans. PHP code can
record a span through the standard library; the context is supplied by the
runtime and the optional strings configure its type and nesting hints:

```php
$span = start_span("getUser", "database");
$span->set_attribute("user_id", 42);
$span->set_message("load user");
$span->end();
```

The detail table reports each span's time from the request start and its
duration up to the next span. Span types default to `internal`.
The HTTP middleware records open and close spans for every request. Resolving a
PHP entrypoint appends another span with that filename.

The scoreboard recognizes the following runtime states:

| State | Meaning |
|---|---|
| `_` | Waiting for a request |
| `s` | PHP runtime starting |
| `R` | Reading or parsing the request/PHP source |
| `P` | Executing PHP |
| `W` | Producing or sending the response |
| `K` | Keepalive |
| `C` | Closing |
| `E` | Runtime error |

At every transition, the middleware adds time spent in the previous state to a
lifetime total. A snapshot also includes elapsed time in the current state of
every active request. The live view renders these totals as a stacked linear
bar with durations and percentages. Totals cover the lifetime of the server;
because concurrent request time is cumulative, summed state time can exceed
wall-clock uptime.

## Representations

Content negotiation applies to every endpoint:

- `Accept: text/json` or `Accept: application/json` returns JSON.
- `Accept: text/plain` returns plain text.
- A `curl/*` user agent receives plain text by default.
- Other clients receive HTML.

The overview and live JSON responses contain the complete `Snapshot`, including server
metadata and all status collections. The log JSON endpoint returns the selected
list of `Request` values; the statistics endpoint returns a
`StatisticsSnapshot`. Plain text is scoped to the selected view.

## Memory impact and pool estimates

The snapshot reports uptime, PID, Go version, GOMAXPROCS, goroutine count, heap,
stack and system memory, the next GC target, GC cycles and pauses, and GC CPU
fraction. The middleware samples process-wide `runtime.MemStats` before and
after each request. From completed samples it calculates average allocation
churn and estimates how many similarly expensive requests could fit before the
next GC target and within the effective memory limit.

The effective limit is the smallest detected value among Go's memory limit,
Linux cgroup limits, and host physical memory. These figures are capacity hints,
not hard pool limits. Go memory statistics are process-wide, so concurrent
requests can include one another's allocations or GC work, and allocated bytes
measure churn rather than retained working set. Use representative traffic and
leave headroom for stacks, caches, database drivers, and native allocations
when choosing concurrency limits.
