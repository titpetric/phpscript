# Regexp bindings

This tutorial walks through adding a binding to phpscript, step by step. The goal binds the "regexp" package APIs to a custom `Regexp\` PHP namespace, as phpscript binds `http.ResponseWriter` to `HTTP\ResponseWriter`. It binds `regexp.Compile` as `Regexp\compile`, handles the error return the way phpscript does. It uses `$rx = new Regexp\Compile(string $expr);`. The call returns a `*regexp.Regexp` as a phpscript alias `Regexp\Regexp` type. Add CompilePOSIX and omit MustCompile. The binding forwards every type method to phpscript. This API is new and specific to the phpscript/go runtime, with no vanilla php counterpart.

Everything below runs in phpscript today. This page quotes the fixtures from `tests/fixtures/bindings/` verbatim, expected output included, and `phpscript test --matrix ./tests/...` checks both engines against them.

The reference for the mechanism is [Go bindings](reference/extensions/bindings.md). The walk below covers one area, start to finish. The `preg_*` family behaves differently, and PCRE parts company with RE2. See [Regular expressions](reference/extensions/regexp.md).

## What the binding is

Two constructors, no others:

```php
$rx = new Regexp\Compile('(\w+)@(\w+)\.com');
$posix = new Regexp\CompilePOSIX('a|ab');
```

Each returns a `*regexp.Regexp`. Reflection makes every exported method of that type callable from PHP without a second registration. `$rx->find_string($s)`, `$rx->split($s, -1)`, `$rx->replace_all_string($s, $repl)`, `$rx->num_subexp()` all work because reflection dispatches on the value, not on a method allowlist.

## Step 1: the package

An area a script calls gets a package under `stdlib/`. `stdlib/ps/regexp/regexp.go` is the whole implementation:

```go
// Package regexp implements Regexp\Compile and Regexp\CompilePOSIX, Go's
// regexp package reached as PHP classes. A script that wants RE2 itself rather
// than the PCRE surface preg_* presents uses these; docs/bindings-regexp.md
// walks the binding from here to its fixtures.
//
// The constructors return *regexp.Regexp itself rather than a facade. Every
// exported method of that type is a regexp operation and publishing the whole
// method set is the binding: $rx->find_string($s), $rx->split($s, -1),
// $rx->replace_all_string($s, $repl). The type is named Regexp, which is the
// class a script sees for the value and the name instanceof compares against.
//
// It is wired in by importing it; stdlib/imports.go does that.
package regexp

import (
	stdregexp "regexp"

	"github.com/titpetric/phpscript/runner"
)

// init contributes the regexp bindings to stdlib.Register.
func init() {
	runner.RegisterArea(`Runtime\PS\Regexp`, Register)
}

// Register installs the regexp compilation classes.
func Register(rt *runner.Runtime) {
	// Regexp\Compile compiles $expr as an RE2 expression and throws when it does not parse; the value it builds carries every method Go's regexp.Regexp has.
	rt.RegisterConstructor("Regexp\\Compile", stdregexp.Compile)
	// Regexp\CompilePOSIX compiles $expr as POSIX ERE, where a match is the leftmost-longest one rather than the leftmost one Perl syntax finds.
	rt.RegisterConstructor("Regexp\\CompilePOSIX", stdregexp.CompilePOSIX)
}
```

Four things in that file are the convention:

- A namespace is a backslash inside a Go string. `"Regexp\\Compile"` is the name a script types. Lookup ignores case and strips a leading `\`. Thus `new \Regexp\compile($e)` resolves the same registration.
- The registration takes `regexp.Compile` as-is. Its signature is `func(string) (*Regexp, error)`, the shape the bridge wants: the bridge omits a return slot declared exactly `error` from what PHP sees, and throws a non-nil value in that slot.
- The doc comment above each registration feeds the generated reference. It starts with the name a script types and ends with punctuation. `internal/apidoc` reads the registration site. It ignores the Go function.
- The import takes an alias. A file in `package regexp` may import `"regexp"` and the identifier resolves to the import, but `stdregexp` says which type a reader sees, as `stdlib/ps/http` writes `nethttp "net/http"`.

The binding omits `MustCompile` and `MustCompilePOSIX`: they panic where a script needs an error to catch. `regexp.Match`, `regexp.MatchString` and `regexp.QuoteMeta` are package functions, each needing its own `RegisterFunc`, and not part of this area.

## Step 2: wiring it in

A binding package contributes itself through `init` and the import is the whole of the wiring. `stdlib/imports.go` blank-imports the standard set:

```go
	_ "github.com/titpetric/phpscript/stdlib/ps/pexec"
	_ "github.com/titpetric/phpscript/stdlib/ps/regexp"
	_ "github.com/titpetric/phpscript/stdlib/ps/session"
