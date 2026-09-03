# Go plugins

A binding package is wired in at compile time: it calls `runner.RegisterBinding`
from its `init()`, `stdlib/imports.go` blank-imports it, and adding one means
rebuilding the binary. See [Go bindings](./bindings.md) for that mechanism,
which is the one to use unless there is a reason not to.

A Go plugin is the same registration API delivered at run time. A plugin is a
`package main` built with `-buildmode=plugin`, loaded by filename, that
registers symbols on the runtime serving a request.

## What a plugin has to know first

Go plugins carry constraints that have nothing to do with phpscript, and they
decide whether a plugin is usable at all:

- **The host must be built with cgo.** `plugin` needs a dynamically linked
  binary. The released `phpscript` binary is built with `CGO_ENABLED=0` so that
  it stays a single static file, and it therefore cannot load a plugin: the
  loader reports `plugin.ErrUnsupported` and a caller skips rather than fails.
- **The Go toolchain version must match exactly.** A plugin shares `runtime`,
  `context` and `sync` with its host. A plugin built by a different Go release
  is refused.
- **Any third-party package both link must be the same version.** `plugin.Open`
  fails with `plugin was built with a different version of package X`.
- **A plugin cannot be unloaded.** It stays mapped for the life of the process;
  replacing one means restarting.
- **Linux and macOS only.**

The interface below removes phpscript from that list. It does not remove Go.

## The two entry points

A plugin exports exactly two functions:

```go
func Init(ctx context.Context) error         // once per process
func Bind(ctx context.Context, h Host) error // once per request
```

`Init` runs once per process, for the first runtime that loads the plugin. It
is for setup that is not per-request: opening a connection pool, reading
configuration, priming a cache. It gets no host: the runtime it would be handed
is whichever one happened to load the plugin first.

`Bind` runs on every request, on the runtime serving that request, and installs
the plugin's symbols on it. It runs again for the next request, so it has to be
cheap and it has to be safe to run twice: a re-bind overwrites the same
registrations.

Both are required. A plugin with no setup writes two lines rather than omitting
`Init`, so that a missing symbol stays a mistake instead of an implied default.

The `error`-free forms `func(context.Context)` and
`func(context.Context, Host)` are accepted too.

## Host is declared by the plugin

`Host` is not imported from phpscript. The plugin declares it:

```go
type Host interface {
	RegisterConstructor(name string, ctor any)
	Output() io.Writer
}
```

Go compares interface types structurally and satisfies them structurally, so
`*runner.Runtime` is passed to that parameter without either side naming the
other. The plugin imports `context` and `io` and nothing else.

This is what makes a plugin survive a phpscript rebuild. A plugin that named
`*runner.Runtime` would link `runner`, `model`, `parser` and everything they
import, and would be refused the moment any of them changed.

The loader checks the interface when the plugin is opened, not when it is first
called: a plugin asking for a method the runtime does not have fails at load
with a message naming the method.

Any subset of the runtime's methods can be asked for. These are the useful
ones, and each takes only builtin or standard library types, which is what
makes them safe to name across the boundary:

| Method                                       | Purpose                                             |
|----------------------------------------------|-----------------------------------------------------|
| `RegisterConstructor(name string, ctor any)` | Bind a class name, so `new Name` reaches the plugin |
| `RegisterFunc(name string, fn any)`          | Bind a function name (see the warning below)        |
| `Output() io.Writer`                         | The writer script output goes to                    |
| `SetConst(name string, val any)`             | Define a PHP constant                               |
| `RegisterShutdown(callback any)`             | Run a callback when the script ends                 |

The same trick works for values, not just for the runtime. A PHP array reaches
a binding as phpscript's own array type, which a plugin cannot name; it can
name the method:

```go
type rangeable interface {
	Range(fn func(key, val any) bool)
}
```

## Bind registers constructors, not functions

`RegisterConstructor` writes a map entry.

`RegisterFunc` bumps the runtime's function generation, which invalidates its
expression type environment, its compile configuration and **every pooled
evaluation environment**. Those caches are most of why a call costs what
[PHP to Go call latency](../../php-go-calls.md) says it costs.

A `Bind` that calls `RegisterFunc` therefore throws that away on every request.
Register a class and put the behaviour on its methods, or register the function
from `Init` on a runtime the host supplies once.

## Loading a plugin

In a `.phpt` fixture, with the `plugins` metadata key:

```phpt
name: go plugin replaces the http bindings
description: ...
plugins: ../../testdata/plugins/http/plugin.so
runner:
  php: false
```

The key takes a whitespace-separated string or a list, and a name without a
`.so` suffix names a directory holding `plugin.so`:

