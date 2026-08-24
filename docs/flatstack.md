# Flat-stack runtime

The `flatstack` package is an alternative entry point for embedding phpscript.
It keeps the `runner` API while adding a compile-once flat-bytecode path for a
growing subset of the public `model.Program` AST.

The API is experimental and may change at any point before a stable release,
or be absorbed into the already provided runner APIs.

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

The entire program is checked and compiled before execution. If any node is not
supported, no flat instruction runs and the complete program is delegated to
the runner. This prevents fallback from repeating constructors, method calls,
output, or other side effects.

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

To change only an existing import path while retaining the local identifier
`runner`, use an import alias:

```go
import runner "github.com/titpetric/phpscript/flatstack"
```

Existing calls such as `runner.New`, `runner.Options`, `runner.IsExit`, and
`runner.NewExprCache` then continue to compile.

## API compatibility

The following flatstack types are aliases of their runner counterparts:

- `Runtime` and `Options`
- `Context` and `Scope`
- `ExprCache` and `IncludeCache`
- `ExitError` and `HostPanicError`
- `IncludeFunc` and `Transpiler`

Because `flatstack.Runtime` is an alias of `runner.Runtime`, APIs that accept a
concrete `*runner.Runtime` remain usable. In particular, these calls require no
adapter:

```go
stdlib.Register(runtime)
stdlib.RegisterFS(runtime, ".")
requestContext.Register(runtime)
```

The package also forwards the runner package functions used by embedders:

- `NewContext` and `FromRequest`
- `NewScope` and `ScopeFromContext`
- `NewExprCache` and `NewIncludeCache`
- `NewTranspiler`
- `IsExit`

### Are runner and flatstack interchangeable?

For code that directly constructs and runs a runtime, **yes at the API and
fallback-behavior level**. The full fixture corpus is executed through both
imports in CI-style tests.

They are **not yet independent equivalent engines**:

- `flatstack.Runtime` deliberately aliases `runner.Runtime`; this preserves the
  complete embedding API rather than duplicating runtime, standard-library,
  include, request, and reflection state.
- Only the documented subset below executes as native flat bytecode.
- Every other valid program uses the existing runner interpreter.
- `annotations.Route` and the bundled CLI/server currently construct `runner.New`
  internally. Merely changing a callback type does not make those paths use
  flatstack.
- `Runtime.OnError` selects the interpreter because its per-statement recovery
  contract has not been added to bytecode.

Use `flatstack.Supports` when native bytecode execution is required:

```go
if err := flatstack.Supports(program); err != nil {
	return fmt.Errorf("benchmark would use runner fallback: %w", err)
}
```

Applications normally do not need this check because transparent fallback is
the compatibility contract. Benchmarks should always use it unless they are
intentionally measuring fallback.

## Native bytecode subset

Flat bytecode currently supports these statements:

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

It supports these expressions:

- Scalar literals
- Local variables, runtime globals, and constants through host lookup
- PHP arrays, keyed/unkeyed literals, reads, writes, and append
- Arithmetic (`+`, `-`, `*`, `/`, `%`)
- Comparisons (`==`, `!=`, `===`, `!==`, `<`, `<=`, `>`, `>=`)
- Bitwise operators (`&`, `|`, `^`, `<<`, `>>`)
- Short-circuit logical operators and unary `!`, `+`, `-`, and `~`
- Prefix/postfix increment and decrement of variables and array indexes
- Assignment expressions and full/Elvis ternaries
- Host and PHP object construction with `new`
- Registered/free function calls and method calls
- Property reads
- String concatenation with `.`
- Anonymous functions, including a by-value `use (...)` capture list, a `$this`
  carried away from an enclosing method, and `static function () {}`

The native operations above do not use `expr-lang`; arithmetic, coercion,
comparison, array access, and truthiness are implemented by the flat VM and its
small PHP-semantics host boundary. The bridge uses runner's existing reflection
path for registered Go constructors/functions/methods. `expr-lang` remains in
the compatibility interpreter for unsupported programs.

The current end-to-end corpus result is **14 native and 14 compatibility
fallback fixtures (28 total)**. Both paths pass all fixtures. `Supports` is the
authoritative per-program answer; a fixture count is useful progress evidence,
not a claim that half of the PHP language is implemented.

