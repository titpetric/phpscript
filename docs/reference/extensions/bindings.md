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

Registration is per runtime. Class and function names become part of that runtime's PHP environment; Go APIs are not exposed automatically. Registration is not a complete method allowlist: PHP reflection dispatch sees the dynamic concrete value returned by a constructor. Every exported method on that concrete type is callable and every exported field is readable, even when the constructor's declared return type is an interface. Return a dedicated facade type when the underlying implementation has exports that scripts must not see.

Methods need no separate registration. If a constructor or registered function returns a concrete Go value, PHP invokes its exported methods through ordinary `$value->method(...)` syntax. Method lookup is case-insensitive and accepts snake case and a returned `time.Time` answers `$time->format(...)`, `$time->add(...)` and `$time->unix()` directly. Package-level Go functions are not methods and still need explicit registration when they belong on the PHP surface, for example `time.Now` as `DateTime::now`. This keeps `Time` aligned with the method set of Go's `time.Time`, while `DateTime` represents the package-level API.

The argument bridge also recognizes Go's `time.Duration` type. The bridge parses a PHP string with `time.ParseDuration` before invoking any registered function or automatically exposed method that declares a duration parameter. Thus a native `time.Time` accepts `$time->add("30m")` without an adapter, while callers may still pass a `Time\Duration` value or an integer nanosecond count. The bridge rejects an invalid duration string as a type error before reflection invokes the Go method.

For request-oriented hosts, keep one concurrency-safe `runner.ExprCache` and install it on each fresh runtime with `SetExprCache`. This reuses compiled expression programs across requests while request globals, context, output, and registered capabilities remain isolated in their own runtime. The built-in HTTP server and annotated route service configure shared expression and include caches for their request runtimes.

## Calling PHP from Go

The sections above run a file. `runner.Lookup` goes the other way and reaches one function inside it, under a signature the host declares:

```go
rt := runner.New(os.Stdout, runner.Options{RootFS: os.DirFS(root)})
stdlib.Register(rt)

handle, err := runner.Lookup[func(*http.Request) bool](rt, "App\\Handler\\main")
if err != nil {
	return err
}
ok := handle(request)
```

`runner.Lookup` is `plugin.Lookup` over a PHP source tree, with the type parameter standing in for the type assertion Go's own plugin API needs. A script may declare the symbol anywhere in the tree: the lookup matches the name against the functions already declared on the runtime and against every program in its include cache, and a runtime whose cache is empty parses its source root through a `Precompiler` on the first lookup that needs one.

The parser qualifies a free function with the namespace its file declares and `App\Handler\main` is the whole name. Resolution tries that name first, case-insensitively as PHP compares one, then falls back to matching the trailing segment and a bare `main` finds it and `Handler\main` picks between two handlers that share the last segment. A name that matches nothing, or more than one declaration, is a `*runner.LookupError` carrying what it matched and a host wiring its handlers sees an ambiguity at startup, and no handler runs on a guess.

`T` must be a function type returning at most one value and an optional trailing `error`. The check happens at the lookup, not at the call. A signature with no error result has nowhere to report a failure and a throw goes to the runtime's error sink, the same place the error of a script that ended badly goes; with one, it fills the slot and the value result stays at its zero.

An invocation carries the arguments alone. The invocation registers no `runner.Context`, `$_GET`, `$_POST` and `$_SERVER` stay undefined, and no file body runs: the runtime hoists the declaring program for its declarations only. What the host passes arrives as that same Go value and an `*http.Request` handed in is the script's `HTTP\Request` and its headers, its query and its body read off the request itself:

```php
<?php
namespace App\Handler;

function main(\HTTP\Request $r) {
	$r->parse_form();
	return $r->method === "POST" && $r->post_form_value("name") !== "";
}
```

The bridge widens arguments to the value set the interpreter operates on, which means the predeclared numeric types become `int64` and `float64`; a named scalar keeps its name, and everything else passes through. Results take PHP's own coercions for the predeclared scalar kinds and a function returning `1` fills a `bool` the way `if (1)` reads it, and a PHP array fills a `[]T` or a `map[K]V`.