```

`stdlib.Register` runs every contributed installer on each runtime it sets up, the interpreter's and the bytecode engine's alike. Flatstack needs no work: `new` compiles to `opConstruct` and `->method()` to `opCallMethod`. Both delegate to the same `helperNew` and `callGoMethod` the interpreter calls. A registration works on both engines or on neither.

## Step 3: what a script sees

```php
$rx = new Regexp\Compile('(\w+)@(\w+)\.com');
echo $rx->find_string("mail tit@example.com now");   // tit@example.com
```

Four behaviours, stated here so nobody hunts for them:

- **The class name is the Go type name.** `get_class($rx)` is `Regexp`, `$rx instanceof Regexp` is true, and `$rx instanceof Regexp\Regexp` is false. The runtime takes a host-backed value's class from the Go type behind it, with the pointer stripped and `HTTP\Request` answers `Request` for the same reason. `Regexp\Regexp` is the name this document uses for the type. The Go type's name is `Regexp` so that the name a script can test for is the right one.
- **Method names are the Go names as PHP spells them.** Lookup tries the exact name, then case-insensitively, then with underscores removed and `FindAllStringSubmatch` is `find_all_string_submatch` and `NumSubexp` is `num_subexp`.
- **A bad pattern is a throw.** The throwable is the error `regexp.Compile` returned. It belongs to no PHP class. Every `catch` clause takes it, with `getMessage()` as Go's text.
- **Two Go results arrive as a PHP list.** `LiteralPrefix` returns `(string, bool)` and `list($prefix, $complete) = $rx->literal_prefix();` reads both. The bridge removes only a slot declared exactly `error`.

`regexp.Regexp` also implements `fmt.Stringer`, which the runtime uses for any Go value in a string context and `echo $rx` prints the pattern.

## Step 4: the half of the method set declared over bytes

`regexp.Regexp` has 24 matching methods, and the `regexp` package declares half of them over `[]byte` where the others take `string`: `Find`, `FindSubmatch`, `FindAllSubmatch`, `ReplaceAll`. Without the rule below, a `[]byte` reaching PHP is neither a string nor a usable value. `echo` printed nothing. `strlen` measured zero. A byte slice reached the reflection fallback as a slice. `count()` answered a byte count, and `foreach` yielded integers.

The runtime now applies one rule: **a Go `[]byte` is a PHP string.** PHP's strings are byte strings, and the generated reference makes the same claim by publishing a `[]byte` return as `string`. `phpval.Bytes` is the test, and the runtime calls it at every point a byte slice would otherwise read as a list:

| Reads a `[]byte` as           | Where                                                                                                                                                                 |
|-------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| its text, in a string context | `phpval.GoString`, which backs `echo`, `implode`, `var_dump`, `print_r` and the string-parameter coercion `strlen` uses                                               |
| not a collection              | `modelutil.IsCollection`, `modelutil.LenValues`, `modelutil.RangeValues` and `is_array` is false, `count()` applies its scalar rule and `foreach` iterates zero times |
| the string's truth value      | `phpTruthy` and `""` and `"0"` are falsey                                                                                                                             |
| identical to an equal string  | `phpIdentical` and `$rx->find($s) === "a@b.com"` holds                                                                                                                |
| a character offset            | `helperIndex` and `$found[0]` is `"a"` rather than `97`                                                                                                               |
| `string`                      | `phpDebugType`, `gettype` and `is_string`                                                                                                                             |

A nested result works through the same rule without a separate conversion: `FindAllSubmatch` returns `[][][]byte`, `foreach` walks the two outer slices. Each innermost element is a byte slice that reads as its text.

## Step 5: the fixtures

A change to runtime behaviour lands with a fixture. With no PHP counterpart, the fixture defines the behaviour. There is no `php` to check against. Each says so in the `description` and opts the `php` runner out. Both Go engines still run them.

`tests/fixtures/bindings/regexp_compile.phpt`:

```
name: regexp compilation and the string method set
description: >
  Regexp\Compile and Regexp\CompilePOSIX are host bindings with no PHP
  counterpart and the expected output is the runtime's contract rather than
  PHP's. The value is a *regexp.Regexp and the class a script sees is Regexp,
  a compile failure arrives as the error regexp.Compile returned, and
  literal_prefix()'s two Go results arrive as a PHP list.
runner:
  php: false
