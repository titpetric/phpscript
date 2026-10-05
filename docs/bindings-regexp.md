# Regexp bindings

This tutorial is a step by step process on how bindings are added to phpscript. The objective here is to bind "regexp" package APIs to a custom 'Regexp\' php namespace, like http.ResponseWriter is bound to HTTP\ResponseWriter and so on. Basically we bind `regexp.Compile` as `Regexp\compile` in phpscript, implicitly handle the error return as phpscript does and use `$rx = new Regexp\Compile(string $expr);` returning a `*regexp.Regexp` as a phpscript alias `Regexp\Regexp` type. Also add CompilePOSIX, omit MustCompile. All type methods are forwarded to phpscript. It is a new API specific to the phpscript/go runtime and carries no compatibility with vanilla php.

Everything below runs in phpscript today. The fixtures are quoted from `tests/fixtures/bindings/` verbatim, expected output included, and they are what `phpscript test --matrix ./tests/...` checks on both engines.

The reference for the mechanism is [Go bindings](reference/extensions/bindings.md); this page is the walk through one area, start to finish. For what the `preg_*` family does instead, and where PCRE and RE2 part company, see [Regular expressions](reference/extensions/regexp.md).

## What the binding is

Two constructors, and nothing else registered:

```php
$rx = new Regexp\Compile('(\w+)@(\w+)\.com');
$posix = new Regexp\CompilePOSIX('a|ab');
```

Each returns a `*regexp.Regexp`, so every exported method of that type is callable from PHP without a second registration. `$rx->find_string($s)`, `$rx->split($s, -1)`, `$rx->replace_all_string($s, $repl)`, `$rx->num_subexp()` all work because reflection dispatches on the value, not on a method allowlist.

## Step 1: the package

An area a script calls is a package under `stdlib/`, one per area. `stdlib/regexp/regexp.go` is the whole implementation:

```go
// Package regexp implements Regexp\Compile and Regexp\CompilePOSIX, Go's
// regexp package reached as PHP classes. A script that wants RE2 itself rather
// than the PCRE surface preg_* presents uses these; docs/bindings-regexp.md
// walks the binding from here to its fixtures.
//
// The constructors return *regexp.Regexp itself rather than a facade. Every
// exported method of that type is a regexp operation, so publishing the whole
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
	runner.RegisterBinding(Register)
}

// Register installs the regexp compilation classes.
func Register(rt *runner.Runtime) {
	// Regexp\Compile compiles $expr as an RE2 expression and throws when it does not parse; the value it builds carries every method Go's regexp.Regexp has.
	rt.RegisterConstructor("Regexp\\Compile", stdregexp.Compile)
	// Regexp\CompilePOSIX compiles $expr as POSIX ERE, where a match is the leftmost-longest one rather than the leftmost one Perl syntax finds.
	rt.RegisterConstructor("Regexp\\CompilePOSIX", stdregexp.CompilePOSIX)
}
```

Four things in that file are the convention rather than taste:

- A namespace is a backslash in a Go string. `"Regexp\\Compile"` is the name a script types, and lookup is case-insensitive with a leading `\` stripped, so `new \Regexp\compile($e)` resolves the same registration.
- `regexp.Compile` is registered as-is. Its signature is `func(string) (*Regexp, error)`, which is the shape the bridge wants: a return slot declared exactly `error` is omitted from what PHP sees, and a non-nil value in it is thrown.
- The doc comment above each registration is what the generated reference publishes. It starts with the name a script types and ends with punctuation; `internal/apidoc` reads the registration site, not the Go function.
- The import is aliased. A file in `package regexp` may import `"regexp"` and the identifier resolves to the import, but `stdregexp` says which one a reader is looking at, as `stdlib/http` writes `nethttp "net/http"`.

`MustCompile` and `MustCompilePOSIX` are left out: they panic instead of returning an error, and the error is what a script needs to catch. `regexp.Match`, `regexp.MatchString` and `regexp.QuoteMeta` are package functions, not methods, so they would each need their own `RegisterFunc`; they are not part of this area.

## Step 2: wiring it in

A binding package contributes itself through `init`, so importing it is the whole of the wiring. `stdlib/imports.go` blank-imports the standard set:

```go
	_ "github.com/titpetric/phpscript/stdlib/pexec"
	_ "github.com/titpetric/phpscript/stdlib/regexp"
	_ "github.com/titpetric/phpscript/stdlib/session"