Two limits apply. A looked-up symbol executes through the interpreter even on a runtime built with `NewFlatStack`, because a flat-declared function lives in the bytecode program's own table and not the runtime's. And `Options.Include` is not run per invocation and a composer autoloader is not installed by a lookup; a function that needs one requires it in its own body, or the host runs the prelude on the runtime first.

The returned function belongs to its runtime, and a runtime serves one goroutine. A host calling one concurrently builds a runtime per goroutine and shares the include and expression caches between them, as the HTTP server does per request.

`BenchmarkLookup` in `tests/runner/lookup_bench_test.go` is that arrangement, split into what each runtime costs (`bind`), what each call costs (`invoke`, against `callable` for the bridge's own share), and what a whole request cycle costs (`request`, against `runtime` for the part of it that is not the script). `BenchmarkLookupTreeSize` holds resolution to constant time against the size of the source root, which matters because a concurrent host binds per runtime and so often per request:

```bash
go test ./runner -run '^$' -bench '^BenchmarkLookup' -benchmem -cpu 1,2,4
```

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

The constructor may take ordinary PHP-supplied arguments after an optional leading `context.Context`. The bridge omits return slots declared exactly as `error`; a non-nil value in any such slot becomes a PHP runtime error that `try`/`catch` catches. Every other slot reaches PHP: one value is the result, and two or more become a PHP list in declaration order, which a script destructures. `regexp.Regexp.LiteralPrefix` returns `(string, bool)` and `list($prefix, $complete) = $rx->literal_prefix();` reads both, and keeping only the first would answer a different question.

The bridge pads missing non-variadic constructor arguments with their Go zero values; relying on this differs from PHP default-parameter semantics and is best avoided.

Go constructors take precedence when the same name is also declared as a PHP class. Namespaced registrations use escaped Go strings, for example `rt.RegisterConstructor("Service\\Database", constructor)`. The built-in `Database` and `SharedMemory` bindings use bare class names.

## Invoking Go methods

Once a constructor returns a Go value, PHP `->` calls exported methods on that value. Method lookup is case-insensitive and PHP `$storage->get(...)` can invoke Go's `Get` method:

```php
$storage->set("color", "blue");
$record = $storage->get("color");
echo $record->value;
```

If the first method parameter is exactly `context.Context`, the runtime inserts its lifecycle context before the arguments supplied by PHP. The runtime matches other arguments positionally and pads omitted trailing arguments with their Go zero values, as it does for constructors and registered functions.

Method returns follow the same exact-`error` and multi-value rules as constructors. A named concrete type that implements `error`, or an error stored in an `any` return slot, is not recognized as an error slot.

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

Registered functions use the same positional conversion and return/error rules as constructors. The bridge accepts variadic Go functions and pads missing fixed arguments with zero values.

The context injected into a free function also contains the active PHP scope; runtime helpers can retrieve it with `runner.ScopeFromContext`. Constructors and Go methods receive the runtime lifecycle context directly, without that scope value.

## Callbacks

A binding may take a callable. PHP has six spellings for one, and every one of them fills a Go function parameter, on a registered function, on a method of a bound Go value and on a constructor alike:

| Spelling                   | Written as                              | Reaches                                        |
|----------------------------|-----------------------------------------|------------------------------------------------|
| A closure                  | `function ($s) { ... }`                 | The declaration, with its `use (...)` values   |
| A declared function's name | `"strtoupper"`                          | The runtime's function table, binding included |
| A static method            | `"Caser::quiet"`                        | The declaration, against an empty instance     |
| A method on its receiver   | `array($obj, "method")`, `$obj->method` | The declaration, with that receiver            |
| A class's static method    | `array("Caser", "quiet")`               | As the string spelling                         |
| An object with `__invoke`  | `$obj`                                  | That method, with that receiver                |

[First-class callable syntax](../functions/README.md#first-class-callable-syntax) adds no row: `strtoupper(...)` and `$obj->method(...)` resolve to the Closure of the first row, and a binding receives that.

A binding declares the parameter in one of two shapes. The uniform one is what `Runtime.Callable` answers and what the runtime invokes everywhere:

```go
rt.RegisterFunc("each_word", func(s string, visit func(...any) (any, error)) error {
	for _, word := range strings.Fields(s) {
		if _, err := visit(word); err != nil {
			return err
		}
	}
	return nil
})
```

The other is Go's own terms, the shape a library's method set already has - `regexp.Regexp.ReplaceAllStringFunc` takes a `func(string) string`. The bridge wraps those with `reflect.MakeFunc` over the declared signature:

```go
rt.RegisterFunc("map_word", func(s string, fn func(string) string) string { return fn(s) })
```

Two rules apply to the declared signature. It returns one value or none, because a PHP closure answers one; a second result has nothing to come from and the bridge refuses the argument as a type error. And the signature is not variadic, apart from the uniform shape itself. The bridge fits what the callable answers to the declared result through the same conversion table an argument takes and a string result reads the value as PHP renders it in a string context and a `bool` result takes a bool and not a truthy int.

Errors differ between the two shapes, and that is the reason to prefer the uniform one for anything that can fail. A signature with an `error` slot gets the error the callback reported. A signature without one leaves it nowhere to go and it crosses the intervening Go frames as a panic, which the host boundary unwraps into the throwable the script threw: a `catch` written around the call takes the script's own exception, never a host panic naming a Go type. A binding between the callback and the boundary must not recover a panic it did not raise.

A callable belongs to the runtime making the call. A comparator or a handler built before an HTTP server started runs on the worker answering the request; see [program re-entry](../../design.md#program-re-entry).

### Describing one to a host

`Runtime.Callable` answers the call. `Runtime.AsCallable` answers the value, a `*runner.Callable`, for a host that runs the thing later and possibly on another runtime:

```go
callable, err := rt.AsCallable(handler)
if err != nil {
	return err
}
result, err := callable.Invoke(worker, w, r)
```

`AsCallable` is the narrower of the two, because it needs a declaration to carry. It refuses the two array spellings by decision: a handler is a function of its arguments, and wrapping one in an array to name a method is a spelling this API leaves out. They stay callable everywhere else. It refuses a Go func for a different reason: the func carries no declaration, and a host holding one already holds the call. The error names which one and what the alternative is.

`Callable.Captures` reports whether the value holds anything from its defining scope: a closure's `use (...)` list, the `$this` one written in a method binds, or a bound method's receiver. Its captures stay read-only, shared by every runtime running it. A declared function and a static method capture nothing and run anywhere.

## Output parameters

A binding returns its result; it cannot write back into a caller's variable, because a PHP variable in this runtime is a name in a frame table, with no cell a Go function could hold. The one exception happens at compile time: `byRefArgs` in `model/byref.go` names the argument positions that are outputs, per function, and both runtimes read it from there; a plain variable at such a position compiles to a setter closure in place of its value. `preg_match`'s `$matches` is the case that exists.

The table is a package-level variable and not part of this API and a host binding declares no output parameter. Return a collection instead. See [Value semantics](../types/value-semantics.md#output-parameters) for the mechanism.

## Context propagation

Two similarly named context types have separate responsibilities:

- `context.Context` is the Go lifecycle/request context. Set it with `Runtime.SetContext`. The runtime injects it only when the callable's **first** parameter is exactly `context.Context`.
- `runner.Context` is phpscript's HTTP data adapter. `runner.FromRequest(r)` builds it, and its `Register` method installs the request superglobals and the header functions. See [Predefined variables](../predefined-variables/README.md).

Neither `runner.Context` nor `*runner.Context` is automatically injected into constructors, methods, or functions. Pass host data through `context.Context`, close over it in a registered function, or register request globals explicitly. A typical HTTP setup uses both paths:

```go
rt.SetContext(r.Context())

requestContext := runner.FromRequest(r)
requestContext.Register(rt)
```

The registered `header()` function stages response headers on `requestContext`. The host must copy `requestContext.ResponseHeaders()` to the HTTP response before committing its body.

Context values, cancellation, and deadlines remain available to bound Go APIs because the runtime passes the original lifecycle context through. The runtime defaults to `context.Background()` when the host sets none.

## Value conversion

Arguments remain dynamically typed on the PHP side. At the Go boundary the reflection bridge:

1. converts `nil` to the zero value of the declared target type;
2. uses a non-nil value directly when assignable to the declared Go type;
3. renders the value as PHP renders it in a string context when the target is a string and `strlen(65)` measures `"65"`, where Go's conversion of the code point 65 produces `"A"`;
4. uses Go reflection conversion when the source type is convertible; and
5. otherwise passes the original value to reflection, which fails at runtime if its type does not match the Go signature.

The bridge pads omitted trailing arguments with their Go zero values, for constructors, registered functions and methods alike. A Go binding has no optional parameters and padding is how a script spells PHP's optional arguments.

The bridge refuses a call passing more arguments than a non-variadic callable declares, with a throwable naming the callable: `strlen() expects at most 1 argument, 2 given`. Both numbers are the script's: an injected `context.Context` is no PHP parameter and a binding declaring one reports the parameters a script may fill and the arguments it wrote, never the longer Go signature. PHP refuses the same call as `ArgumentCountError` and words it `expects exactly 1 argument`; the wording differs because padding makes every parameter after the first omitted one optional. An error a binding returns belongs to no PHP class and every clause takes it and the caught value answers `getMessage()` and the rest of the `Throwable` methods. See [Exceptions](../exceptions/README.md).

This is not a complete PHP-to-Go coercion system. Prefer stable scalar signatures, and check values in the binding when the host runs untrusted scripts.

PHP `foreach` traverses Go slices and arrays. `->` reads exported Go struct fields under case-insensitive names. Go maps and slices support PHP-style index reads. phpscript arrays are `*model.Array`; they are not automatically converted to arbitrary Go map or slice types.

A `[]byte` is the exception to that, and is a PHP string: PHP's strings are byte strings, and half of Go's text API takes byte slices. `echo`, `strlen`, `var_dump`, `gettype`, `is_string`, `===`, an offset read and truthiness all read a returned `[]byte` as its text, `is_array` is false, and `foreach` over one iterates zero times. A `[][]byte` is still a list, of strings. `phpval.Bytes` is the test, and [Regexp bindings](../../bindings-regexp.md) lists where the runtime applies it.

A binding may declare its callback in Go's own terms, in place of the uniform `func(...any) (any, error)`. `regexp.Regexp.ReplaceAllStringFunc` declares `func(string) string`, and every spelling PHP calls a callable fills it: a closure, a declared function by name, `Class::method`, `array($object, "method")`. A Go function type with no error slot leaves a callback nowhere to report one and an error the PHP callable raises crosses the intervening Go frames as a panic and arrives at the caller as the throwable the script threw. A callback target must return one value or none, and must not be variadic.

Bindings run in-process and may expose mutable Go pointers. Their lifetime, thread safety, authorization, and transaction boundaries remain the host application's responsibility.

## HTTP binding benchmark

`BenchmarkGoBindingHTTP` in `tests/fixtures_test.go` compares two `http.HandlerFunc` implementations performing the same operation:

1. construct `Storage` from the request context;
2. call `Set`, `Get`, and `Tenant`; and
3. write `acme:blue` to the HTTP response.

The `go_handler` sub-benchmark calls the Go constructor and methods directly. The `php_vm_handler` sub-benchmark creates a runtime for the request, registers the constructor, and performs the same calls from a pre-parsed PHP program. It uses a shared expression cache keyed by AST node, warmed by the correctness check before timing, as production HTTP hosts should. The timing excludes source parsing and request creation, and reuses one request across iterations. It counts per-iteration response-recorder allocation, runtime setup, expression compilation on a cache miss, closure execution, reflective dispatch, and response writing where applicable.

Run it with:

```bash
go test ./tests -run '^$' -bench '^BenchmarkGoBindingHTTP$' -benchmem
```

The difference between the two sub-benchmarks estimates the end-to-end overhead of using a fresh PHP VM for this binding path on the current machine. That difference is not a reflection-only microbenchmark; do not treat it as a fixed production latency figure.
