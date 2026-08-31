# Migrating phpscript to oida v0.3.0

Written for later execution. Nothing here is applied.

## Why

phpscript pins `github.com/titpetric/oida v0.2.0` and binds it in one place,
`telemetry/alias.go`. The host, `titpetric/platform`, pins the same version and
names oida directly in `options.go`, `platform.go` and `telemetry_module.go`.

v0.3.0 makes the root package the whole public API: the sub-packages carry no
compatibility promise, the process wide tracer is gone, and the symbols no
integration called were removed or unexported. Both repositories have to move
together, because platform builds the tracer that phpscript records into.

## What phpscript actually uses

Its own call sites reach 35 symbols through `telemetry.`, and only five of them
are affected: `TracingMiddleware`, `TraceHost`, `BackgroundHost`, `Mount` and
`NewOptions`. The rest of the work is deleting bindings nothing calls.

`telemetry.Tracer` stays. `Module.tracer` is typed with it, `NewModule` takes
it, `Module.Tracer()` returns it, and `cmd/phpscript/server` and the tests call
that accessor.

## Changes in oida v0.3.0

Gone from the root package:

- `Configure`, `Default`, `Resolve`, `MustResolve`. `New` is the only
  constructor, and there is no process wide tracer to fall back on.
- `TracingMiddleware(opts)`. The middleware is `(*Tracer).Middleware`, which
  has the same `func(http.Handler) http.Handler` shape.
- `NewStorageMemory`, `NewStorageDisk`, `StorageMemory`, `StorageDisk`. The
  drivers live in `storage/` with unexported types, and `Configure`-style
  selection happens through `OIDA_STORAGE_DRIVER` and the `OIDA_STORAGE_*`
  variables.
- `NewRateSampler` and `SamplerFunc`. `Options.SampleRate` covers the rate, and
  `Options.Sampler` takes a type with a `Sample` method.
- `TraceHost`, `ValidID`, `IsBytes`, `BackgroundHost`, `(*Tracer).Storage`.
- The view model aliases: `Recorder`, `HTTPInfo`, `Memory`, `MemoryUse`,
  `PoolEstimate`, `StateDuration`, `Statistic`, `HostStat`, `Stats`, `Spans`.
  `Snapshot` stays, since `(*Tracer).Snapshot` returns it.
- `Options.Tracer`. Entry points take the tracer itself.

Changed:

- `NewOptions(serviceName string) Options` takes the service name.
- `Mount(r Router, t *Tracer) error` takes the tracer, and `Router` is
  `Handle(pattern string, h http.Handler)`, the method chi and
  `*http.ServeMux` share. A router whose `Handle` returns a value, such as
  gorilla's, wraps in `oida.RouterFunc`.
- `frontend.Mount`, `frontend.MountServeMux`, `frontend.Router` and
  `frontend.Handler(opts)` are gone; `frontend.Handler(recorder)` is what was
  `HandlerFor`. Nothing outside oida should name the package at all.
- `Options.ReadEnv`, which `NewOptions` sets, is what applies the `OIDA_*`
  environment inside `New`. Options built as a literal read no environment.

## Steps

1. **platform first**, because phpscript takes the tracer from it.
   - `telemetry_module.go`: `oida.TracingMiddleware(options)` becomes
     `tracer.Middleware`, and `frontend.Mount(r, m.options)` becomes
     `oida.Mount(r, m.tracer)`. The `frontend` import goes with it.
   - `options.go`: `oida.NewOptions()` takes the service name; pass the one
     the platform already knows.
   - Bump `github.com/titpetric/oida` to v0.3.0, run the tests.

2. **phpscript/telemetry/alias.go**: delete the bindings for symbols that no
   longer exist, keeping the ones the repository calls. `TracingMiddleware`
   becomes a method call on the tracer the module holds, so the wrapper goes
   rather than being rewritten. `TraceHost` and `BackgroundHost` have no
   replacement in the API: if the grouping label is still wanted, read
   `Trace.HTTP` and fall back to a constant phpscript owns.

3. **phpscript call sites**:
   - `cmd/phpscript/server/run.go:522` and `run_test.go:194` call
     `telemetry.Mount(router, options)`; they pass the tracer now.
   - `telemetry.NewOptions()` gains the service name.
   - The two `TracingMiddleware` uses take the middleware off the tracer.

4. **Bump and verify**: `go get github.com/titpetric/oida@v0.3.0`,
   `go mod tidy`, then the phpscript test suite. `telemetry/module_test.go`
   exercises the recorder end to end, so a wiring mistake surfaces there.

## Worth deciding while doing it

- The alias file binds far more than phpscript uses. Binding only the 35
  symbols the repository calls would make the next oida upgrade a smaller
  read.
- `platform` and `phpscript` both name oida. That is what the telemetry
  package documentation says, and it is why this migration touches two
  repositories; folding the host wiring into one of them would end that.