```

`stdlib.Register` runs every contributed installer on every runtime it sets up, the interpreter's and the bytecode engine's alike. There is nothing to do for flatstack: `new` compiles to `opConstruct` and `->method()` to `opCallMethod`, and both delegate to the same `helperNew` and `callGoMethod` the interpreter calls, so a registration works on both engines or on neither.

## Step 3: what a script sees

```php
$rx = new Regexp\Compile('(\w+)@(\w+)\.com');
echo $rx->find_string("mail tit@example.com now");   // tit@example.com
```

Four behaviours are worth stating rather than discovering:

- **The class name is the Go type name.** `get_class($rx)` is `Regexp` and `$rx instanceof Regexp` is true; `$rx instanceof Regexp\Regexp` is false. The runtime takes a host-backed value's class from the Go type behind it, with the pointer stripped, so `HTTP\Request` answers `Request` for the same reason. `Regexp\Regexp` is the name this document uses for the type; the Go type is named `Regexp` so that the name a script can test for is the right one.
- **Method names are the Go names as PHP spells them.** Lookup tries the exact name, then case-insensitively, then with underscores removed, so `FindAllStringSubmatch` is `find_all_string_submatch` and `NumSubexp` is `num_subexp`.
- **A bad pattern is a throw.** It is the error `regexp.Compile` returned, and it belongs to no PHP class, so every `catch` clause takes it and `getMessage()` is Go's text.
- **Two Go results arrive as a PHP list.** `LiteralPrefix` returns `(string, bool)`, so `list($prefix, $complete) = $rx->literal_prefix();` reads both. Only a slot declared exactly `error` is removed.

`regexp.Regexp` also implements `fmt.Stringer`, which the runtime uses for any Go value in a string context, so `echo $rx` prints the pattern.

## Step 4: the half of the method set declared over bytes

`regexp.Regexp` has 24 matching methods, and half of them are declared over `[]byte` rather than `string`: `Find`, `FindSubmatch`, `FindAllSubmatch`, `ReplaceAll`. Before this binding a `[]byte` reaching PHP was neither a string nor a usable value - `echo` printed nothing, `strlen` measured zero, and a byte slice reached the reflection fallback as a slice, so `count()` answered a byte count and `foreach` yielded integers.

The rule the runtime now applies is one sentence: **a Go `[]byte` is a PHP string.** PHP's strings are byte strings, which is the same claim the generated reference was already making by publishing a `[]byte` return as `string`. `phpval.Bytes` is the test, and it is asked at every point a byte slice would otherwise read as a list:

| Reads a `[]byte` as           | Where                                                                                                                                                     |
|-------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------|
| its text, in a string context | `phpval.GoString`, which backs `echo`, `implode`, `var_dump`, `print_r` and the string-parameter coercion `strlen` uses                                   |
| not a collection              | `model.IsCollection`, `model.LenValues`, `model.RangeValues`, so `is_array` is false, `count()` applies its scalar rule and `foreach` iterates zero times |
| the string's truth value      | `phpTruthy`, so `""` and `"0"` are falsey                                                                                                                 |
| identical to an equal string  | `phpIdentical`, so `$rx->find($s) === "a@b.com"` holds                                                                                                    |
| a character offset            | `helperIndex`, so `$found[0]` is `"a"` rather than `97`                                                                                                   |
| `string`                      | `phpDebugType`, `gettype` and `is_string`                                                                                                                 |

A nested result works through the same rule without a conversion of its own: `FindAllSubmatch` returns `[][][]byte`, `foreach` walks the two outer slices, and each innermost element is a byte slice that reads as its text.

## Step 5: the fixtures

A change to runtime behaviour lands with a fixture. These bindings have no PHP counterpart, so the fixture is the definition of the behaviour rather than a check against `php`, and each one says so in its `description` and opts the `php` runner out. Both Go engines still run them.

`tests/fixtures/bindings/regexp_compile.phpt`:

```
name: regexp compilation and the string method set
description: >
  Regexp\Compile and Regexp\CompilePOSIX are host bindings with no PHP
  counterpart, so the expected output is the runtime's contract rather than
  PHP's. The value is a *regexp.Regexp, so the class a script sees is Regexp,
  a compile failure arrives as the error regexp.Compile returned, and
  literal_prefix()'s two Go results arrive as a PHP list.