```yaml
plugins: ../../testdata/plugins/http/plugin.so ../../testdata/plugins/basic/plugin.so
plugins: [../../testdata/plugins/http, ../../testdata/plugins/basic]
```

A relative name is looked for beside the fixture, then under the module root,
then under the working directory, so one spelling works from `go test`, from
`phpscript test` and from the repository root.

A fixture that loads a plugin has to opt the `php` runner out, because the
classes come from a `.so` the `php` binary knows nothing about. `ParseFixture`
refuses the fixture otherwise rather than leaving it to fail in the matrix.

## Ordering, and replacing a standard library binding

Plugins load and bind **after** `stdlib.Register`, so a plugin registering
`HTTP\Client` replaces the standard library's. Plugins bind in the order they
are named, so a later one wins over an earlier one.

`tests/testdata/plugins/http` is the worked example. It re-registers
`HTTP\Request` and `HTTP\Client`, and with `"log" => true` writes a marker line
into `Output()` that the standard library does not write, which is what
`tests/fixtures/plugins/http_client.phpt` asserts. Deleting the `plugins:` line makes
that fixture fail, which is the point of writing it that way.

Note what replacing costs: the plugin's `Request` and `Client` are its own
types, unrelated to `stdlib/http`'s, so a plugin client cannot send a standard
library request. Replace a family of names together.

Read `Output()` at construction time, not at `Bind` time. `ResetSession` swaps
the runtime's writer between requests and output buffering pushes another on
top of it, so a writer captured during `Bind` writes into the previous
request's buffer.

## Building

```bash
CGO_ENABLED=1 go build -buildmode=plugin -o plugin.so .
```

Pass neither `-trimpath` nor `-ldflags`. A plugin has to match **the binary
that opens it**, and the binaries that can open one (`go test`'s and
`go install .`'s) are built without them. `bin/phpscript-linux-amd64` uses
`-trimpath` but is also `CGO_ENABLED=0`, so it never opens a plugin at all.

`atkins plugins:build` builds the plugins under `tests/testdata/plugins`. The
test harness also rebuilds a plugin whose `.so` is missing or older than a `.go`
file beside it, which is what makes the toolchain match a guarantee rather than
a convention: the plugin and the test binary come from the same `go` in the
same invocation. Set `PHPSCRIPT_PLUGIN_BUILD=0` to use prebuilt artifacts.

`.so` files are not committed.

## What the loader refuses

| Situation                                                  | Result                                     |
|------------------------------------------------------------|--------------------------------------------|
| Host built without cgo                                     | `ErrUnsupported`; callers skip             |
| `Init` or `Bind` missing                                   | `ErrMissingSymbol`, naming which           |
| A signature the loader cannot call                         | `ErrSymbolType`, naming the signature      |
| A `Host` the runtime does not satisfy                      | `ErrSymbolType`, naming the method         |
| `runner.RegisterBinding` called from the plugin's `init()` | `ErrRegisteredBinding`                     |
| Built against a different version of a shared package      | The Go runtime's error, naming the package |

The `RegisterBinding` case is worth explaining. A plugin's `init()` runs inside
`plugin.Open`. One that contributes a process-global binding installs itself
into every runtime built afterwards, which is the opposite of a per-request
`Bind` and would be invisible at the call site. The loader counts the registry
across the load and refuses the plugin.

A panic in `Init` or `Bind` becomes a `runner.HostPanicError`, the same error a
panicking binding produces, rather than unwinding the interpreter.

## A complete plugin

```go
package main

import (
	"context"
	"io"
)

type Host interface {
	RegisterConstructor(name string, ctor any)
}

func Init(ctx context.Context) error { return nil }

func Bind(ctx context.Context, h Host) error {
	h.RegisterConstructor("Greeter", func() *Greeter { return &Greeter{} })
	return nil
}

type Greeter struct{}

func (g *Greeter) Greet(name string) string { return "hello " + name }
```

```php
<?php
$greeter = new Greeter();
echo $greeter->greet("world");
```

Method names are matched case-insensitively and with underscores, the same as
any Go binding: `SetAttribute` is `$x->set_attribute()`. Everything in
[Go bindings](./bindings.md) about constructors, context injection, return
values and thrown errors applies unchanged, including the warning that
registration is not a method allowlist: every exported method and field of the
returned type is reachable from a script.

## Cost

A plugin binding costs the same as a compiled-in one. A plugin symbol is an
ordinary function value once `dlopen` has run, so the per-call cost is the
bridge's, measured in [PHP to Go call latency](../../php-go-calls.md). What a
plugin adds is at load: one `dlopen`, once per process per path.