---
<?php

$rx = new Regexp\Compile('(\w+)@(\w+)\.com');

echo get_class($rx), "\n";
echo $rx instanceof Regexp ? "instance\n" : "not an instance\n";

// String() is the pattern and the value renders as itself.
echo $rx, "\n";
echo $rx->num_subexp(), "\n";

echo $rx->match_string("tit@example.com") ? "match\n" : "no match\n";
echo $rx->find_string("mail tit@example.com now"), "\n";
echo implode("|", $rx->find_string_submatch("mail tit@example.com now")), "\n";
echo $rx->replace_all_string("a@b.com c@d.com", "<$1>"), "\n";

// Two Go results arrive as a list, not as the first one.
list($prefix, $complete) = (new Regexp\Compile('abc\d+'))->literal_prefix();
echo $prefix, ":", $complete ? "complete" : "partial", "\n";

// Named groups are numbered and named, which preg_match does not report.
$named = new Regexp\Compile('(?P<user>\w+)@(?P<host>\w+)');
echo implode(",", $named->subexp_names()), "\n";
echo $named->subexp_index("host"), "\n";

// POSIX takes the leftmost-longest match where Perl syntax takes the first.
echo (new Regexp\Compile('a|ab'))->find_string("xabz"), "\n";
echo (new Regexp\CompilePOSIX('a|ab'))->find_string("xabz"), "\n";

echo implode("/", (new Regexp\Compile(',\s*'))->split("a, b,c", -1)), "\n";

try {
	new Regexp\Compile('(unclosed');
	echo "compiled\n";
} catch (Exception $e) {
	echo $e->getMessage(), "\n";
}
---
Regexp
instance
(\w+)@(\w+)\.com
2
match
tit@example.com
tit@example.com|tit|example
<a> <c>
abc:partial
,user,host
2
a
ab
a/b/c
error parsing regexp: missing closing ): `(unclosed`
```

`tests/fixtures/bindings/regexp_bytes.phpt`:

```
name: a regexp method returning bytes answers a string
description: >
  Half of regexp.Regexp's method set is declared over []byte, and a PHP string
  is a byte string and the runtime reads a binding's []byte as a string rather
  than as a list of integers. The expected output is the runtime's contract:
  these are host bindings with no PHP counterpart.
runner:
  php: false
---
<?php

$rx = new Regexp\Compile('(\w+)@(\w+)\.com');
$subject = "mail a@b.com and c@d.com now";

$found = $rx->find($subject);
echo $found, "\n";
echo strlen($found), "\n";
echo gettype($found), "\n";
echo is_string($found) ? "string" : "not a string", "\n";
echo is_array($found) ? "array" : "not an array", "\n";
echo $found === "a@b.com" ? "identical" : "different", "\n";
echo $found[0], $found[1], $found[2], "\n";

// A submatch is a list of byte strings, and each element reads as its text.
echo implode("|", $rx->find_submatch($subject)), "\n";
echo count($rx->find_submatch($subject)), "\n";

$pairs = array();
foreach ($rx->find_all_submatch($subject, -1) as $i => $match) {
	foreach ($match as $group => $text) {
		$pairs[] = $i . "." . $group . "=" . $text;
	}
}
echo implode(" ", $pairs), "\n";

echo $rx->replace_all($subject, "<$2>"), "\n";

// No match is the nil slice, which reads as the empty string and is falsey.
$missing = $rx->find("nothing here");
echo "[", $missing, "]\n";
echo $missing ? "truthy" : "falsey", "\n";
echo strlen($missing), "\n";
---
a@b.com
7
string
string
not an array
identical
a@b
a@b.com|a|b
3
0.0=a@b.com 0.1=a 0.2=b 1.0=c@d.com 1.1=c 1.2=d
mail <b> and <d> now
[]
falsey
0
```

`tests/fixtures/bindings/regexp_callback.phpt`. `ReplaceAllStringFunc` declares `func(string) string`, not the uniform `func(...any) (any, error)` a binding takes a callable as by convention and the argument boundary converts a PHP callable into the Go function type the binding declares:

```
name: a closure reaches a regexp method declaring a Go callback
description: >
  ReplaceAllStringFunc takes a func(string) string, not the uniform callable
  shape, and a PHP closure is converted to it at the argument boundary. The
  expected output is the runtime's contract: Regexp\Compile is a host binding
  with no PHP counterpart. A closure that throws reaches the caller's catch,
  because an error raised inside a Go callback crosses the boundary as a panic
  the host turns back into a throwable.
runner:
  php: false
---
<?php

