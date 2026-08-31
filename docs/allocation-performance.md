# Allocation performance in bindings

phpscript has no marshalling layer. A registered Go function is invoked by
reflection and whatever it returns is boxed into `any` and handed to the VM,
which dispatches on the dynamic type. `foreach`, `$x[0]`, `$m["key"]`,
`$obj->field` and `$obj->method()` all work against native Go values through
reflection fallbacks.

That freedom is the whole point, and it has one consequence worth writing
down: **a binding pays for the value it builds, not for the type it declares.**
Returning `any` costs nothing. Building a `*model.Array` costs a lot.

This document is the guideline, the reasoning behind it, and a checklist of
every binding in the tree.

## The measured baseline

All numbers from `tests/bindings_test.go` on an Intel N150, Go 1.27. The
"call" benchmarks drive the real reflection return path
(`runner.invokeAny` -> `runner.firstReturn`), so the floor of 2 allocs is
`reflect.Value.Call` itself.

Same five-element list, five representations:

| Return shape                 | B/op | allocs/op | ns/op |
|------------------------------|-----:|----------:|------:|
| `[]string`, no copy          |   48 |         2 |   254 |
| `[]string`                   |  128 |         3 |   328 |
| `any` holding `[]string`     |  144 |         4 |   313 |
| `[]any`                      |  208 |         8 |   469 |
| `*model.Array`               |  408 |        11 |   724 |
| `any` holding `*model.Array` |  424 |        12 |   814 |

Five rows of two columns, the database shape:

| Return shape                     | B/op | allocs/op | ns/op |
|----------------------------------|-----:|----------:|------:|
| `[]map[string]any`               | 1856 |        18 |  1635 |
| `*model.Array` of `*model.Array` | 2648 |        36 |  2750 |

`explode(",", "a,b,c,d,e")`, before and after this guideline was applied:

| Implementation       | B/op | allocs/op | ns/op |
|----------------------|-----:|----------:|------:|
| `[]string` (now)     |  128 |         3 |   412 |
| `*model.Array` (was) |  488 |        12 |   765 |

The `*model.Array` rows are cheaper than they used to be (728 B/13 allocs and
2888 B/38 in an earlier revision of this document) because `model.Array` now
has a list mode (see the audit at the bottom). The ordering of the table is
unchanged: a slice still beats it, and the gap on the nested database shape is
still an order of magnitude in time.

## The rules

**1. Return the Go value you already have.**

```go
// Good: strings.Split already allocated exactly this.
rt.RegisterFunc("explode", func(delim, s string) []string {
	return strings.Split(s, delim)
})

// Bad: same data, 4x the allocations, plus a box per key and per value.
rt.RegisterFunc("explode", func(delim, s string) *model.Array {
	out := model.NewArray()
	for _, part := range strings.Split(s, delim) {
		out.Append(part)
	}
	return out
})
```

**2. Prefer shapes in this order.**

| Shape                          | Cost                                                           | Use for        |
|--------------------------------|----------------------------------------------------------------|----------------|
| `[]T` (`[]string`, `[]Record`) | 1 allocation, no boxing                                        | lists          |
| `map[string]any`               | 1 allocation (+ buckets)                                       | records, rows  |
| `*Struct`                      | 1 allocation, free to box                                      | single objects |
| `[]any`                        | 1 allocation + a box per element                               | mixed lists    |
| `*model.Array`                 | struct + `map[any]any` + key slice + a box per key *and* value | see rule 4     |

**3. `any` versus a concrete return type does not matter.** `firstReturn`
calls `reflect.Value.Interface()` either way. Measured difference for a slice
is one 16-byte allocation; for scalars it is nil (`bind_int` 32 B/2 allocs,
`bind_int_any` 40 B/2 allocs). Declare whichever reads better. Use `any` when
the value is genuinely polymorphic; PHP's `strpos` returning `false|int` is
the honest case.

**4. Return `*model.Array` for exactly three reasons.**

