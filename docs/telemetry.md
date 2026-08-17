# Telemetry

`phpscript server` records what it does: every request becomes a trace, and the
work inside it becomes spans. Includes, PHP calls, templates, database queries
and anything a script measures itself land in the same timeline, served by a
front end at `/debug/oida`.

The recorder is [oida](https://github.com/titpetric/oida). phpscript binds it
into `telemetry/`, which is the only package allowed to import it: every other
package instruments through `telemetry.Start`, `telemetry.StartSpan` and the
types bound there, so the provider is named in one place.

The bundled server enables telemetry by default. Set `telemetry.enabled` to
`false` in a file passed with `-f` to turn it off; retention, sampling, the
mount path and memory sampling are configurable in the same section. See
[Configuration](./configuration.md#http-modules).

## Views

| Path                     | View                                                              |
|--------------------------|-------------------------------------------------------------------|
| `/debug/oida`            | The domains this process serves and how much traffic each carries |
| `/debug/oida/live`       | Traces in flight and completed traces as they are recorded        |
| `/debug/oida/traces`     | Retained traces, filterable by host, span kind and status         |
| `/debug/oida/stats`      | Rolling statistics of the retained traces                         |
| `/debug/oida/trace/{id}` | One trace: its timeline, its spans and what it cost               |

Picking a domain on the landing page narrows every other view to it. The live
view streams over server sent events when `live_stream` is enabled and falls
back to a refresh timer when the browser cannot stream.

Content negotiation applies to every view: `Accept: application/json` returns
JSON, `Accept: text/plain` and a `curl/*` user agent return plain text, and
everything else gets HTML.

## Integration

One `telemetry.Module` is the middleware, the platform module and the runtime
observer:

```go
recorder, err := telemetry.NewModule(config.Telemetry)
if err != nil {
	return err
}
svc.Use(recorder.Middleware)
svc.Register(recorder)

rt.SetContext(r.Context())
rt.Observe(recorder)
```

The middleware records the request, the platform module mounts the front end,
and the observer forwards what the interpreter reports onto the running trace:

- `UpdateStatus` moves the trace through the scoreboard states.
- `UpdateFilename` records the PHP entrypoint the request resolved to.
- `UpdateIncludedFiles` records how many files it included beyond that.
- `Trace` records a span for interpreter work: an include, a call, a template.

Work that does not arrive over the network, such as a `@startup` file, is
recorded through `TrackLifecycle` as a background trace.

Each request receives a ULID in its `Request-Id` request and response headers.
It is the trace identifier, so a log line carrying it links straight to
`/debug/oida/trace/{id}`. `telemetry.TraceID(ctx)` returns it.

Statistics group by routed pattern where one is meaningful, so `/hello/Ada` and
`/hello/Grace` aggregate into `GET /hello/{name}`. The PHP file server is
mounted on a catch-all, and requests it serves group by path instead, which is
the PHP file that ran.

## Spans from PHP

A script measures its own work with `start_span`. The optional second argument
is the span kind:

```php
$span = start_span("getUser", "database");
$span->set_attribute("user_id", 42);
$span->end();
```

The span is the Go span, so its methods are `end()`, `set_name()`,
`set_source()`, `set_attribute()` and `record_error()`. Source location is
filled in from the line of PHP that started the span, so `set_source()` is only
needed to point somewhere else.

Kinds are `internal` (the default), `http`, `database`, `external`, `template`,
`cache` and `queue`. The set is open: an unrecognized string is a valid kind and
gets its own color in the timeline.

Spans nest. A span started while an include or a PHP call is running is recorded
below it, which is what gives the trace the shape of the request rather than a
flat list.

Instrumentation is nil safe. A script that calls `start_span` in a CLI run, in a
process with telemetry disabled, or in a request the sampler rejected, gets a
span whose methods do nothing.

## What the bindings record

Span names are stable and low cardinality, because a name is an identity: the
specifics go in attributes, which is what the detail view expands under each
row.

| Span                                      | Kind                              | Attributes                                                                           |
|-------------------------------------------|-----------------------------------|--------------------------------------------------------------------------------------|
| `GET /path` (the request)                 | `http`                            | `filename`, `included_files`                                                         |
| `include file.php`                        | `internal`, `template` for `.tpl` | —                                                                                    |
| `new Class`, `$var.method`                | `internal`                        | —                                                                                    |
| `php error`                               | `internal`                        | the message is the recorded error                                                    |
| `$db.Get`, `$db.Query`, …                 | `database`                        | `query`, `query_type`, `query_comment`, `args`, `transaction_depth`, `rows` on reads |
| `$db.Begin` → `$db.Commit`/`$db.Rollback` | `database`                        | one span covering the transaction                                                    |
| `migrate`                                 | `database`                        | `migrations`                                                                         |
| `session load`/`save`/`delete`/`prune`    | `cache`                           | `hit`, `bytes`                                                                       |
| `mail`                                    | `external`                        | `to`, `subject`, `bytes`, `host`                                                     |

`query_type` is the keyword the statement starts with, lowercased, and
`query_comment` is the text of a `/* */` comment in front of it: a query written
as `/* userGet */ SELECT * FROM user ...` records `select` and `userGet`. Both
group a trace by the query behind it, which the statement text alone does not.
The comment stays in the statement sent to the server, so `SHOW PROCESSLIST` and
the slow query log show the same tag as the trace does.

`args` carries the values bound to the statement, positional ones as a list and
named ones as the map they came from. A placeholder query says nothing about
which row was read, so the values are the point; they are as sensitive as the
columns they filter on, which is a reason to set `Options.Authorize` before
exposing the front end. Message bodies and session IDs are the two things never
recorded: only their size, and nothing at all, respectively.

Expected outcomes are not failures. A `get()` that found no row, a session that
is not there, and a request the client cancelled are recorded as a miss or an
empty result, not as an error, because a recorded error fails the trace and the
SLA computed from it. The same rule decides the scoreboard: a page ending in
`exit()` ran to completion, and only `exit(1)` and above is an error.

A trace ID is the cheapest correlation there is, so the server writes it into
the log line of a failed request. It is the same value as the `Request-Id`
header and the last path segment of the trace detail page.

## Scoreboard states

The states follow the convention used by servers such as lighttpd:

| State | Meaning                                   |
|-------|-------------------------------------------|
| `_`   | Waiting for a request                     |
| `s`   | PHP runtime starting                      |
| `R`   | Reading or parsing the request/PHP source |
| `P`   | Executing PHP                             |
| `W`   | Producing or sending the response         |
| `K`   | Keepalive                                 |
| `C`   | Closing                                   |
| `E`   | Runtime error                             |

At every transition the time spent in the previous state is added to a lifetime
total, rendered in the live view as a stacked bar with durations and shares.
Totals cover the lifetime of the process; because concurrent request time is
cumulative, summed state time can exceed wall-clock uptime.

## Memory and pool estimates

With `track_memory_use` enabled, `runtime.MemStats` is read around each trace
and the deltas are recorded on it: heap delta, allocated bytes, allocations, GC
cycles and GC pause. The snapshot adds process figures - uptime, PID, Go
version, GOMAXPROCS, goroutines, heap, stack and system memory, the next GC
target and GC CPU fraction - and estimates how many similarly expensive requests
fit before the next GC target and within the effective memory limit.

Go memory statistics are process-wide, so concurrent requests include one
another's allocations and GC work, and allocated bytes measure churn rather than
retained working set. These are capacity hints, not pool limits: measure with
representative traffic and leave headroom for stacks, caches, database drivers
and native allocations.

## Access control

The front end is not authenticated by default. Set `Options.Authorize` before
exposing it on a public listener; it gates every route the front end serves,
including its assets.
