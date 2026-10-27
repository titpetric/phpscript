# Flat-stack runtime

The `flatstack` package is an alternative entry point for embedding phpscript. It keeps the `runner` API while adding a compile-once flat-bytecode path for a growing subset of the public `model.Program` AST.

The API is experimental: it may change at any point before a stable release, and a later release may fold it into the existing runner APIs.

```diagram
parser.Parse
     │
     ▼
model.Program
     │
     ├── supported by flatstack ──▶ compiler ──▶ immutable opcodes
     │                                          │
     │                                          ▼
     │                                  operand/slot VM
     │                                          │
     │                                          ▼
     │                                   runner host bridge
     │
     └── unsupported ─────────────▶ runner interpreter + expr VM
```

The compiler checks and compiles the entire program before execution. If any node is not supported, no flat instruction runs and `Runtime.Run` delegates the complete program to the runner. This prevents fallback from repeating constructors, method calls, output, or other side effects.

## Using flatstack

Normal use differs only in the imported package and constructor:

```go
package main

import (
	"context"
	"os"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/stdlib"
)

func main() {
	program, err := parser.Parse(`<?php echo "hello"; ?>`)
	if err != nil {
		panic(err)
	}

	runtime := flatstack.New(os.Stdout, flatstack.Options{})
	runtime.SetContext(context.Background())
	stdlib.Register(runtime)

	if err := runtime.Run(program); err != nil {
		panic(err)
	}
}
```

To change only an existing import path while retaining the local identifier `runner`, use an import alias:

```go
import runner "github.com/titpetric/phpscript/flatstack"
```

Existing calls such as `runner.New`, `runner.Options`, `runner.IsExit`, and `runner.NewExprCache` then continue to compile.

## API compatibility

The following flatstack types are aliases of their runner counterparts:

- `Runtime` and `Options`
- `Context` and `Scope`
- `ExprCache` and `IncludeCache`
- `ExitError` and `HostPanicError`
- `IncludeFunc` and `Transpiler`

Because `flatstack.Runtime` is an alias of `runner.Runtime`, APIs that accept a concrete `*runner.Runtime` remain usable. In particular, these calls require no adapter:

```go
stdlib.Register(runtime)
stdlib.RegisterFS(runtime, ".")
runner.FromRequest(r).Register(runtime)
```

The package also forwards the runner package functions used by embedders:

- `NewContext` and `FromRequest`
- `NewScope` and `ScopeFromContext`
- `NewExprCache` and `NewIncludeCache`
- `NewTranspiler`
- `IsExit`

### Are runner and flatstack interchangeable?

For code that directly constructs and runs a runtime, **yes at the API and fallback-behavior level**. CI-style tests run the full fixture corpus through both imports.

They are **not yet independent engines with matching behavior**:

- `flatstack.Runtime` aliases `runner.Runtime`, which preserves the complete embedding API. Duplicating it would mean a second copy of runtime, standard-library, include, request and reflection state.
- Only the documented subset below executes as native flat bytecode.
- Every other valid program uses the existing runner interpreter.
- `annotations.Route` and the bundled CLI/server construct `runner.New` internally. Merely changing a callback type does not make those paths use flatstack.
- `Runtime.OnError` selects the interpreter because the compiler does not yet lower its per-statement recovery contract to bytecode.

Use `flatstack.Supports` when the program must execute as native bytecode:

```go
if err := flatstack.Supports(program); err != nil {
	return fmt.Errorf("benchmark would use runner fallback: %w", err)
}
```

Applications normally do not need this check because transparent fallback is the compatibility contract. Benchmarks should always use it unless they are intentionally measuring fallback.

## Native bytecode subset

Flat bytecode supports these statements:

- Variable and array-index assignment, including compound assignment and append
- Expression statements
- `echo`
- Inline HTML
- `if`/`else`
- `for`/`while` and `foreach`, including nested `break` and `continue`
- `switch`, fallthrough, and `break`
- `try`/`catch`/`finally` and `throw`
- Top-level PHP class declarations and free function declarations
- `include` / `include_once` / `require` as statements or expressions
- `list()` / array destructuring assignment
- `defer()`, registered on the frame in flight and run LIFO when that frame returns

It supports these expressions:

- Scalar literals
- Local variables, runtime globals, and constants through host lookup
- PHP arrays, keyed/unkeyed literals, reads, writes, and append
- Arithmetic (`+`, `-`, `*`, `/`, `%`)
- Comparisons (`==`, `!=`, `===`, `!==`, `<`, `<=`, `>`, `>=`)
- Bitwise operators (`&`, `|`, `^`, `<<`, `>>`)
- Short-circuit logical operators and unary `!`, `+`, `-`, and `~`
- Prefix/postfix increment and decrement of variables, array indexes, properties and function statics
- Assignment expressions and full/Elvis ternaries
- Host and PHP object construction with `new`, including a dynamic class name `new $cls(...)`
- Registered/free function calls and method calls, including a dynamic method name `$obj->$m(...)`, trailing variadic parameters and parameter defaults
- Static calls `Class::method(...)` and static properties `Class::$name`, dispatched through the host bridge onto the interpreter's resolution
- Function statics `static $x = ...` in named functions and methods, against the same per-statement storage the interpreter uses
- Invoking a callable held in a value: `$fn($x)`; a string naming a compiled function takes a VM frame, everything else resolves through the host
- First-class callable syntax, `callable(...)`, in all four call forms. The host resolves the callable and the value is the same Closure the interpreter builds, and the declaration it names runs there on call
- Property reads, writes and `unset($obj->prop)`
- String concatenation with `.`
- Anonymous functions, including a by-value `use (...)` capture list, a `$this` carried away from an enclosing method, and `static function () {}`

