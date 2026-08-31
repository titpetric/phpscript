# Go bindings

Go bindings let an embedding application expose selected functions and object capabilities to PHP without serialization or an inter-process call. The PHP VM still adds parsing/execution and reflection overhead; the invoked implementation runs directly in the host process.

## Runtime setup

Create a runtime, attach its lifecycle context, and register the entry points the script needs to call:

```go
var out strings.Builder
rt := runner.New(&out, runner.Options{})
rt.SetContext(request.Context())
rt.SetExprCache(sharedExprCache)
rt.RegisterFunc("lookup_name", lookupName)
rt.RegisterConstructor("Storage", NewStorage)

program, err := parser.Parse(source)
if err == nil {
	err = rt.Run(program)
}
```

Registration is per runtime. Class and function names become part of that runtime's PHP environment; Go APIs are not exposed automatically. Registration is not a complete method allowlist, however: PHP reflection dispatch sees the dynamic concrete value returned by a constructor. Every exported method on that concrete type is callable and every exported field is readable, even when the constructor's declared return type is an interface. Return a dedicated facade type when the underlying implementation has exports that scripts must not see.

Methods need no separate registration. If a constructor or registered function
returns a concrete Go value, PHP invokes its exported methods through ordinary
`$value->method(...)` syntax. Method lookup is case-insensitive and accepts
snake case, so a returned `time.Time` provides `$time->format(...)`,
`$time->add(...)`, and `$time->unix()` directly. Package-level Go functions are
not methods and still need explicit registration when they belong on the PHP
surface, for example `time.Now` as `DateTime::now`. This keeps `Time` aligned
with the method set of Go's `time.Time`, while `DateTime` represents the
package-level API.

The argument bridge also recognizes Go's `time.Duration` type. A PHP string is
parsed with `time.ParseDuration` before invoking any registered function or
automatically exposed method that declares a duration parameter. Thus a native
`time.Time` accepts `$time->add("30m")` without an adapter, while callers may
still pass a `Time\Duration` value or an integer nanosecond count. An invalid
duration string is rejected as a type error before reflection invokes the Go
method.

For request-oriented hosts, retain one concurrency-safe `runner.ExprCache` and install it on each fresh runtime with `SetExprCache`. This reuses compiled expression programs across requests while request globals, context, output, and registered capabilities remain isolated in their own runtime. The built-in HTTP server and annotated route service configure shared expression and include caches for their request runtimes.

## Binding a constructor

`RegisterConstructor` maps a PHP class name to any Go function accepted by the reflection bridge:

```go
type Storage interface {
    Set(context.Context, string, string)
    Get(context.Context, string) (Record, error)
}

func NewStorage(ctx context.Context) (Storage, error) {
    tenant, _ := ctx.Value(tenantKey{}).(string)
    return &memoryStorage{tenant: tenant}, nil
}

rt.SetContext(ctx)
rt.RegisterConstructor("Storage", NewStorage)
```

PHP construction invokes that function and leaves its first non-error return value on the PHP stack:

```php
$storage = new Storage;
```

The constructor may take ordinary PHP-supplied arguments after an optional leading `context.Context`. Return slots declared exactly as `error` are omitted; a non-nil value in any such slot becomes a PHP runtime error and can be handled with `try`/`catch`. Among all other slots, the first non-nil interface value is exposed and later values are discarded. Prefer the conventional `(T, error)` shape and avoid additional returns.

Missing non-variadic constructor arguments are padded with their Go zero values; relying on this differs from PHP default-parameter semantics and is best avoided.

Go constructors take precedence when the same name is also declared as a PHP class. Namespaced registrations use escaped Go strings, for example `rt.RegisterConstructor("Service\\Database", constructor)`. The built-in `Database` and `SharedMemory` bindings use bare class names.

## Invoking Go methods

Once a constructor returns a Go value, PHP `->` calls exported methods on that value. Method lookup is case-insensitive, so PHP `$storage->get(...)` can invoke Go's `Get` method:

```php
$storage->set("color", "blue");
$record = $storage->get("color");
echo $record->value;
```

If the first method parameter is exactly `context.Context`, the runtime inserts its lifecycle context before the arguments supplied by PHP. Other arguments are matched positionally, and omitted trailing arguments are padded with their Go zero values, as they are for constructors and registered functions.

Method returns follow the same exact-`error` and first-non-nil-value rules as constructors. A named concrete type that implements `error`, or an error stored in an `any` return slot, is not recognized as an error slot.

## Binding functions

`RegisterFunc` exposes a free Go function under a PHP function name:

```go
func LookupName(ctx context.Context, id int64) (string, error) {
    return repository.LookupName(ctx, id)
}

rt.RegisterFunc("lookup_name", LookupName)
```

```php
$name = lookup_name(42);
```

Registered functions use the same positional conversion and return/error rules as constructors. Variadic Go functions are supported. Missing fixed arguments are padded with zero values.

