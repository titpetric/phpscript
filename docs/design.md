# Design decisions

Decisions about what phpscript is, and what it will not become. They are here so that a change proposing one of them again starts from the reasoning. [Known divergences from PHP](README.md#known-divergences-from-php) lists what a script sees where the two differ; this page says why.

## No OOP

phpscript runs classes as namespaced bags of properties and methods. It has no object model beyond that, and will not get one.

What exists:

- `class` declarations with properties, methods and class constants
- `__construct`, `$this`, `__invoke`
- static properties and methods, `self::` and `static::`, including the variable spelling `Class::$m()`
- function-level `static $x` variables, persistent per function and per closure value
- `Class::class`
- `interface` declarations and `implements`, as a contract check, plus constants read through the interface's own name; see below

What does not, and will not:

- inheritance. A class gets no members from any other class: no inherited methods, no inherited properties or constants, no `parent::`, no constructor to fall back on.
- traits
- `abstract`, `final` and `readonly` as anything but parsed modifiers
- magic methods beyond `__construct` and `__invoke`

The parser records `extends` on `model.ClassDecl` so the formatter prints back the file it read and the linter can see the declaration. The `runner` package must not read it. **Implementing `extends` is out of bounds.** The throwable hierarchy carries no parent table; `runner.TestNoInheritanceAtRuntime` fails the build if `model.Class` regrows a `Parent` field.

Write composition instead, and declare the members a class uses. A class that calls something it did not declare fails at the call, naming the class and the method.

### Interfaces are a contract, not a parent

An interface names method signatures. A class that says `implements` must declare every one of them itself, and that is the whole of what an interface does. A class inherits no member: no method body comes from an interface, no property or constant comes from one, and there is no interface-based dispatch. A class that passes the check has exactly the members it wrote, which is the same rule the rest of this page states.

The parser accepts `interface A extends B, C`, and the check tests a class against the union of what every listed interface declares. The check computes that union when it runs; no member moves anywhere.

An interface does hold its constants. `Hook::POSITION_PRE` resolves through the name declaring the constant, exactly as a class constant does, on both backends and in the linter. That is direct resolution, not inheritance: an implementing class acquires none of them and `Store::LIMIT` through `class Store implements Quota` fails as an undefined class constant where PHP answers it, and a constant declared on an extended interface does not answer through the extender's name. `tests/fixtures/oop/interface_constants_not_acquired.phpt` pins the divergence.

The check runs on the AST before anything executes: in `runner.hoist` for the interpreter, and in the flatstack compiler's `collectClasses` for the bytecode engine and both backends reach the same verdict. `phpscript lint` reports a missing method as a finding, and at run time it raises a `RuntimeException` naming the class, the interface and the method. A name no `interface` declaration in the same file defines is not a contract and is not checked and `implements Countable` loads: phpscript declares none of PHP's built-in interfaces, and the unit it registers is the file.

`instanceof` does consult the list. A class records the interface names it declared, plus the names those interfaces extend, and `$store instanceof Reader` is true when `Store implements Writer` and `Writer extends Reader`. That is a name comparison over what the declaration listed, and no member arrives through it: the class still answers only with the methods it wrote.

## Exceptions without a hierarchy

Every PHP throwable class is one Go type, `stdlib.Exception`, carrying the name the script constructed. `get_class($e)` returns that name and a script discriminates further and how it writes a default "unhandled exception type" branch.

phpscript answers a catch clause from the name:

| Clause names                                    | Takes                                                                        |
|-------------------------------------------------|------------------------------------------------------------------------------|
| `Throwable`, or no name                         | everything                                                                   |
| anything, when the error is no PHP class at all | everything and a Go binding's error reaches the catch a script already wrote |
| `Exception`                                     | any class whose name does not end in `Error`                                 |
| `Error`                                         | any class whose name ends in `Error`                                         |
| any other name                                  | that class name, case-insensitively                                          |

The suffix is the whole of the split between a fault in the program and a condition the program raised, and it agrees with PHP for every built-in name: `ErrorException` is an `Exception`, `TypeError` and `AssertionError` are `Error`s.

Three consequences differ from PHP, and are the price of having no hierarchy:

- `catch (LogicException $e)` does not take an `InvalidArgumentException`.
- `catch (Exception $e)` takes a class of your own named `NotFound`, and `catch (Error $e)` takes one named `MyError`.
- `$e instanceof Throwable` is false. `Throwable` is a PHP built-in interface, and no declaration in the program lists it, and the check has nothing to read.

`instanceof` compares the class name, and an interface name against the list the class declared. No member follows `extends` on a class.

## Variable types: advisory, not enforced

The first literal assigned to a variable declares its type in convention: `phpscript lint` reports a later literal of another class as "no reassignment: $a already declared as string". The finding is advisory and the runtime follows PHP, retyping the variable silently.

phpscript refuses enforcement. A throw on the same rule, catchable, on both engines, carries the allowances a stateless check permits: int and float as one number class, null-first as declaring nothing, false as the stdlib's T|false absence sentinel, foreach targets as per-iteration bindings. The rules hold together; the cost lands on existing code. Ordinary PHP retypes variables constantly and every violation needs a rewrite before the program runs at all, and the stdlib's own return conventions need special-casing to keep the runtime usable. A convention a linter flags is worth keeping, and a contract that stops running programs is not. The reasoning stays here so the next proposal to enforce it starts from that cost.

## Program re-entry

An HTTP server handler written in PHP is the program being re-entered. The Go side takes the connection on a goroutine of its own and calls back into the program to answer it, once per request, and the call returns when the handler does.

Re-entry gets a stack and reuses everything else. The scope a handler runs in holds `$w`, `$r` and a bound method's `$this`: the runtime decodes no superglobals, and nothing carries over from the last request the worker answered. The parsed AST, the include cache, the compiled expression chains, the bytecode, the symbol tables and the objects a handler captured are all shared and read and the process pays the parse and the compile once.

No two workers can share the execution. Frames, the scope pool, the output stack, function statics, the request a worker answers and the memory accounting all change as a program runs, with no lock and `runner.Runtime` holds one set per worker and not one per process: `Fork` builds a runtime with the same symbols and the same caches and none of the execution state, and `Pool` forks once per worker and feeds the workers from one queue. The shape a threaded VM would use instead - one runtime, a stack per goroutine, indexed by goroutine id - is not available here, and it would cost no less: Go cannot read a goroutine id without parsing a stack trace, around four microseconds, and the frames are one of six things such a VM would still have to split.

A call inside a handler is re-entry as well, and follows the same rule. A callable the script built before the server started is a value like any other, and a handler reaches one as a capture, off a shared object, or out of an array; calling it runs the declaration on the runtime doing the calling, never on the runtime that built the value. The value crosses, the execution does not. `Callable.on(rt)` decides it, and `invoker.call` threads the calling runtime into `coerceArgOn` so a binding that calls back - `usort`, `array_map`, `preg_replace_callback` - binds its callback the same way. The runtime is never looked up by goroutine, because the caller already holds the handle.

Cancellation is per invocation: the mux enters the request's context for the length of the handler and leaves it when the handler returns. `connection_aborted()` reports that context, which also ends `sleep()` early when the client goes away. The execution limit is not per invocation. `set_time_limit` moves the runtime's own deadline, and a script that serves is one script and a handler that moves it moves the server's. [use-cases/http-server.md](use-cases/http-server.md) is the same model from a script's side.

A binding called inside a handler receives a context derived from the request, not from the session: the request is authoritative, because Go middleware ahead of the handler attaches values, deadlines and cancellation to it, and every one of those has to reach a binding blocked in I/O. `ignore_user_abort(true)` keeps the request's values and drops its cancellation and a disconnect ends no work the handler chose to finish, and the active span still resolves through the session and a database call inside a traced request nests where the handler is. `runner.bindingContext` is the derivation and `runner/bindingctx.go` carries the chain order.

## Won't build

Separate from the "Not implemented" rows in the [language reference](reference/README.md), which mean "not yet". Every row here is a decision that stays, and each names what to use instead.

| PHP API                                                                  | Use instead                                                                                                                                                                                     |
|--------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `extends` semantics, traits, `parent::`                                  | Composition; declare the members a class uses. An interface is a contract to check, never a parent to inherit from                                                                              |
| `abstract` semantics                                                     | An `interface`; the modifier parses, the class instantiates, and a bodyless method returns null. `phpscript lint` reports both                                                                  |
| Magic methods beyond `__construct` and `__invoke`                        | Explicit methods                                                                                                                                                                                |
| `new self()`, `new static()`                                             | `new ClassName()`; the keywords are not resolved and fail loudly as an undefined class                                                                                                          |
| `session_start`, `session_id`, `session_destroy`, `$_SESSION`            | `Session\Manager`, `Session\Storage\Disk`, `Session\Storage\Memory`, `Auth\Storage\PasswordFile`                                                                                                |
| `curl_*`                                                                 | `HTTP\Client`, `HTTP\Request`                                                                                                                                                                   |
| PDO, `mysqli_*`, `pg_*`, `sqlite3_*`                                     | `Database`, `Database\Migrate`                                                                                                                                                                  |
| `shmop_*`, `apcu_*`, `sem_*`                                             | `SharedMemory`                                                                                                                                                                                  |
| `extension_loaded`                                                       | No replacement; it answers false for every name, because phpscript loads no extensions and never will                                                                                           |
| `opcache_*`                                                              | `phpinfo()`; it reports the parsed and compiled tree, and there is no arena for the rest of the family to describe                                                                              |
| `strftime`, `gmstrftime`, `gmdate`, `mktime`                             | `DateTime`, `Time`; Go layouts rather than format characters. `date` and `strtotime` exist as numeric-and-layout shims                                                                          |
| `JSON_PRETTY_PRINT`, `JSON_UNESCAPED_SLASHES`, every other `JSON_*` flag | `json_encode($value)` for the compact string; `new JSON\Encoder($stream)` with `set_indent()` where the output needs indenting or streaming                                                     |
| `uniqid`                                                                 | `ulid()`, `uuid()`, or `bin2hex(random_bytes($n))`                                                                                                                                              |
| `create_function`                                                        | Closures                                                                                                                                                                                        |
| `${expression}` string interpolation, and `$$name`                       | `${name}` and `${name[$k]}` parse and behave as `$name` and `$name[$k]`. The expression form names a variable by the value the expression returns, which needs `$$name`; write `{$...}` instead |
| `global`                                                                 | Pass collaborators as parameters; the statement parses and binds nothing                                                                                                                        |
| `eval`                                                                   | No replacement; there is no runtime source evaluation                                                                                                                                           |
| `goto`                                                                   | No replacement                                                                                                                                                                                  |
| `yield`, generators, Fibers                                              | No replacement; there is no coroutine model                                                                                                                                                     |
| `trigger_error`, `restore_error_handler`, `@`                            | `try`/`catch` in PHP, `Runtime.OnError` in Go                                                                                                                                                   |
| `&` outside `foreach`                                                    | Return the value; see [Value semantics](reference/types/value-semantics.md)                                                                                                                     |

### global

`global $x;` parses and has no effect: the variable stays unset inside the function, where PHP would import the binding from the global scope. This is a decision. A function that names its collaborators as parameters stands on its own, to read, to test and to move; a function that reaches for `global` depends on state the call site never mentions, which is the wrong-at-a-distance coupling this runtime avoids. Ported code that keeps its `global` lines loads cleanly and then reads the variable as unset and treat every `global` statement in a port as a parameter the signature still needs. `phpscript lint` reports each one as a warning, as it does a class `extends` clause, the other statement that parses and confers nothing.

### references

`&` outside a `foreach` will never arrive: the runtime has no reference values and aliasing two names to one storage is not expressible. The spellings still parse and survive the formatter as written, because a port should format cleanly before a rewrite: `$a = &$b` binds the value, and `function &getRef()` (free function, method or closure) returns the value. What never happens is the aliasing: a later write through one name is not seen through the other, and a caller holding a "reference" return holds a copy. `phpscript lint` reports every marker, as it does `global` and `extends`; a parameter's `&`, a closure's `use (&$x)` and `foreach ($a as &$v)` keep their meaning and are not reported. Return the value instead; see [Value semantics](reference/types/value-semantics.md).

### Dates and times

PHP's date family is not implemented beyond a small shim. `date()`'s format characters, `strftime()`'s `%` codes and `strtotime()`'s English are three spellings of the same job, and the runtime binds Go's instead: `DateTime::now()`, `DateTime::parse()` and `$t->format()` take [Go layouts](https://pkg.go.dev/time#pkg-constants), where the layout is the reference instant written the way the output should read. `2006-01-02` is a date; `Y-m-d` is a table lookup, and `strtotime("next thursday")` is a guess.

The shim in `stdlib/compat` covers the epoch corner of that family and stops there. `strtotime` matches a fixed list of layouts from most to least specific, and returns `false` for anything else, English included. The list is RFC 3339 with and without fraction and offset, `Y-m-d H:i:s` and its prefixes, RFC 1123, `@epoch`, and `now`. `date` knows the numeric format characters (`Y y m n d j H G h g i s U`) and backslash escapes, and writes every other character through unchanged; a word or a zone in the output is `$t->format()` with a Go layout.

The value a script holds is Go's `time.Time`, the type the database driver already scans a `DATETIME` column into and the same value flows from a query to a template with no conversion on the way. Every method returns a new one and the mutable/immutable split PHP carries as two classes does not arise. See the [stdlib/time section](reference/extensions/implemented-apis.md) for the surface.

`echo $t` prints `2026-08-26 14:48:00`, not Go's `2026-08-26 14:48:00 +0000 UTC` and not RFC 3339. PHP has no string form for a date to copy: `echo $dateTime` is a fatal `Error`, `Object of class DateTime could not be converted to string`. The rule comes from every place PHP writes a datetime of its own accord. The `date` field of a `var_dump`, a `print_r` and a `json_encode` all read `Y-m-d H:i:s`, and PDO returns a `DATETIME` column to a script as the stored text. Go spells that layout `time.DateTime`. phpscript drops the zone for the reason PHP drops it there: the zone is a separate field, outside the reading; `$t->format("2006-01-02 15:04:05 MST")` prints it when it matters.

Dispatching straight to Go's methods has two consequences. A duration is nanoseconds, as in Go and `$t->add(86400)` advances the clock by 86.4 microseconds, where a day needs `$t->add("24h")`. And a Go method with more than one result arrives as a PHP list, read with `list($year, $week) = $t->iso_week();`. The short `[$a, $b] =` spelling does not parse here.

### JSON

`json_encode($value)` encodes the value. The encoder accepts `$flags` and ignores it, as `json_decode` accepts `$depth` and `$flags` and a literal one changes nothing. phpscript defines no `JSON_*` constant, and an undefined constant throws and `json_encode($v, JSON_PRETTY_PRINT)` raises before the call runs.

PHP has those flags because its encoder makes choices a caller then has to undo. `JSON_UNESCAPED_SLASHES` is the clearest: PHP writes `{"path":"a\/b"}`, which is legal JSON that no other language emits, and the flag exists to turn it off. This runtime encodes with Go's `encoding/json`, which writes the slash as itself and there is no choice to reverse and the flag has no work to do.

The rest are presentation, and presentation lives on the bound Go encoder. `new JSON\Encoder($stream)` is `encoding/json`'s encoder over any handle `fopen()` returns: `$enc->set_indent("", " ")` selects what `JSON_PRETTY_PRINT` selects in PHP, chosen once where the script constructs the encoder and not at every call site, and `encode($value)` writes to the stream as it walks the value and a response body goes out without assembling a string of it first. `new JSON\Decoder(fopen("php://input", "r"))` is the read side, one value per `decode()` behind a `while ($dec->more())` loop. The constructor pair is the favoured form wherever the document needs indenting, streaming or reading in pieces; `json_encode($value)` stays the one-shot spelling for a compact string, which is one behaviour to test and one output to compare, and a payload a caller diffs or signs does not change shape depending on a flag at the call site.

Both spellings answer the same document for the same value: the two encoders share one value walk, an object keeps the order the script built it in, and a whole number decodes as an int on both paths.

`json_decode` accepts `$depth` and `$flags` and ignores them, because library code passes them defensively. `$associative` must be true or omitted: decoding into objects is not implemented, on either path. `stdClass` itself exists, and `(object)` builds one and this is a gap in the decoder, with no class missing.

`phpscript lint` warns on a `JSON_*` name and the finding arrives before a port runs, ahead of the first request that reaches the line. The name inside a string is not a use and `defined("JSON_PRETTY_PRINT")` reads as written.

`stdlib/core.TestNoJSONFlagConstants` fails the build if anything registers a `JSON_*` constant.

### Regular expressions

`preg_*` is a reduced surface: `preg_replace` and `preg_replace_callback` take one string pattern and one subject, with no array form, `preg_replace` has no `$limit`, and a named group takes a number but never populates `$matches["name"]`. The reduction is not the ceiling, because the same build binds Go's `regexp` package as classes: `new Regexp\Compile($pattern)` and `new Regexp\CompilePOSIX($pattern)` expose every method of `*regexp.Regexp` under PHP spellings and constructing one reaches named groups, leftmost-longest matching and a compile-once pattern value, and `preg_*` stays unchanged.

The split is the one JSON makes above. The PHP names keep PHP's behaviour for ported code - a PCRE pattern, PHP's `$matches` shapes, the `PREG_*` flags that exist - and the bound Go stdlib is the option for code written here, where the pattern is RE2 syntax and the engine's own API answers what the reduced surface does not. [Regular expressions](reference/extensions/regexp.md) carries both surfaces, the two-engine compilation behind `preg_*` and what still differs from PCRE; [Regexp bindings](bindings-regexp.md) is the walkthrough.

### Cookies

`setcookie()` and `setrawcookie()` exist and stage a `Set-Cookie` header. Both take PHP's positional list (`$expires`, `$path`, `$domain`, `$secure`, `$httponly`) and PHP's array form, which adds `samesite`; `setcookie` url-encodes the value and `setrawcookie` writes it as it stands. The header text is `net/http`'s: an attribute reads `Path` where PHP writes `path`, and the expiry is RFC 1123; RFC 6265 treats both spellings alike.

The session cookie belongs to `Session\Manager`. `Session\Manager::start()` mints the session id from `crypto/rand` and sets the header itself with `HttpOnly`, `Path=/` and `SameSite=Lax` fixed, and a script cannot weaken those attributes or choose the value; `destroy()` writes the expiry of that same cookie. `$_COOKIE` is the read side, which the runtime fills per request.

[demos/dbadmin](../demos/dbadmin) is the demonstration that the shape is enough: every page behind a session, a login against the application's own `.htpasswd`, and a logout, with no cookie string and no password hash anywhere in its PHP.

### Error handlers

phpscript registers `set_error_handler()` as a stub that accepts a callback and returns null. There are no notices, warnings or error levels for a handler to receive and nothing ever calls it; it exists so that library code calling it defensively still loads. `trigger_error()` is not registered at all.