runner:
  php: false
---
<?php

$rx = new Regexp\Compile('(\w+)@(\w+)\.com');

echo get_class($rx), "\n";
echo $rx instanceof Regexp ? "instance\n" : "not an instance\n";

// String() is the pattern, so the value renders as itself.
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
  is a byte string, so the runtime reads a binding's []byte as a string rather
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

`tests/fixtures/bindings/regexp_callback.phpt`. `ReplaceAllStringFunc` declares `func(string) string`, not the uniform `func(...any) (any, error)` a binding usually takes a callable as, so the argument boundary converts a PHP callable into the Go function type the binding asked for:

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

A Go function type that declares no error slot leaves a callback nowhere to report one, so an error a PHP callback raises crosses the intervening Go frames as a panic and the host boundary unwraps it back into the error a script threw. That is why the last case catches `no replacement for a@b.com` rather than a host panic.

Run them:

```bash
go install .
phpscript test --matrix -v ./tests/fixtures/bindings/...
```

`go install .` is not optional. `phpscript test` runs the binary on `PATH`, so an edited runtime that has not been installed is tested in its previous state.

## Step 6: the Go test

The fixtures state the behaviour; the package test states that both engines produce it. `stdlib/regexp/regexp_test.go` runs each case on `runner.New` and `flatstack.New` and fails when the two disagree:

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

The file is named after the source it covers, which `splint`'s pairing linter reads: a `_test.go` with no same-named source beside it fails the lint gate.

One trap this test exists to catch. The fixture harness compiles a program through the flat compiler first and reports the fixture as SKIP for that column when the compiler rejects it, so a fixture using syntax the bytecode engine does not compile - `use (&$x)` is one - passes the matrix while testing one engine. A fixture claiming to cover flatstack has to avoid that syntax, and the package test has no such escape.

## Step 7: regenerate and gate

The reference inventory is generated from the live runtime and the registration comments, so a new area regenerates it:

```bash
atkins test:introspection    # implemented-apis.md, phpscript info, reference README.md
atkins                       # the default pipeline: format, test, matrix, docs, image
```

`atkins` rewrites the two generated documents under `docs/reference/`, so a new registration shows up there and is committed. It does not write `docs/test-fixtures.md` or `docs/coverage/`: those are gitignored and come from `atkins gen`, which is not part of the default target either, so a new package leaves them and `docs/assets/*.svg` stale until it is run by hand.

The generated class entry lists the method set `*regexp.Regexp` published, which is the whole of it minus the six names `apidoc` treats as host plumbing (`Error`, `String`, `GoString`, `SetID`, `MarshalJSON`, `UnmarshalJSON`). Those are hidden from the reference and still callable.

## What publishing a raw Go type costs

`stdlib/http` does the opposite of this binding. Each of its types holds its `net/http` value in an unexported field, because registration is not a method allowlist and embedding `*http.Request` would publish the whole of `net/http` as methods on `HTTP\Request`.

`regexp.Regexp` is published raw because its 40 exported methods are regexp operations and nothing a script should not reach. Five of them do something other than match, and are reachable: `Longest()` reconfigures the compiled expression in place, `Copy()` duplicates it, and `MarshalText`/`UnmarshalText`/`AppendText` are the encoding interfaces. A type with exports beyond its subject gets a facade instead; this one has none.