The context injected into a free function also contains the active PHP scope; runtime helpers can retrieve it with `runner.ScopeFromContext`. Constructors and Go methods receive the runtime lifecycle context directly, without that scope value.

## Output parameters

A binding returns its result; it cannot write back into a caller's variable, because a PHP variable in this runtime is a name in a frame table rather than a cell a Go function could hold. The one exception is arranged at compile time: `byRefArgs` in `model/byref.go` names the argument positions that are outputs, per function, which is where both runtimes read it from, and an argument at such a position that is a plain variable is emitted as a setter closure instead of its value. `preg_match`'s `$matches` is the case that exists.

The table is a package-level variable rather than part of this API, so a host binding cannot currently declare an output parameter. Return a collection instead. See [Value semantics](../types/value-semantics.md#output-parameters) for the mechanism.

## Context propagation

Two similarly named context types have separate responsibilities:

- `context.Context` is the Go lifecycle/request context. Set it with `Runtime.SetContext`. It is injected only when the callable's **first** parameter is exactly `context.Context`.
- `runner.Context` is phpscript's HTTP data adapter. `runner.FromRequest(r)` builds it, and its `Register` method installs the request superglobals and the header functions. See [Predefined variables](../predefined-variables/README.md).

Neither `runner.Context` nor `*runner.Context` is automatically injected into constructors, methods, or functions. Pass host data through `context.Context`, close over it in a registered function, or register request globals explicitly. A typical HTTP setup uses both paths:

```go
rt.SetContext(r.Context())

requestContext := runner.FromRequest(r)
requestContext.Register(rt)
```

The registered `header()` function stages response headers on `requestContext`. The host must copy `requestContext.ResponseHeaders()` to the HTTP response before committing its body.

Context values, cancellation, and deadlines remain available to bound Go APIs because the original lifecycle context is propagated. The runtime defaults to `context.Background()` when no context is set.

## Value conversion

Arguments remain dynamically typed on the PHP side. At the Go boundary the reflection bridge:

1. converts `nil` to the zero value of the declared target type;
2. uses a non-nil value directly when assignable to the declared Go type;
3. renders the value as PHP renders it in a string context when the target is a string, so `strlen(65)` measures `"65"` rather than Go's conversion of the code point 65 to `"A"`;
4. uses Go reflection conversion when the source type is convertible; and
5. otherwise passes the original value to reflection, which fails at runtime if its type does not match the Go signature.

Omitted trailing arguments are padded with their Go zero values, for constructors, registered functions and methods alike. A Go binding has no optional parameters, so padding is how PHP's optional arguments are spelled.

Passing more arguments than a non-variadic callable declares is refused, with a throwable naming the callable: `strlen() expects at most 1 argument, 2 given`. PHP refuses the same call as `ArgumentCountError` and words it `expects exactly 1 argument`; the wording differs because padding makes every parameter after the first omitted one optional. An error a binding returns belongs to no PHP class, so every clause takes it and the caught value answers `getMessage()` and the rest of the `Throwable` methods. See [Exceptions](../exceptions/README.md).

This is not a complete PHP-to-Go coercion system. Prefer stable scalar signatures and validate values in the binding when scripts are untrusted.

Go slices and arrays can be traversed with PHP `foreach`. Exported Go struct fields can be read with `->` using case-insensitive names. Go maps and slices support PHP-style index reads. phpscript arrays are `*model.Array`; they are not automatically converted to arbitrary Go map or slice types.

Bindings run in-process and may expose mutable Go pointers. Their lifetime, thread safety, authorization, and transaction boundaries remain the host application's responsibility.

## HTTP binding benchmark

`BenchmarkGoBindingHTTP` in `tests/fixtures_test.go` compares two `http.HandlerFunc` implementations performing the same operation:

1. construct `Storage` from the request context;
2. call `Set`, `Get`, and `Tenant`; and
3. write `acme:blue` to the HTTP response.

The `go_handler` sub-benchmark calls the Go constructor and methods directly. The `php_vm_handler` sub-benchmark creates a runtime for the request, registers the constructor, and performs the equivalent calls from a pre-parsed PHP program. It uses a shared source-keyed expression cache, warmed by the correctness check before timing, as production HTTP hosts should. Source parsing, VM compilation, and request creation are excluded, with one request reused across iterations. Each fresh runtime still transpiles the current AST so closures and nested-expression metadata remain request-local. Per-iteration response-recorder allocation, runtime setup, transpilation, VM execution, reflective dispatch, and response writing are included where applicable.

Run it with:

```bash
go test ./tests -run '^$' -bench '^BenchmarkGoBindingHTTP$' -benchmem
```

The difference between the two sub-benchmarks estimates the end-to-end overhead of using a fresh PHP VM for this binding path on the current machine. It is not a reflection-only microbenchmark and should not be treated as a fixed production latency figure.