Arithmetic, coercion, comparison, array access, and truthiness live in the flat VM and its small PHP-semantics host boundary. The bridge uses runner's existing reflection path for registered Go constructors, functions and methods. The compatibility interpreter evaluates expressions on runner/expr, its closure-chain engine, for unsupported programs.

The current end-to-end corpus result is **262 of the 264 `.phpt` fixtures compiling native**; `tests/fixtures/functions/static_var_closure.phpt` and `tests/fixtures/functions/func_get_args.phpt` remain with the interpreter. `Supports` is the authoritative per-program answer; a fixture count is useful progress evidence, not a claim that phpscript implements the whole PHP language.

### Current native barriers

The complete program atomically selects fallback when it contains any unsupported form. The major remaining forms are:

- `compact()` of a name declared `static $x`, which lives in a bag and not a frame slot. Every other form compiles: a literal name becomes the slot it stands for, and a name computed at run time resolves through the name table the program carries
- `static $x` inside a closure (its bag counts per closure value, which is interpreter state), at top level, or as a by-reference output parameter
- By-reference closure captures `use (&$x)`, closure parameter defaults, and variadic or by-reference closure parameters
- `func_get_args()`, refused by name at every call site. A compiled frame seeds its declared parameters into slots and keeps no argument list and an argument past the last parameter is not in the frame to report. It names the one entry in `runner.ScopeBuiltins`, and the first-class reference `func_get_args(...)` is not refused, having no frame to read: the function table resolves that name and carries no scope builtin and both engines report the same undefined function
- Anonymous classes, `new class { ... }`. The bytecode carries a class name where an anonymous class carries its declaration
- `try` without a `catch` clause
- Casts
- PHP constructors (`__construct`) still run in the interpreter
- Nested `class` declarations (no-op, same as the interpreter hoist; PHP registers them at runtime)
- Included files still execute through the interpreter
- `include` and `require` both fail the request on a missing file (PHP `include` is a warning)
- A host without Include fails at `opInclude`, after earlier opcodes have run

These are not called "unsupported programs" at the public runtime boundary: they are valid phpscript programs and execute through runner. "Unsupported" in a `Supports` error means only "not yet lowerable to native flat bytecode."

### Closure identity

A closure the bytecode engine creates is a Go function value, where the interpreter's is a `runner.Callable`. A Go function value has no identity `===` can read - Go cannot compare two of them, and `reflect.DeepEqual` calls every non-nil pair unequal - so `$a = function () {}; $b = $a; $a === $b;` is false here and true on the interpreter and in php. That is the one documented disagreement between the two engines, and it does not extend to the first-class callable syntax: `greet(...)` and the other three spellings answer a `Callable` on both engines and `$fn(...) === $fn` holds on both.

Fixing it means the engine's closure value carries an identity, which means a value type both packages can see in place of a bare `func(...any) (any, error)`, and a case for it everywhere the host resolves a callable. Comparing the function pointer is no fix: two closures built from one literal share it, which would answer true where php answers false.

## Host calls, errors, and panics

Flat bytecode uses the runner's existing host bridge, including:

- Automatic `context.Context` injection
- PHP-style case-insensitive method lookup
- Argument conversion
- Go constructor and method error propagation
- Exported Go struct field access

The VM binds a frame handle (`engine.FrameLocals`) to the host once per run, with no copy of its locals into a map around every call. The host decides when a callee needs the scope: it passes no scope at all to a function-table hit whose signature does not take a `context.Context`, which is the interpreter's own contract for the same binding; context bindings and the undefined-function path materialise a scope from `Snapshot` before the callee body runs and write it back after. The by-reference marks rely on that snapshot-before-call ordering, and the engine's `vm_test` pins it.

`Snapshot` is the frame's named variables, the set `compact()`, `extract()` and `get_defined_vars()` read. It holds no argument list and the compiler refuses `func_get_args()`.

A host's own variables are the other half of the same split. `Run` asks an optional `Globals() map[string]any` for them and seeds them into the top-level frame, the way `runInterpreted` seeds its global scope before executing a file and a function and a closure reach a global exactly as far as they do in the interpreter: not at all. `Host.Lookup` answers the superglobals and the constant table, which every frame does see. Answering a global from `Lookup` instead handed `$argv` to every frame in the program.