### Current native barriers

The complete program atomically selects fallback when it contains any currently
unsupported form. The major remaining forms are:

- Property increment/decrement and class-constant / static-property forms
- Invoking a callable held in a value: `$fn(...)`, `$array[0](...)`,
  `$this->handler(...)`. A closure compiles, but only a binding such as
  `usort()` or `call_user_func()` can call one
- By-reference closure captures `use (&$x)`, closure parameter defaults, and
  variadic or by-reference closure parameters
- `defer()`, which registers its callback on the frame that called it
- `try` without a `catch` clause
- Casts
- PHP constructors (`__construct`) still run in the interpreter
- Nested `class` declarations (no-op, same as the interpreter hoist; PHP registers them at runtime)
- Included files still execute through the interpreter
- `include` and `require` both fail the request on a missing file (PHP `include` is a warning)
- A host without Include fails at `opInclude`, after earlier opcodes have run

These are not called "unsupported programs" at the public runtime boundary:
they are valid phpscript programs and execute through runner. "Unsupported" in
a `Supports` error means only "not yet lowerable to native flat bytecode."

## Host calls, errors, and panics

Flat bytecode uses the runner's existing host bridge, including:

- Automatic `context.Context` injection
- PHP-style case-insensitive method lookup
- Argument conversion
- Go constructor and method error propagation
- Exported Go struct field access

Panics raised by registered Go constructors, functions, or methods become
`HostPanicError` at the reflection boundary. Native bytecode `try`/`catch` can
catch these errors exactly like a returned Go error:

```php
try {
    $host->crash();
} catch (Exception $error) {
    echo "caught: " . $error;
}
```

Without a catch, `Runtime.Run` returns the error to Go. The VM also has a
last-resort recovery guard for engine defects; ordinary host panics do not rely
on that guard.

## Compilation and caching

`Runtime.Run` looks for flat bytecode in the configured `ExprCache`. On a cache
miss it compiles the complete AST and stores the immutable bytecode by program
identity. Share the cache between runtimes that execute the same parsed program:

```go
cache := flatstack.NewExprCache()

runtime := flatstack.New(output, flatstack.Options{})
runtime.SetExprCache(cache)
runtime.Run(program)
```

For compile-once/run-many workloads, parse the source once and reuse both the
`*model.Program` and cache. Re-parsing creates a different program identity and
therefore a new flat compilation.

## Validation and benchmarks

The test suite contains:

- Native flat-bytecode correctness and host-bridge tests
- Runner-fallback and side-effect atomicity tests
- Shared-cache parallel tests and race checks
- Allocation-budget and deep-expression tests
- Opted-in `.phpt` fixtures through both runtime imports
- Compiler-input, malformed-AST, native differential, and fallback differential
  fuzz targets

Run the normal and race suites with:

```bash
set -a; . ./.env.testing; set +a
go test ./...
go test -race ./flatstack ./runner
```

Run the flatstack benchmarks with:

```bash
go test ./flatstack -run '^$' -bench '^BenchmarkFlatstack' -benchmem
go test ./tests -run '^$' \
  -bench 'BenchmarkGoBindingHTTP|BenchmarkFlatstackMinitplImportSwap' -benchmem
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

1. Compile included files to bytecode instead of the interpreter.
2. Compile PHP constructors on the native path.
3. Nested `class` declarations at runtime (PHP semantics).
4. Complete exception `finally` semantics on `return`/`throw` and remaining lvalue/cast forms.
5. Add instruction, call-depth, and deadline budgets to native execution.
6. Pool operand/local/iterator storage to reduce per-run allocations.
7. Cache native-rejection decisions and use a structural cache key where
   callers need to reparse identical source frequently.
8. Let `annotations.Route` and CLI/server entry points select a runtime factory so
   they can opt into flatstack instead of always constructing `runner.New`.
9. Track native-versus-fallback execution in diagnostics so production users
   can measure bytecode coverage without calling `Supports` separately.

Flatstack is therefore interchangeable as an embedding API and for observable
fixture behavior, but it is not yet a standalone replacement for runner's
implementation. Replacing runner with copied or inlined code would be the wrong
next step: shared runtime/bridge infrastructure avoids two diverging APIs while
the opcode compiler and VM replace execution semantics incrementally.