$rx = new Regexp\Compile('\w+@\w+\.com');
$subject = "mail a@b.com and c@d.com now";

echo $rx->replace_all_string_func($subject, function ($match) {
	return strtoupper($match);
}), "\n";

// A declared function reaches it by name, as it does preg_replace_callback.
function redact($match) {
	return str_repeat("*", strlen($match));
}
echo $rx->replace_all_string_func($subject, "redact"), "\n";

// The byte-slice half of the pair takes func([]byte) []byte, and a closure
// returning a string fills it.
echo $rx->replace_all_func($subject, function ($match) {
	return "<" . $match . ">";
}), "\n";

try {
	$rx->replace_all_string_func($subject, function ($match) {
		throw new Exception("no replacement for " . $match);
	});
	echo "replaced\n";
} catch (Exception $e) {
	echo $e->getMessage(), "\n";
}
---
mail A@B.COM and C@D.COM now
mail ******* and ******* now
mail <a@b.com> and <c@d.com> now
no replacement for a@b.com
```

A Go function type that declares no error slot leaves a callback nowhere to report one. Any error a PHP callback raises crosses the intervening Go frames as a panic, and the host boundary unwraps it back into the error a script threw. That is why the last case catches `no replacement for a@b.com`. A host panic never reaches the script.

Run them:

```bash
go install .
phpscript test --matrix -v ./tests/fixtures/bindings/...
```

`go install .` first. `phpscript test` runs the binary on `PATH`. A run against an edited runtime nobody installs tests the state of its last install.

## Step 6: the Go test

The fixtures state the behaviour, which the package test confirms for both engines. `stdlib/ps/regexp/regexp_test.go` runs each case against both engines and fails when the two disagree:

```go
func run(t *testing.T, src string) string {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var interpreted, flat strings.Builder
	for _, engine := range []struct {
		name string
		out  *strings.Builder
		rt   *runner.Runtime
	}{
		{name: "runtime", out: &interpreted, rt: runner.New(&interpreted, runner.Options{})},
		{name: "flatstack", out: &flat, rt: flatstack.New(&flat, flatstack.Options{})},
	} {
		stdlib.Register(engine.rt)
		if err := engine.rt.Run(prog); err != nil {
			t.Fatalf("%s: run: %v (output %q)", engine.name, err, engine.out.String())
		}
	}
	if interpreted.String() != flat.String() {
		t.Fatalf("engines disagree: runtime %q, flatstack %q", interpreted.String(), flat.String())
	}
	return interpreted.String()
}
```

The file takes its name from the source it covers, which `splint`'s pairing linter reads: a `_test.go` requires a same-named source in that directory, or the lint gate fails.

One trap this test exists to catch. The fixture harness compiles a program through the flat compiler first and reports the fixture as SKIP for that column when the compiler rejects the program. A fixture using syntax the bytecode engine does not compile - `use (&$x)` is an example - passes the matrix while testing one engine. A fixture claiming to cover flatstack must avoid that syntax. The package test has no such escape.

## Step 7: regenerate and gate

The reference inventory comes from the live runtime and the registration comments. A new area regenerates it:

```bash
atkins test:introspection    # implemented-apis.md, phpscript info, reference README.md
atkins                       # the fast gate: format, test, generated docs
```

`atkins` rewrites the two generated documents in `docs/reference/`. A new registration shows up there. It goes into the commit. It does not write `docs/test-fixtures.md` or `docs/coverage/`: `.gitignore` lists those, which `atkins gen` writes outside the default target. A new package leaves those files stale, along with `docs/assets/*.svg`, until someone runs `atkins gen` by hand.

The generated class entry lists the method set `*regexp.Regexp` published: every method minus the six names `apidoc` treats as host plumbing (`Error`, `String`, `GoString`, `SetID`, `MarshalJSON`, `UnmarshalJSON`). `apidoc` hides those from the reference. A script still calls them.

## What publishing a raw Go type costs

`stdlib/ps/http` does the opposite of this binding. Each of its types holds its `net/http` value in an unexported field, because registration is not a method allowlist and embedding `*http.Request` would publish the whole of `net/http` as methods on `HTTP\Request`.

This binding publishes `regexp.Regexp` raw because its 40 exported methods are regexp operations and nothing a script should not reach. Five of them do something other than match, and are reachable: `Longest()` reconfigures the compiled expression in place. `Copy()` duplicates the expression, and `MarshalText`/`UnmarshalText`/`AppendText` are the encoding interfaces. A type with exports beyond its subject gets a facade instead. Every export here is a regexp operation.
