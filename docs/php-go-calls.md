# PHP to Go call latency

[Allocation performance in bindings](./allocation-performance.md) is about what
a binding pays to hand a value back. This document is about the other half:
what it costs to reach the binding at all.

The two are separable, and the reason to separate them is that one of them is
under the script's control in a way that is not obvious. A binding's return
shape is a decision made once, in Go, when the binding is written. Its dispatch
cost is decided by the script, every time, by how the call is spelled:

**A call pays for how it is spelled, not for what it does.**

The same Go function, registered twice under two names, costs 1683 ns called one
way and 3020 ns called the other. Nothing about the work differs.

## The measured baseline

All numbers from `tests/bridge_test.go` on an Intel N150, Go 1.27. The subject
is `tests.AnalyticsRing.Record`, a fixed-size overwriting buffer that stores
three fields into preallocated storage. It allocates nothing
(`TestAnalyticsRecordDoesNotAllocate` pins that), so every allocation below
belongs to the bridge.

Each engine row is the marginal cost of one call: a script calling the binding
a thousand times, minus the same loop with an empty body. Subtracting the
control is what keeps the row from measuring the engine's `for` loop.

| Path                                     | ns/call | allocs/call | calls/s    |
|------------------------------------------|--------:|------------:|-----------:|
| Direct Go call                           |      15 |           0 | 66,800,000 |
| `reflect.Value.Call`                     |     197 |           0 |  5,100,000 |
| expr-lang alone                          |     630 |           8 |  1,590,000 |
| flatstack, `func(...any) (any, error)`   |     742 |           7 |  1,350,000 |
| flatstack, namespaced                    |    1115 |           8 |    897,000 |
| flatstack, global                        |    1207 |           8 |    829,000 |
| interpreter, `func(...any) (any, error)` |    1112 |           9 |    899,000 |
| interpreter, global                      |    1683 |          12 |    594,000 |
| flatstack, leading `context.Context`     |    1957 |          12 |    511,000 |
| interpreter, leading `context.Context`   |    2653 |          17 |    377,000 |
| interpreter, namespaced                  |    3020 |          20 |    331,000 |

Construction, which reaches the host through `__new` rather than a function
call, and includes one method call on the constructed value:

| Path                        | ns/op | allocs/op |
|-----------------------------|------:|----------:|
| `new` + method, flatstack   |  4614 |        27 |
| `new` + method, interpreter |  7981 |        44 |

**The headline: a PHP script can make roughly 330,000 to 1,350,000 calls per
second into Go on one core**, and which end of that range it lands on is
decided by three choices, none of which are about the binding's work.

## The rules

**1. On the interpreter, a namespaced call costs 79% more than a global one.**

| Spelling             | ns/call | allocs/call |
|----------------------|--------:|------------:|
| `analytics_record()` |    1683 |          12 |
| `Analytics\record()` |    3020 |          20 |

Same function value, same buffer, registered under two names. The difference is
dispatch, and it is explained below.

**2. On flatstack, that penalty does not exist.**

| Spelling             | ns/call |
|----------------------|--------:|
| `Analytics\record()` |    1115 |
| `analytics_record()` |    1207 |

The two are the same within noise. A namespaced binding is not intrinsically
expensive; it is expensive on one of the two engines, for a reason specific to
that engine. Read rule 1 as a statement about the interpreter, not about
namespaces.

**3. A `context.Context` parameter costs about 900 ns and 5 allocations.**

| Binding signature                             | ns/call |
|-----------------------------------------------|--------:|
| `func(string, int64, int64)`                  |    1683 |
| `func(context.Context, string, int64, int64)` |    2653 |

Take a context when the binding needs one: it is how a binding reaches the
calling scope, the request, the environment and the trace. Do not take one out
of habit on a binding called in a loop. Note that every constructor and every
Go method call pays this whether or not the binding asked, which is most of why
the construction figures above are what they are.

**4. The uniform shape is worth a third of the call.**

A binding registered as `func(...any) (any, error)` is dispatched by
`runner.invokeFast`'s type switch and never touches `reflect`:

| Binding signature            | interpreter | flatstack |
|------------------------------|------------:|----------:|
| `func(...any) (any, error)`  |     1112 ns |    742 ns |
| `func(string, int64, int64)` |     1683 ns |   1207 ns |

That is 34% on the interpreter and 39% on flatstack, for a signature that gives
up its own type checking: the binding then coerces its arguments by hand. It is
worth it for a binding called in a loop and not worth it for one called once
per request. `tests/analytics.go` registers both shapes so this row stays
measured rather than remembered.

**5. Prefer flatstack for call-heavy scripts.**

Flatstack is 28% faster on the same call and allocates a third less. It is not
universally applicable: `flatstack.Supports` reports whether a program compiles
to bytecode, and one that does not falls back to the interpreter silently.
Benchmarks gate on `Supports` for that reason.

## Why the numbers look like this

**The namespaced penalty is two reflect dispatches instead of one.**