The compiler also resolves binary operators into an opcode class, and the VM computes the both-`int64` shapes (and both-string concat) inline through the same `internal/phpval` rules `phpArith` reads. Every other operand shape dispatches to the host with the operator name and coercion has one home. Slot type inference stops here, and a measurement is why: values live in `[]any`, where the type assertion is the unboxing and an opcode holding monomorphic int operand slots saves nothing over the dynamic guard.

Panics raised by registered Go constructors, functions, or methods become `HostPanicError` at the reflection boundary. Native bytecode `try`/`catch` can catch these errors exactly like a returned Go error:

```php
try {
    $host->crash();
} catch (Exception $error) {
    echo "caught: " . $error;
}
```

Without a catch, `Runtime.Run` returns the error to Go. The VM also has a last-resort recovery guard for engine defects; ordinary host panics do not rely on that guard.

## Compilation and caching

`Runtime.Run` looks for flat bytecode in the configured `ExprCache`. On a cache miss it compiles the complete AST and stores the immutable bytecode by program identity. Share the cache between runtimes that execute the same parsed program:

```go
cache := flatstack.NewExprCache()

runtime := flatstack.New(output, flatstack.Options{})
runtime.SetExprCache(cache)
runtime.Run(program)
```

For compile-once/run-many workloads, parse the source once and reuse both the `*model.Program` and cache. Re-parsing creates a different program identity and a new flat compilation.

`runner.Precompiler` is that reuse as a setting. It walks the `.php` files of a source tree, parses each into an `IncludeCache` and compiles each into an `ExprCache`, and `runner.Options.Precompile` makes `Runtime.LoadFile` read an entrypoint back out of the include cache, with no second parse. A server that does not is re-parsing every entrypoint per request and paying a flat compilation for each, because the cache keys the bytecode by the AST the parse produced. `phpscript server` runs one pass per site before it accepts a request; see [Precompilation](configuration.md#precompilation).

## Validation and benchmarks

The test suite contains:

- Native flat-bytecode correctness and host-bridge tests
- Runner-fallback and side-effect atomicity tests
- Shared-cache parallel tests and race checks
- Allocation-budget and deep-expression tests
- Opted-in `.phpt` fixtures through both runtime imports
- Compiler-input, malformed-AST, native differential, and fallback differential fuzz targets

Run the normal and race suites with:

```bash
set -a; . ./.env.testing; set +a
go test ./...
go test -race ./flatstack ./runner
```

Run the flatstack benchmarks with:

```bash
go test ./flatstack -run '^$' -bench '^BenchmarkFlatstack' -benchmem
go test ./tests -run '^$' -bench 'BenchmarkGoBindingHTTP' -benchmem
```

The `BenchmarkEngine*` benchmarks run the same Supports-gated programs through both engines as `engine=` sub-benchmarks:

```bash
go test ./flatstack -run '^$' -bench '^BenchmarkEngine' -benchmem -count 6 | tee engines.txt
benchstat -col /engine engines.txt
```

Run individual fuzz targets in isolated jobs:

```bash
go test ./flatstack -run '^$' -fuzz '^FuzzFlatstackCompilerInput$' -fuzztime=30s
go test ./flatstack -run '^$' -fuzz '^FuzzFlatstackDifferential$' -fuzztime=30s
go test ./flatstack -run '^$' -fuzz '^FuzzFlatstackModelAST$' -fuzztime=30s
go test ./tests -run '^$' -fuzz '^FuzzFlatstackImportSwapFallback$' -fuzztime=30s
```

## Remaining work

The highest-value next steps are:

1. Compile included files to bytecode, where the interpreter runs them today.
2. Compile PHP constructors on the native path.
3. Nested `class` declarations at runtime (PHP semantics).
4. Complete exception `finally` semantics on `return`/`throw` and remaining lvalue/cast forms.
5. Add instruction, call-depth, and deadline budgets to native execution.
6. Cache native-rejection decisions and use a structural cache key where callers need to reparse identical source frequently.
7. Let `annotations.Route` and CLI/server entry points select a runtime factory so they can opt into flatstack, where they always construct `runner.New` today.
8. Track native-versus-fallback execution in diagnostics so production users can measure bytecode coverage without calling `Supports` separately.

The run's exec state pools operand, local, iterator, handler and call-frame storage, user-function frames reuse stashed slabs, a call borrows its arguments off the operand stack for its duration, and the frame handle above carries the host locals in place of a copy: a precompiled loop runs at zero allocations per run, and a host call in a loop adds nothing beyond the callee's own work. `TestFlatstackPrecompiledAllocationBudget` fails, not skips, when any of that regresses.

Flatstack is interchangeable as an embedding API and for observable fixture behavior, but not yet a standalone replacement for runner's implementation. Replacing runner with copied or inlined code would be the wrong next step: shared runtime/bridge infrastructure avoids two diverging APIs while the opcode compiler and VM replace execution semantics incrementally.
