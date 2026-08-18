# Telemetry: the status page becomes oida

**2026-08-17.** `stdlib/status`, the hand written request scoreboard, is gone.
Request tracing now runs on [oida](https://github.com/titpetric/oida), bound
into the new `telemetry/` package and served at `/debug/oida`.

## The rule

`telemetry/` is the only package allowed to import `github.com/titpetric/oida`.
Everything else, the interpreter included, instruments through the symbols bound
there. The bindings are type aliases and thin wrappers, so `*telemetry.Span` is
`*oida.Span`: nothing is adapted at runtime, and replacing the provider stays a
one package change instead of a repository wide one.

`telemetry/` carries the two things phpscript needs on top of the library:
the source location a PHP frame publishes into its context, so a span points
back at the line that started it, and `telemetry.Module`, which is the HTTP
middleware, the platform module and the runtime observer in one value.

## What changed for a service

- The config section `status:` is now `telemetry:`, and it is oida's own option
  set: `path`, `service_name`, `enabled`, `ring_buffer_size`, `top_requests`,
  `max_spans_per_trace`, `sample_rate`, `track_memory_use`, `live_stream`,
  `ignore_paths`. The nested `options:` level is gone.
- `/debug/server-status` is now `/debug/oida`, with `/live`, `/traces`,
  `/stats` and `/trace/{id}` below it. HTML, JSON and plain text as before.
- Statistics group by routed pattern where one is meaningful. The PHP file
  server is mounted on a catch-all, so its requests group by path, which is the
  file that ran.

## What changed for a script

`start_span()` returns the telemetry span, so its methods are the Go methods:
`end()`, `set_name()`, `set_source()`, `set_attribute()` and `record_error()`.
`set_message()`, `set_filename()` and `set_line()` are gone; the first is
`set_name()` and the last two are one `set_source()`. Source location is filled
in from the line of PHP that started the span, so a script rarely sets it.

Spans nest instead of pairing open and close markers. A span started while an
include or a PHP call is running is recorded below it, and a database
transaction is one span from `begin()` to `commit()` or `rollback()` rather than
three events to match up. The trace now has the shape of the request.

## Coverage

Instrumentation follows the [oida instrumentation
guide](https://github.com/titpetric/oida/blob/main/docs/guide-instrumentation.md):
stable low-cardinality span names, specifics as attributes, and expected
outcomes left off the failure count.

- Database spans carry `query`, `args`, `transaction_depth`, and `rows` on
  reads. The bound values are recorded, not just their count: a placeholder
  query does not say which row was read.
- `sql.ErrNoRows`, a cancelled request, and a session that is not there are
  control flow, not failures. They no longer fail the trace or the SLA.
- Mail is recorded as external work (`to`, `subject`, `bytes`, `host`), from
  both `mail()` and `new SMTP`. `SMTP.Send` takes a context for it; the
  runtime injects it, so scripts are unchanged.
- Session storage is recorded as cache work (`hit`, `bytes`) around whichever
  backend a script constructed. The session ID is never recorded.
- Migrations are one `migrate` span carrying the file count.
- A failed request logs its trace ID, which is also its `Request-Id`.

## Removed

- `stdlib/status` in full, including its template, stylesheet and ULID.
- `model.Request`, `model.RequestSpan`, `model.Status`, `model.Flag` and
  `model.SpanType`: the recorded data model moved to the library, and the
  interpreter speaks `telemetry.State`, `telemetry.Kind` and `*telemetry.Span`.

Upstream, `oida.Options.SampleRate` became a percentage: `100` traces
everything and is the default, `0` traces nothing.