`runner/transpile.go`'s `emitCall` routes a call through one of two paths:

```go
if n.Fallback != "" || strings.ContainsRune(n.Name, '\\') {
	return joinCall("__func", strconv.Quote(n.Name), strconv.Quote(n.Fallback), args), nil
}
t.addCall(n.Name)
return joinCall(n.Name, "", "", args), nil
```

A global call becomes a bare expr identifier, and `runner.installFunc` puts the
binding's closure into the evaluation environment under that name. Calling it
reaches the binding directly.

A name containing `\` becomes `__func("Analytics\\record", "", args...)`.
`__func` is `adapt(rt.helperFunc(ref))`, and `helperFunc`'s signature,
`func(name, fallback string, args ...any) (any, error)`, is not one of the
shapes `runner.invokeFast` recognises. So the call does a full
`reflect.Value.Call` on the *helper* (building a `[]reflect.Value`, coercing
each argument), then a map lookup for the name, and only then the second
`invokeAny` on the binding itself.

Flatstack does not have this penalty because it does not go through expr at
all: `runner/flatstack.go` calls `helperFunc` directly as a Go function.

The same `invokeFast` miss applies to every other transpiler helper (`__new`,
`__call`, `__get`, `__arith`, `__array`), so a full reflect call sits behind
every `new`, every `$obj->method()` and every arithmetic operation on the
interpreter. Only `__bool`, `__concat` and `__index` hit the fast path. That is
the single largest remaining cost in the table.

**The context cost is up to four allocations, not one.**

`runner.invokeWithScopeContext` derives
`contextWithScope(contextWithEnv(rt.ctx, rt.Env), scope)`. That is two
`context.WithValue` calls, and `contextWithScope` adds up to two more for
`telemetry.WithSpanFilename` and `WithSpanLine`, both of which fire because
`__FILE__` and `__LINE__` are set in scope on every statement.

## Remaining work

These are the costs the table above locates, ranked by what they are worth.
None has been applied yet: the baseline exists first so that each one can be
proved rather than assumed.

| # | Change                                                                                                                                                         | Expected                                                                                                              | State       |
|---|----------------------------------------------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------|-------------|
| 1 | Hand-written uniform shims for the `__` transpiler helpers, so they stop going through `adapt` and `reflect`                                                   | Removes one full reflect dispatch from every namespaced call, `new`, method call and arithmetic op on the interpreter | Not started |
| 2 | Emit namespaced calls as a mangled bare identifier so they take the `installFunc` fast path                                                                    | Closes the 79% interpreter gap in rule 1                                                                              | Not started |
| 3 | Precompute an invoker per registered callable, so `reflect.TypeOf`, `IsVariadic`, `NumIn` and `wantsContext` are resolved at registration rather than per call | 10-20 ns/call; prerequisite for 4 and 6                                                                               | Not started |
| 4 | Collapse the derived call context into one value and memoise it per scope                                                                                      | Most of the 900 ns in rule 3                                                                                          | Not started |
| 5 | Extend `invokeFast` to cover shapes whose coercion is identity or `phpString` only                                                                             | Moves rule 4's two rows together                                                                                      | Not started |
| 6 | Skip flatstack's per-call frame round trip for callees that do not read the frame                                                                              | Three maps and a quadratic scan per host call                                                                         | Not started |
| 7 | Constant-time case-insensitive lookup for functions, classes and constructors                                                                                  | Removes an O(table) scan from every exact-key miss                                                                    | Not started |

One item was considered and is **not** planned: removing the `defer recover()`
from `runner.invokeAny`. A panic in host code has to reach PHP as a catchable
error (`tests/fixtures/exceptions/host_panic_catch.phpt`), and the error carries the
callable's name, which is only available at the call site. A statement-level
boundary could not reproduce the message.

## How to measure

The microbenchmarks, which produce every row above:

```bash
go test ./tests/ -run XXX -bench BenchmarkBridge -benchmem -count=6 > before.txt
# apply a change
go test ./tests/ -run XXX -bench BenchmarkBridge -benchmem -count=6 > after.txt
benchstat before.txt after.txt
```

End to end, per engine, including the interpreter/flatstack/php comparison:

```bash
phpscript test --matrix --count 2000 --profile tests/fixtures/bindings/analytics_record.phpt
```

`phpscript test` reports P50, P95, P99, allocations per operation and bytes per
operation per fixture, and renders a Markdown table when its output is not a
terminal. Both are wired up as `atkins bench:bridge`.

`tests/bridge_test.go` also holds `TestBridgeCellsAgree`, which runs every cell
once and checks that each recorded exactly one entry. A cell that stopped
calling the host would otherwise look like the fastest row in the table.

## Go plugins

A binding delivered by a Go plugin costs the same as a compiled-in one: a
plugin symbol is an ordinary function value once `dlopen` has run, and the
runtime cannot tell the difference. The cost of a plugin is at load, not per
call. See [Go plugins](./reference/extensions/plugins.md).