- **The script appends to it.** A Go slice cannot grow through the interface
  value holding it, so `$a[] = "x"` on a returned slice is an error. Element
  writes (`$a[0] = "x"`) and map key writes (`$m["k"] = "x"`, including new
  keys) do work; see `TestBindingCollectionsAreWritableInPlace`.
- **Insertion order is part of the contract.** A Go map re-randomises on every
  `foreach`. If a value is iterated more than once and the output must match,
  it needs an `*model.Array`. This is why `json_decode` returns one for JSON
  objects and why the introspection listings keep theirs.
- **Hybrid int/string keys with PHP's ordering.** Nothing else models it.

Everything else (projections, listings, query results, regex captures) is a
read-only collection the script walks once.

**5. Take arguments as `any`, not `*model.Array`.** A parameter typed
`*model.Array` makes `reflect.Call` panic the moment a script passes a
binding's `[]string`. Read arguments through `model.RangeValues` /
`model.LenValues` / `model.IsCollection` (`model/collection.go`), which handle
`*Array`, slices and maps with typed fast paths.

**6. Presize everything.** `model.NewArraySize(n)`, `make([]any, 0, n)`,
`make(map[string]any, n)`. A five-element `*model.Array` built by `Append`
grows its key slice 1->2->4->8: four allocations before any data. Map size hints
are hints, not capacity, but they still avoid the rehash-and-copy cycle.

**7. Hoist per-call setup to package scope.** Anything built from constants
(a `strings.Replacer`, a compiled `regexp`, a lookup table) must not be
constructed inside the binding closure. The regex shims already cache compiled
patterns (`regexpCache`); `htmlspecialchars` still does not (see the TODO).

## Why the numbers look like this

**Interface boxing is a heap allocation, with one exception.** Converting a
non-pointer value to `any` calls a `runtime.convT*` helper. `convT64` returns a
pointer into the runtime's preallocated `staticuint64s` array for values
0-255 and calls `mallocgc` for anything larger, so boxing the integer `7` is
free and boxing `4096` costs 8 bytes. Pointers, slices headers into existing
backing arrays, and maps box without a fresh allocation for the pointee. This
is why `*model.Array` is expensive twice over: it boxes every key *and* every
value into a `map[any]any`.

**`map[any]any` pays for hashing too.** An interface-keyed map cannot use a
compile-time-specialised hash function; it dispatches through the type
descriptor at runtime. Go 1.24's Swiss Table maps improved lookup and insert
throughput measurably (roughly 20-30% on microbenchmarks) but did not change
that dispatch cost; a concrete key type still beats an interface key.

**`reflect.Value.Call` allocates before your code runs.** It builds a slice for
the results on every call, plus per-call preparation, which is the 2-alloc
floor in the table above. It is a long-standing known cost. The mitigation is
not to micro-optimise the call but to make the *returned value* cheap, and to
avoid re-entering reflection more times than necessary.

**Escape analysis will keep things on the stack if you let it.** A value whose
lifetime the compiler can bound stays on the stack and costs nothing. Returning
it through an interface defeats that, which is unavoidable at the binding
boundary, but everything *inside* the binding is still eligible. Check with
`go build -gcflags=-m ./stdlib` and look for `escapes to heap` / `moved to heap`. Narrow lifetimes, avoid returning pointers to locals you did not need to
allocate, and prefer generics over `any` in internal helpers where the type is
known.

**Where the guidance stops.** Reflection at the boundary is the design; the
project trades some throughput for "any Go function is a PHP function with no
glue". The rules above recover the part of that cost which buys nothing.

