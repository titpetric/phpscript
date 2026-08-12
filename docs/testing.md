# Testing phpscript

Run the full test suite from the repository root:

```bash
go test ./...
```

Most language and runtime behavior should be covered by a `.phpt` fixture.
Use a Go test in the package that owns the behavior when the assertion needs
direct access to Go APIs, parser models, runtime state, concurrency, or error
types.

## `.phpt` fixtures

Fixtures live directly in [`tests/fixtures`](../tests/fixtures). The test
harness discovers every file with a `.phpt` extension and runs it through the
default `runner` runtime.

Each fixture has three sections separated by a line containing only `---`:

```text
<YAML metadata>
---
<PHP source>
---
<expected output>
```

For example:

```phpt
name: string concatenation
description: Concatenating two strings produces their combined value.
---
<?php
$greeting = "hello";
echo $greeting . " world";
?>
---
hello world
```

The YAML metadata supports these fields:

| Field | Required | Purpose |
|---|---:|---|
| `name` | yes | Human-readable subtest name. |
| `description` | yes | Behavior and intent covered by the fixture. |
| `error` | no | Substring that must occur in the chain of an uncaught runtime error. |
| `stdin` | no | String exposed to the script through `STDIN`. |
| `flatstack` | no | Set to `true` to run the fixture through flatstack as well. |

The expected-output section is always checked. Trailing newline differences are
ignored, but all other output must match exactly. For an uncaught error, set
`error` to a stable identifying substring and normally expect the host response
body `Internal Server Error`:

```phpt
name: uncaught exception
description: An uncaught exception is returned to the embedding host.
error: boom
---
<?php
throw new Exception("boom");
?>
---
Internal Server Error
```

Files used by `include`, autoloading, templates, or filesystem APIs can be
placed below `tests/fixtures`. The fixture runtime exposes that directory as
its root filesystem, so fixture code refers to those files with paths relative
to it.

## Testing with flatstack

Every `.phpt` fixture runs through the default runtime. To additionally run a
fixture through the native flatstack bytecode runtime, add one metadata field:

```yaml
flatstack: true
```

No Go-side fixture list needs updating. The flatstack harness discovers the
flag automatically. It also calls `flatstack.Supports` before execution, so the
test fails if the program would use the compatibility interpreter instead of
native flat bytecode. Only add the flag after all syntax and runtime operations
used by the fixture are supported by flatstack.

## Running fixture tests

Run all default-runtime fixtures:

```bash
go test ./tests -run '^TestFixtures$'
```

Run all flatstack-enabled fixtures:

```bash
go test ./tests -run '^TestFlatstackFixtures$'
```

Use the fixture name to focus on one subtest:

```bash
go test ./tests -run 'TestFixtures/string_concatenation'
go test ./tests -run 'TestFlatstackFixtures/string_concatenation'
```

Before submitting a change, run the package tests affected by the change and
then the complete suite with `go test ./...`.
