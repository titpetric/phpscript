# Design decisions

Decisions about what phpscript is, and what it will not become. They are here so
that a change proposing one of them again starts from the reasoning rather than
from scratch. [Known divergences from PHP](README.md#known-divergences-from-php)
lists what a script sees where the two differ; this page says why.

## No OOP

phpscript runs classes as namespaced bags of properties and methods. It has no
object model beyond that, and it is not getting one.

What exists:

- `class` declarations with properties, methods and class constants
- `__construct`, `$this`, `__invoke`
- static properties and methods, `self::` and `static::`, including the
  variable spelling `Class::$m()`
- function-level `static $x` variables, persistent per function and per
  closure value
- `Class::class`
- `interface` declarations and `implements`, as a contract check and nothing
  more; see below

What does not, and will not:

- inheritance. A class gets no members from any other class: no inherited
  methods, no inherited properties or constants, no `parent::`, no constructor
  to fall back on.
- traits
- `abstract`, `final` and `readonly` as anything but parsed modifiers
- magic methods beyond `__construct` and `__invoke`

`extends` is parsed and recorded on `model.ClassDecl` so the formatter prints
back the file it read and the linter can see the declaration. Nothing in
`runner` may read it. **Implementing `extends` is out of bounds.** It was
implemented once, in the throwable hierarchy, and removed;
`runner.TestNoInheritanceAtRuntime` fails the build if `model.Class` regrows a
`Parent` field.

Write composition instead, and declare the members a class uses. A class that
calls something it did not declare fails at the call, naming the class and the
method.

### Interfaces are a contract, not a parent

An interface names method signatures. A class that says `implements` must
declare every one of them itself, and that is the whole of what an interface
does. Nothing is inherited: no method body comes from an interface, no property
or constant is acquired from one, and there is no interface-based dispatch. A
class that passes the check has exactly the members it wrote, which is the same
rule the rest of this page states.

`interface A extends B, C` is parsed, and the names a class is checked against
are the union of what every listed interface declares. The union is computed
when the check runs; no member moves anywhere, because an interface holds none.

The check runs on the AST before anything executes: in `runner.hoist` for the
interpreter, and in the flatstack compiler's `collectClasses` for the bytecode
engine, so both backends reach the same verdict. `phpscript lint` reports a
missing method as a finding, and at run time it raises a `RuntimeException`
naming the class, the interface and the method. A name no `interface`
declaration in the same file defines is not a contract and is not checked, which
is what lets `implements Countable` load: phpscript declares none of PHP's
built-in interfaces, and a file is registered as a unit.

`instanceof` does consult the list. A class records the interface names it
declared, plus the names those interfaces extend, and `$store instanceof Reader`
is true when `Store implements Writer` and `Writer extends Reader`. That is a
name comparison over what the declaration listed, and no member arrives through
it: the class still answers only with the methods it wrote.

## Exceptions without a hierarchy

Every PHP throwable class is one Go type, `stdlib.Exception`, carrying the name
the script constructed. `get_class($e)` returns that name, which is how a script
discriminates further and how it writes a default "unhandled exception type"
branch.

A catch clause is answered from the name:

| Clause names                                    | Takes                                                                        |
|-------------------------------------------------|------------------------------------------------------------------------------|
| nothing, or `Throwable`                         | everything                                                                   |
| anything, when the error is no PHP class at all | everything, so a Go binding's error reaches the catch a script already wrote |
| `Exception`                                     | any class whose name does not end in `Error`                                 |
| `Error`                                         | any class whose name ends in `Error`                                         |
| any other name                                  | that class name, case-insensitively                                          |

The suffix is the whole of the split between a fault in the program and a
condition the program raised, and it agrees with PHP for every built-in name:
`ErrorException` is an `Exception`, `TypeError` and `AssertionError` are
`Error`s.

Three consequences differ from PHP, and are the price of having no hierarchy:

- `catch (LogicException $e)` does not take an `InvalidArgumentException`.
- `catch (Exception $e)` takes a class of your own named `NotFound`, and
  `catch (Error $e)` takes one named `MyError`.
- `$e instanceof Throwable` is false. `Throwable` is a PHP built-in interface,
  and no declaration in the program lists it, so there is nothing for the check
  to read.

`instanceof` compares the class name, and an interface name against the list the
class declared. Nothing follows `extends` on a class.

## Won't implement

Separate from the "Not implemented" rows in the
[language reference](reference/README.md), which mean "not yet". Nothing here is
planned, and each row names what to use instead.

| PHP API                                                                  | Use instead                                                                                                                    |
|--------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------|
| `extends` semantics, traits, `parent::`                                  | Composition; declare the members a class uses. An interface is checked, never inherited from                                   |
| `abstract` semantics                                                     | An `interface`; the modifier parses, the class instantiates, and a bodyless method returns null. `phpscript lint` reports both |
| Magic methods beyond `__construct` and `__invoke`                        | Explicit methods                                                                                                               |
| `new self()`, `new static()`                                             | `new ClassName()`; the keywords are not resolved and fail loudly as an undefined class                                         |
| `setcookie`, `setrawcookie`                                              | `Session\Manager`, or `header("Set-Cookie: ...", false)`                                                                       |
| `session_start`, `session_id`, `session_destroy`, `$_SESSION`            | `Session\Manager`, `Session\Storage\Disk`, `Session\Storage\Memory`                                                            |
| `curl_*`                                                                 | `HTTP\Client`, `HTTP\Request`                                                                                                  |
| PDO, `mysqli_*`, `pg_*`, `sqlite3_*`                                     | `Database`, `Database\Migrate`                                                                                                 |
| `shmop_*`, `apcu_*`, `sem_*`                                             | `SharedMemory`                                                                                                                 |
| `strftime`, `gmstrftime`, `gmdate`, `mktime`                             | `DateTime`, `Time`; Go layouts rather than format characters. `date` and `strtotime` exist as numeric-and-layout shims         |
| `JSON_PRETTY_PRINT`, `JSON_UNESCAPED_SLASHES`, every other `JSON_*` flag | `json_encode($value)`; the encoding is not configurable                                                                        |
| `uniqid`                                                                 | `ulid()`, `uuid()`, or `bin2hex(random_bytes($n))`                                                                             |
| `create_function`                                                        | Closures                                                                                                                       |
| `${var}` string interpolation                                            | `{$var}`                                                                                                                       |
| `global`                                                                 | Pass collaborators as parameters; the statement parses and binds nothing                                                       |
| `eval`                                                                   | nothing; there is no runtime source evaluation                                                                                 |
| `goto`                                                                   | nothing                                                                                                                        |
| `yield`, generators, Fibers                                              | nothing; there is no coroutine model                                                                                           |
| `trigger_error`, `restore_error_handler`, `@`                            | `try`/`catch` in PHP, `Runtime.OnError` in Go                                                                                  |
| `&` outside `foreach`                                                    | Return the value; see [Value semantics](reference/types/value-semantics.md)                                                    |

### global

`global $x;` parses and does nothing: the variable stays unset inside the
function, where PHP would import the binding from the global scope. This is a
decision, not a gap. A function that names its collaborators as parameters can
be read, tested and moved on its own; a function that reaches for `global`
depends on state the call site never mentions, which is the wrong-at-a-distance
coupling this runtime is built to avoid. Ported code that keeps its `global`
lines loads cleanly and then reads the variable as unset, so treat every
`global` statement in a port as a parameter waiting to be written.
`phpscript lint` reports each one as a warning, as it does a class `extends`
clause, the other statement that parses and confers nothing.

### references

`&` outside a `foreach` will not be implemented: the runtime has no reference
values, so aliasing two names to one storage is not expressible. The spellings
still parse and survive the formatter as written — `$a = &$b` binds the value,
and `function &getRef()` (free function, method or closure) returns the value —
because a port should format cleanly before it is rewritten. What never
happens is the aliasing: a later write through one name is not seen through
the other, and a caller holding a "reference" return holds a copy.
`phpscript lint` reports every marker, as it does `global` and `extends`; a
parameter's `&`, a closure's `use (&$x)` and `foreach ($a as &$v)` keep their
meaning and are not reported. Return the value instead; see
[Value semantics](reference/types/value-semantics.md).

### Dates and times

PHP's date family is not implemented beyond a deliberately small shim.
`date()`'s format characters, `strftime()`'s `%` codes and `strtotime()`'s
English are three spellings of the same job, and the runtime binds Go's instead:
`DateTime::now()`, `DateTime::parse()` and `$t->format()` take
[Go layouts](https://pkg.go.dev/time#pkg-constants), where the layout is the
reference instant written the way the output should read. `2006-01-02` is a
date; `Y-m-d` is a table lookup, and `strtotime("next thursday")` is a guess.

The shim in `stdlib/compat` covers the epoch corner of that family and stops
there. `strtotime` matches a fixed list of layouts from most to least
specific — RFC 3339 with and without fraction and offset, `Y-m-d H:i:s` and
its prefixes, RFC 1123, `@epoch`, `now` — and returns `false` for anything
else, English included. `date` knows the numeric format characters
(`Y y m n d j H G h g i s U`) and backslash escapes, and writes every other
character through unchanged; a word or a zone in the output is `$t->format()`
with a Go layout.

The value a script holds is Go's `time.Time`, which is what the database driver
already scans a `DATETIME` column into, so the same value flows from a query to
a template without a conversion in between. Every method returns a new one, so
the mutable/immutable split PHP carries as two classes does not arise. See the
[stdlib/time section](reference/extensions/implemented-apis.md) for the surface.

`echo $t` prints `2026-08-26 14:48:00`, not Go's `2026-08-26 14:48:00 +0000 UTC`
and not RFC 3339. PHP has no string form for a date to copy: `echo $dateTime`
is a fatal `Error`, "Object of class DateTime could not be converted to string".
The rule comes from every place PHP writes a datetime of its own accord. The
`date` field of a `var_dump`, a `print_r` and a `json_encode` all read
`Y-m-d H:i:s`, and PDO hands a `DATETIME` column to a script as the text it was
stored as. Go spells that layout `time.DateTime`. The zone is dropped for the
reason PHP drops it there, that it is a separate field rather than part of the
reading; `$t->format("2006-01-02 15:04:05 MST")` prints it when it matters.

Dispatching straight to Go's methods has two consequences. A duration is
nanoseconds, as it is in Go, so `$t->add(86400)` advances the clock by 86.4
microseconds rather than by a day; write `$t->add("24h")` instead. And a Go
method with several results arrives as a PHP list, read with
`list($year, $week) = $t->iso_week();`. The short `[$a, $b] =` spelling does
not parse here.

### JSON

`json_encode($value)` encodes the value. `$flags` is accepted and ignored, as
`json_decode` accepts `$depth` and `$flags`, so a literal one changes nothing.
No `JSON_*` constant is defined, and an undefined constant throws, so
`json_encode($v, JSON_PRETTY_PRINT)` raises before the call is made.

PHP has those flags because its encoder makes choices a caller then has to
undo. `JSON_UNESCAPED_SLASHES` is the clearest: PHP writes `{"path":"a\/b"}`,
which is legal JSON that no other language emits, and the flag exists to turn
it off. This runtime encodes with Go's `encoding/json`, which writes the slash
as itself, so there is no choice to reverse and the flag has no work to do.

The rest are presentation. `JSON_PRETTY_PRINT` indents for a reader the
producer cannot see; whatever consumes the document can indent it, and `jq`
does. Emitting compact JSON and leaving the formatting to the consumer is one
behaviour to test and one output to compare, and a payload that is diffed or
signed does not change shape depending on a flag at the call site.

`json_decode` accepts `$depth` and `$flags` and ignores them, because library
code passes them defensively. `$associative` must be true or omitted: decoding
into objects is not implemented. `stdClass` itself exists, and `(object)` builds
one, so this is a gap in the decoder rather than a missing class.

`phpscript lint` warns on a `JSON_*` name, so a port is told before it runs
rather than at the first request that reaches the line. The name inside a
string is not a use, so `defined("JSON_PRETTY_PRINT")` reads as written.

`stdlib/core.TestNoJSONFlagConstants` fails the build if a `JSON_*` constant
is registered.

### Cookies

`setcookie()` is the row a reader questions, because dbadmin has a login. The
session cookie is written from Go. `Session\Manager::start()` mints the session
id from `crypto/rand` and sets the header itself with `HttpOnly`, `Path=/` and
`SameSite=Lax` fixed, so a script cannot weaken those attributes or choose the
value. For any other cookie, `header()` already takes `$replace = false` and so
emits repeated `Set-Cookie` headers; `setcookie()` in PHP is a string formatter
over exactly that. `$_COOKIE` is the read side and is populated per request.

[demos/dbadmin](../demos/dbadmin) is the demonstration that the shape is
sufficient: a complete login, logout, CSRF and post-redirect-message flow with
no cookie string anywhere in its PHP.

### Error handlers

`set_error_handler()` is registered, and is a stub that accepts a callback and
returns null. There are no notices, warnings or error levels for a handler to
receive, so nothing ever calls it; it exists so that library code calling it
defensively still loads. `trigger_error()` is not registered at all.