Sources: [Stack Allocations and Escape Analysis](https://goperf.dev/01-common-patterns/stack-alloc/),
[Avoiding Interface Boxing](https://goperf.dev/01-common-patterns/interface-boxing/),
[runtime: prevent allocation when converting small ints to interfaces](https://github.com/golang/go/commit/9828c43288a53d3df75b1f73edad0d037a91dff8),
[runtime/iface.go](https://github.com/golang/go/blob/master/src/runtime/iface.go),
[reflect: Call is slow (golang/go#7818)](https://github.com/golang/go/issues/7818),
[Faster Go maps with Swiss Tables](https://go.dev/blog/swisstable),
[Memory Preallocation](https://goperf.dev/01-common-patterns/mem-prealloc/),
[Escape Analysis in Go](https://blog.jetbrains.com/go/2026/07/20/escape-analysis/).

## The bigger lever (fixed)

This section used to say that the size of the function table was the dominant
cost: `runner.baseEnv` rebuilt the expression environment on **every** `Eval`,
allocating one closure per registered function, and roughly 78% of a script's
allocations were that rebuild. The same script against a runtime with the full
stdlib versus one with a single binding registered measured 649 vs 145
allocs/op.

It no longer does. `runner` now:

- pools evaluation environments per `Runtime` (`acquireEnv` / `releaseEnv`) and
  reaches the registered function's scope through a `scopeRef` indirection
  instead of capturing it, so an environment is built once rather than per
  `Eval`;
- populates an environment with the functions an expression actually calls, on
  demand (`Runtime.installFunc`, fed by `Transpiler.Calls`), instead of the
  whole table;
- caches the expr compile configuration per function-table generation
  (`Runtime.exprConfig`) and builds its type-env nature directly
  (`typeEnvNature`) rather than letting expr walk the table reflectively.

| Runtime           | B/op  | allocs/op | ns/op |
|-------------------|------:|----------:|------:|
| full stdlib (was) | 28031 |       649 | 55986 |
| one binding (was) |  3822 |       145 | 14627 |
| full stdlib (now) |   929 |        24 |  5562 |
| one binding (now) |   929 |        24 |  4936 |

The two are now identical: a script pays for the functions it calls, not for
the size of the table it could call from.
`BenchmarkScriptEnvFullStdlib` / `BenchmarkScriptEnvMinimal` measure it.

### The compile-time type env, and why it is still there

The third bullet was the single largest item in the tree once the runtime env
was fixed: `expr.Compile(src, expr.Env(typeEnv), ...)` makes expr walk the
whole ~95-entry type-env map through `reflect.Value.MapKeys` + `MapIndex` +
`copyVal` on **every compile**. That was 64% of all allocations.

The obvious fix, dropping `expr.Env` entirely because PHP is dynamically typed
and the comment above `Runtime.compile` claimed we compiled without type
information anyway, is **wrong, and silently so**. `expr/parser.parseCall`
checks its own `predicates` table *before* the disabled-builtins list, and the
only thing that stops a name being parsed as expr's predicate syntax is
`conf.Config.IsOverridden(name)`, which consults `Config.Env`. PHP's `count`,
`map`, `filter`, `find`, `sum`, `reduce` and `sortBy` all collide.
`expr.DisableAllBuiltins()` does not cover this. With no env, `count($x)`
compiles to expr's `count` predicate instead of the registered PHP function.

So the env stays; what was removed is the per-compile cost of deriving it. The
config is built once per function-table generation, and because every entry is
the same shared stub, one `nature.Nature` is derived and reused for all keys
(`typeEnvNature`) instead of one reflective walk per key.

If you change any of this, `runner/compile_test.go::TestCompileMatchesExprEnv`
is the guard: it compiles a corpus both ways and diffs
`vm.Program.Disassemble()`, so a config change that alters emitted bytecode
fails loudly rather than becoming a subtle interpreter bug.

## How to measure

`tests/bindings.go` defines one binding per return shape; `tests/bindings_test.go`
asserts the semantics and benchmarks the cost at three levels:

```sh
go test ./tests/ -run TestBinding -count=1
go test ./tests/ -run XXX -bench 'BenchmarkConstruct' -benchtime 200000x  # the value alone
go test ./tests/ -run XXX -bench 'BenchmarkCall'      -benchtime 200000x  # + reflection return path
go test ./tests/ -run XXX -bench 'BenchmarkScript'    -benchtime 20000x   # + the VM
```

When changing a shape, keep the old implementation as a second binding
(`bind_explode_legacy` next to `bind_explode_native`) so the benchmark measures
the change instead of asserting it.

## TODO: binding audit

Every registered function and class, its return shape, and whether it is
optimal. Regenerate the function list with
`go run ./scripts/list-apis`; discover Go signatures with
`go doc -short -u ./stdlib` (see also `go doc ./stdlib/database.Database`).

Legend: **OK**: optimal, nothing to do. **OK (by design)**: allocates, but the
allocation buys required semantics. **TODO**: a real improvement is available.

### Arrays

| Binding            | Returns                                                | Status                                                                                                                                                                                                                                               |
|--------------------|--------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `array_column`     | `[]any`, presized, or `*model.Array` with `$index_key` | OK (by design): the keyed form carries hybrid int/string keys (rule 4). Rows are read through `model.RangeValues`, so the `[]map[string]any` of `Database::get_all` needs no conversion                                                              |
| `array_filter`     | `(*model.Array, error)`, presized                      | OK (by design): preserves keys, holes included                                                                                                                                                                                                       |
| `array_flip`       | `*model.Array`, presized                               | OK (by design): the values become keys, normalised through `phpval.Key`                                                                                                                                                                              |
| `array_key_exists` | `bool`                                                 | OK: constant time on `*model.Array` and `map[string]any`, a walk on any other shape                                                                                                                                                                  |
| `array_keys`       | `[]any`, presized                                      | OK                                                                                                                                                                                                                                                   |
| `array_map`        | `([]any, error)`, presized                             | OK                                                                                                                                                                                                                                                   |
| `array_merge`      | `*model.Array`, presized                               | OK (by design): merges string keys. Returning `[]any` for the all-lists case was tried and reverted: once `model.Array` gained list mode it measured 8 allocs either way, and the slice cannot serve `$x = array_merge($a, $b); $x[] = "z"` (rule 4) |
| `array_reduce`     | `(any, error)`                                         | OK: one carry value, no intermediate collection                                                                                                                                                                                                      |
| `array_reverse`    | `[]any`, presized, or `*model.Array` when keys survive | OK (by design): `$preserve_keys`, or any string key, needs ordered keys (rule 4); an all-integer-keyed array renumbers to a plain list                                                                                                               |
| `array_slice`      | `[]any`, exact size                                    | OK                                                                                                                                                                                                                                                   |
| `array_splice`     | `([]any, error)`                                       | OK (by design): resizes its argument, so it requires `*model.Array` and now errors instead of panicking on a slice                                                                                                                                   |
| `array_sum`        | `any` (`int64` or `float64`)                           | OK (by design): PHP's return type, promoting to `float64` at the first float or on overflow                                                                                                                                                          |
| `array_unique`     | `*model.Array`, presized                               | OK (by design): preserves keys                                                                                                                                                                                                                       |
| `array_values`     | `[]any`, presized                                      | OK                                                                                                                                                                                                                                                   |
| `compact`          | `map[string]any`                                       | OK                                                                                                                                                                                                                                                   |
| `count`            | `int64` via `model.LenValues`                          | OK                                                                                                                                                                                                                                                   |
| `in_array`         | `bool`                                                 | OK                                                                                                                                                                                                                                                   |
| `range`            | `[]any`, presized                                      | OK: the element count is computed up front, so a float range neither reallocates nor accumulates rounding error                                                                                                                                      |
| `usort`            | `bool`, sorts `*model.Array` or a Go slice in place    | OK                                                                                                                                                                                                                                                   |

### Array sorting

The key-preserving half of the sort family. `sort`, `rsort` and `usort` only
permute values, so `sortValues` can sort a Go slice in place through its backing
array. These six move the key with the value, which means rebuilding the array:
`arrayEntries` snapshots the pairs, the snapshot is sorted, then `Clear` plus
`arrayReplay` in restore mode writes every pair back with `Set`. `Append` would
hand out fresh integer keys and quietly turn the call into `sort()`.

Cost is one `[]arrayEntry` of `n` pairs per call, plus the map and key slice if
the sorted key order pushes the array out of list mode. Sorting the snapshot
rather than the live storage is deliberate: the rewrite never iterates what it
is overwriting, the same reason `array_shift` snapshots.

All six require a `*model.Array` and error on a native Go slice, following
`arrayTarget` (`array_splice`, `array_shift`). A Go slice has no keys to
preserve, so sorting one would be `sort()` under another name.

| Binding  | Returns                                           | Status                                                                                               |
|----------|---------------------------------------------------|------------------------------------------------------------------------------------------------------|
| `ksort`  | `(bool, error)`, rebuilds `*model.Array` in place | OK (by design): one snapshot of `n` entries, then a `Set` per entry                                  |
| `krsort` | `(bool, error)`, rebuilds `*model.Array` in place | OK (by design): as `ksort`, comparing the other way round                                            |
| `asort`  | `(bool, error)`, rebuilds `*model.Array` in place | OK (by design): as `ksort`, comparing values                                                         |
| `arsort` | `(bool, error)`, rebuilds `*model.Array` in place | OK (by design): as `asort`, comparing the other way round                                            |
| `uasort` | `(bool, error)`, rebuilds `*model.Array` in place | OK (by design): the comparator is resolved once per call with `rt.Callable`, not once per comparison |
| `uksort` | `(bool, error)`, rebuilds `*model.Array` in place | OK (by design): as `uasort`, comparing keys                                                          |

### Strings

| Binding                                                                      | Returns                              | Status                                                                                                                                                                  |
|------------------------------------------------------------------------------|--------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `explode`                                                                    | `[]string` from `strings.Split`      | OK                                                                                                                                                                      |
| `implode`                                                                    | `string`, `[]string` fast path       | OK                                                                                                                                                                      |
| `htmlspecialchars`                                                           | `string`                             | OK: the `strings.Replacer` is now a package-level `var` (was rebuilt per call: 11 allocs -> 2)                                                                          |
| `crc32`                                                                      | `int64`                              | OK: the `[]byte(s)` conversion *did* escape (`crc32.ChecksumIEEE` leaks its argument). Now a package-level table plus a byte-wise loop for inputs ≤ 256 B: 1 alloc -> 0 |
| `sprintf`                                                                    | `string`                             | OK: `fmt` boxes its arguments, but the VM already handed them over as `any`                                                                                             |
| `str_replace`                                                                | `string`                             | OK: reads any collection shape                                                                                                                                          |
| `strlen`, `str_repeat`, `strtoupper`, `strtolower`, `trim`, `ltrim`, `rtrim` | `int64` / `string`                   | OK                                                                                                                                                                      |
| `substr`, `strstr`                                                           | `string` / `any`, subslices, no copy | OK                                                                                                                                                                      |
| `strpos`                                                                     | `any` (`false` or `int64`)           | OK (by design): PHP's return contract; offsets ≥ 256 cost one 8-byte box                                                                                                |
| `stream_get_contents`                                                        | `(string, error)`                    | OK (by design): `io.ReadAll` plus one string copy                                                                                                                       |
| `ucfirst`, `lcfirst`, `ucwords`                                              | `string`                             | OK: byte-wise, and the `[]byte` copy is made only when a byte actually changes                                                                                          |
| `chr`                                                                        | `string`                             | OK: a package-level table of the 256 one-byte strings, so the return is a shared constant: 1 alloc -> 0                                                                 |
| `ord`                                                                        | `int64`                              | OK                                                                                                                                                                      |
| `fnmatch`                                                                    | `bool`                               | OK: the matcher walks both strings in place with one saved backtrack point per star, so it allocates nothing and a run of stars costs the same as one                   |
| `mb_strtolower`, `mb_strtoupper`                                             | `string`                             | OK: `strings.ToLower`/`ToUpper`, which return the input unchanged when no case maps                                                                                     |
| `mb_strlen`                                                                  | `int64`                              | OK: `utf8.RuneCountInString` counts in place, where a `[]rune` conversion would allocate the whole string again                                                         |
| `mb_substr`                                                                  | `string`, subslice, no copy          | OK: the two byte offsets are found by walking the string, again avoiding a `[]rune` round trip                                                                          |

### Math

PHP's numeric return types are the constraint here: `abs`, `pow`, `min` and
`max` hand back an `any` because the type they return is the type they were
given.

| Binding                 | Returns                           | Status                                                                                                                                                                                 |
|-------------------------|-----------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `abs`                   | `any` (`int64` or `float64`)      | OK (by design): PHP's return type follows the argument, decided by `phpval.Number`                                                                                                     |
| `floor`, `ceil`, `sqrt` | `float64`                         | OK                                                                                                                                                                                     |
| `round`                 | `float64`                         | OK (by design): rounds the decimal text of `strconv.FormatFloat(v, 'f', -1, 64)`, one string per call. Rounding the binary value would make `round(1.005, 2)` 1.0 and `round(2.5)` 2.0 |
| `pow`                   | `any` (`int64` or `float64`)      | OK: a squaring loop over `int64`, the same rule the `**` operator follows; only the boxed result allocates                                                                             |
| `log`                   | `float64`                         | OK: base 2 and 10 read through `math.Log2` and `math.Log10`, as PHP does                                                                                                               |
| `min`, `max`            | `any`, the winning element itself | OK: a variadic call compares the `[]any` the VM already built; a single collection argument goes through `phpval.Values`                                                               |
| `number_format`         | `string`                          | OK: one `strings.Builder` grown to the grouped length, over the digit string the rounding helper returns                                                                               |

### Encoding

| Binding                     | Returns                     | Status                                                                                                              |
|-----------------------------|-----------------------------|---------------------------------------------------------------------------------------------------------------------|
| `base64_encode`             | `string`                    | OK: `base64.StdEncoding.EncodeToString`, one buffer                                                                 |
| `base64_decode`             | `any` (`string` or `false`) | OK (by design): PHP's string-or-false contract. The output slice is presized to `len/4*3+3` and decoded in one pass |
| `urlencode`, `rawurlencode` | `string`                    | OK: package-level `[256]bool` tables, one counting pass, then a builder grown to the exact escaped length           |
| `urldecode`, `rawurldecode` | `string`                    | OK: a string with nothing to decode is returned as is, no builder at all                                            |
| `http_build_query`          | `string`                    | OK: reads any collection shape through `model.RangeValues`, one builder grown from `model.LenValues`                |

### JSON

| Binding       | Returns                                        | Status                                                                                                                     |
|---------------|------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------|
| `json_encode` | `(any, error)` holding `string`                | OK: native shapes pass through `jsonEncodeValue` untouched                                                                 |
| `json_decode` | `[]any` for arrays, `*model.Array` for objects | OK (by design): the object case fixes one iteration order for the value's lifetime; a map would re-randomise per `foreach` |

### Regular expressions

| Binding          | Returns                                                         | Status              |
|------------------|-----------------------------------------------------------------|---------------------|
| `preg_match`     | `int64`; `$matches` is the `[]string` from `FindStringSubmatch` | OK: zero conversion |
| `preg_match_all` | `int64`; `$matches` is `[]any` of `[]string`                    | OK                  |
| `preg_replace`   | `string`, compiled patterns cached                              | OK                  |

### Filesystem

| Binding                                             | Returns                       | Status                                                                                                                                                           |
|-----------------------------------------------------|-------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `glob`                                              | `[]string`                    | OK: `fs.Glob`/`filepath.Glob` build the only slice; the error slot went away because PHP answers an empty array, not a throw, for a pattern that matches nothing |
| `file_get_contents`                                 | `any` (`string` or `false`)   | OK (by design): the file contents are the allocation                                                                                                             |
| `file_exists`, `mkdir`, `unlink`, `touch`, `fclose` | `bool`                        | OK                                                                                                                                                               |
| `filemtime`                                         | `int64`                       | OK                                                                                                                                                               |
| `dirname`, `basename`                               | `string`                      | OK                                                                                                                                                               |
| `fopen`                                             | `any` (`*os.File` or `false`) | OK                                                                                                                                                               |
| `fwrite`                                            | `any` (`int64` or `false`)    | OK                                                                                                                                                               |
| `chdir`, `getcwd`                                   | `bool` / `string`             | OK: both read the runtime's own field, and getcwd builds one short string                                                                                        |

### Processes

| Binding              | Returns                     | Status                                                                                                                                                     |
|----------------------|-----------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `exec`               | `any` (`string` or `false`) | OK: `cmd.Output()` allocates the captured stdout, which is the value; `$output` appends into the caller's `*model.Array` rather than building a second one |
| `system`, `passthru` | `any`                       | OK: the process writes straight through to `rt.Output()`, so nothing is buffered; `system` keeps one line rather than the stream                           |
| `shell_exec`         | `any` (`string` or `null`)  | OK: the captured stdout is the value                                                                                                                       |
| `escapeshellarg`     | `string`                    | OK: one `strings.ReplaceAll`, which returns the input unchanged when there is no quote to escape                                                           |
| `escapeshellcmd`     | `string`                    | OK: a presized `strings.Builder`, one pass                                                                                                                 |

### Type checks

| Binding                                                                       | Returns                         | Status |
|-------------------------------------------------------------------------------|---------------------------------|--------|
| `is_array`                                                                    | `bool` via `model.IsCollection` | OK     |
| `is_bool`, `is_int`, `is_numeric`, `is_object`, `is_string`, `isset`, `empty` | `bool`                          | OK     |

### Runtime introspection

| Binding                                                      | Returns                                | Status                                                                                                                                                                         |
|--------------------------------------------------------------|----------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `get_declared_classes`                                       | `[]string`                             | OK                                                                                                                                                                             |
| `get_defined_functions`                                      | `map[string][]string`                  | OK                                                                                                                                                                             |
| `get_included_files`                                         | `[]string` (defensive copy)            | OK                                                                                                                                                                             |
| `get_include_path`, `php_sapi_name`                          | `string`                               | OK                                                                                                                                                                             |
| `get_defined_constants`                                      | `*model.Array`, presized               | OK (by design): sorted listing                                                                                                                                                 |
| `get_defined_vars`                                           | `*model.Array`, presized               | OK (by design): sorted listing                                                                                                                                                 |
| `getallheaders`, `get_all_headers`, `apache_request_headers` | `*model.Array` via `runner.mapToArray` | OK (by design): sorted for determinism; `mapToArray` presizes with `model.NewArraySize`                                                                                        |
| `phpinfo`                                                    | `(bool, error)`                        | OK                                                                                                                                                                             |
| `token_get_all`                                              | `[]any` of `[]any`                     | OK: was `*model.Array` of `*model.Array`. Triples are carved out of chunked backing arrays and ids/lines come from a pre-boxed table: 6003 -> 1828 allocs on a 5.6 KB template |
| `token_name`                                                 | `string`                               | OK                                                                                                                                                                             |

### Language and control

| Binding                                 | Returns                                         | Status                  |
|-----------------------------------------|-------------------------------------------------|-------------------------|
| `call_user_func_array`                  | `(any, error)`, forwards a `[]any` with no copy | OK                      |
| `func_get_args` (env helper)            | `[]any`, the frame's own slice                  | OK: zero allocation     |
| `class_exists`, `spl_autoload_register` | `(bool, error)`                                 | OK                      |
| `spl_autoload`                          | `error`                                         | OK                      |
| `function_exists`                       | `bool` (stub)                                   | OK                      |
| `exit`, `die`                           | `(any, error)`                                  | OK                      |
| `getenv`                                | `any` (`string` or `false`)                     | OK                      |
| `putenv`                                | `bool`                                          | OK                      |
| `set_include_path`                      | `string`                                        | OK                      |
| `header`                                | nothing                                         | OK                      |
| `defer`                                 | `error`                                         | OK                      |
| `register_shutdown_function`            | nothing                                         | OK                      |
| `start_span`                            | `*telemetry.Span`                               | OK: pointer, boxes free |

### Classes

| Binding                                          | Constructor / methods                                                                          | Status                                                                                                                                                                                     |
|--------------------------------------------------|------------------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `Database`                                       | `*Database`; `Get`/`GetAll` return the bridge's `map[string]any` / `[]map[string]any` uncopied | OK: the largest win in the tree. Note the trade: column order is arbitrary *per iteration* rather than fixed per row. A stable order needs an ordered row type upstream in `titpetric/pdo` |
| `Database\Migrate`                               | `*DatabaseMigrate`; `Load`/`Run` return `error`                                                | OK                                                                                                                                                                                         |
| `SMTP`                                           | `(*SMTP, error)`; `Send` returns `error`; options accepted as `any`                            | OK                                                                                                                                                                                         |
| `Session\Manager`                                | `(*SessionManager, error)`; `Get` `(string, error)`, `Valid` `(bool, error)`, `Start` `error`  | OK                                                                                                                                                                                         |
| `Session\Storage\Disk`, `Session\Storage\Memory` | pointers                                                                                       | OK                                                                                                                                                                                         |
| `SharedMemory`                                   | `*SharedMemory`; `Get`/`Count` `string`, `Incr` `int64`, `Has`/`Delete` `bool`                 | OK                                                                                                                                                                                         |
| `Exception`                                      | `(*Exception, error)`                                                                          | OK: was a struct **value**, which boxed a copy and left `$e->message = "x"` failing with "not a writable object property". The pointer costs the same one allocation                       |

### Outside the binding layer

| Item                    | Status                                                                                                                                                                                                                                                                                                                                                                                                                        |
|-------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `runner.baseEnv`        | **Done.** Replaced by pooled environments with on-demand function installation and a cached compile config; see "The bigger lever (fixed)" above                                                                                                                                                                                                                                                                              |
| `parser.TokenGetAll`    | **Done.** See `token_get_all` above                                                                                                                                                                                                                                                                                                                                                                                           |
| `model.Array` internals | **Done.** `Array` has a list mode: while every key is the dense sequence `0..n-1` the values live in a `[]any` and neither the `map[any]any` nor the key slice is allocated. The first key that breaks the invariant promotes it, permanently. A 5-element build went 11 -> 9 allocs, 50 elements 66 -> 57, and `Range` over a list is ~17x faster with zero allocations                                                      |
| `parser` lexer / AST    | **Done.** Operator tokens come from a package-level table of substrings instead of `string(c)` per token, the token slice is presized, and AST nodes are carved out of chunked backing arrays. Parsing a 10.8 KB file went 3197 -> 564 allocs                                                                                                                                                                                 |
| expr compile pipeline   | **Remaining, external.** `runner.compile` is now the largest single block (~48% cumulative): expr's own parser and compiler turning the transpiled source into a `vm.Program`. It is a *cold-start* cost (`ExprCache` means a long-lived runtime pays it once per distinct expression), so it dominates one-shot CLI runs and not servers. Reducing it means emitting fewer or shorter expressions, not micro-optimising expr |
| `reflect.Value.Call`    | **Remaining, by design.** ~43% cumulative. This is the reflection boundary the project trades throughput for; see "Where the guidance stops"                                                                                                                                                                                                                                                                                  |
