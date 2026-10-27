# Constants

| PHP language-reference feature | Status                | Notes                                                                      |
|--------------------------------|-----------------------|----------------------------------------------------------------------------|
| Global constant declarations   | Compatibility         | `define()` and the `const` statement both declare a runtime constant.      |
| Host-registered constants      | phpscript extension   | A Go host can register constants with `Runtime.SetConst`.                  |
| Class constants                | Compatibility         | `const NAME = value` declares one and `Class::NAME` reads it.              |
| Predefined constants           | Partial compatibility | Standard-library registration installs a small runtime-defined set.        |
| Magic constants                | Partial compatibility | `__NAMESPACE__`, `__FILE__`, `__DIR__` and `__LINE__` compile to literals. |

## Class constants

```php
class Status
{
    const READY = "ready";
}

echo Status::READY;
```

One class declaration can hold more than one constant. Class constants carry no visibility check.

## Runtime constants

Embedded applications can expose a value before execution:

```go
rt.SetConst("APP_ENV", "production")
```

The script can then read `APP_ENV` as a bare identifier. `define()`, the top-level `const` statement, `defined()`, and `constant()` do the same from PHP, and `get_defined_constants()` inspects the whole set. `define()` and `const` write the same table; `const` evaluates its value once, at the declaration.

```php
define("APP_ENV", "production");
const APP_VERSION = "1.0";
echo defined("APP_ENV") ? constant("APP_ENV") : "unset";
```

A constant is visible in every scope, including inside functions and methods. A bare identifier resolves from the current scope first and the constant table second and a variable of the same name shadows one.

A bare identifier nothing defines throws:

```php
echo MISSING;         // RuntimeException: Undefined constant "MISSING"
echo $missing;        // null, and the script continues
```

The two are separate lookups and an unset variable is still the null PHP reads it as.

PHP 8 raises `Error` for the same expression. This raises `RuntimeException` and `catch (Exception $e)`, `catch (RuntimeException $e)` and `catch (Throwable $e)` take it and `catch (Error $e)` does not. An `Error` in PHP is a fault a caller is not expected to handle, and a name this runtime does not define is a condition a script can answer for. Use `defined()` to ask without throwing.

## Magic constants

The compiler resolves each one as it reads the file, the way php resolves them, and leaves a literal in the parsed program. The runtime looks none up while the script runs.

The table is php's own list, in the order the [language reference](https://www.php.net/manual/en/language.constants.magic.php) lists them.

| Name            | Status        | Answers                                                                                                                         |
|-----------------|---------------|---------------------------------------------------------------------------------------------------------------------------------|
| `__LINE__`      | Compatibility | The line it sits on.                                                                                                            |
| `__FILE__`      | Divergence    | The path the file reads under, which is the source filesystem's spelling.                                                       |
| `__DIR__`       | Divergence    | The directory of that path, by the same rule.                                                                                   |
| `__FUNCTION__`  | Compatibility | The enclosing function, namespace-qualified; a method's bare name inside one.                                                   |
| `__CLASS__`     | Compatibility | The enclosing class, namespace-qualified. Empty outside one.                                                                    |
| `__METHOD__`    | Compatibility | `Class::method` inside a class, the function's own name outside one.                                                            |
| `__NAMESPACE__` | Compatibility | The namespace the file declares, empty in a file that declares none.                                                            |
| `__TRAIT__`     | Won't build   | No answer. There are no traits, and there is no name to answer. See [Design](../../design.md).                                  |
| `__PROPERTY__`  | Won't build   | No answer. There are no property hooks for it to name.                                                                          |
| `::class`       | Partial       | The class name, namespace-qualified. `Name::class`, `self::class` and `static::class` resolve; `$object::class` does not parse. |

A name in the last three rows is an ordinary undefined constant: reading `__TRAIT__` throws `Undefined constant "__TRAIT__"`, the same as any name nothing declares.

### What defined() sees

None of these is in the constant table, and php reports the same:

```php
echo __LINE__;                  // the line this is written on
var_dump(defined("__FILE__"));  // false, in php too
```

The name and the string are separate. `define("__FILE__", "x")` declares an ordinary constant that `defined()` and `constant()` answer, and the bare `__FILE__` still compiles to the path, because the token resolves ahead of that line. php behaves the same way.

### Where the path constants differ

`__FILE__` and `__DIR__` name the path the file reads under, which is the source filesystem's spelling and not the host's: `/public/index.php` where php says `/srv/site/public/index.php`. See [Known divergences](../../README.md).

Source passed to the runtime as a string, with no file behind it, has no path to compile. There `__FILE__` and `__DIR__` fall back to the runtime's own entrypoint. The other magic constants never fall back, because the compiler knows a line, a function and a class either way.

### Scope

A name resolves where it sits, not where the call comes from. A method returning `__CLASS__` answers its own class whichever caller reached it.

```php
function freeFunction() {
	return __FUNCTION__ . "|" . __CLASS__ . "|" . __METHOD__;
}
echo freeFunction();     // freeFunction||freeFunction

class Widget {
	public function instance() {
		return __FUNCTION__ . "|" . __CLASS__ . "|" . __METHOD__;
	}
}
echo (new Widget)->instance();   // instance|Widget|Widget::instance
```

At the top level all three are empty strings. The fixture that holds every one of these to php's own output is `tests/fixtures/paths/api/magic_scope_test.php`.

## Predefined constants

Registering the standard library installs the platform constants a PHP library branches on: `PHP_VERSION` and `PHP_VERSION_ID`, `PHP_MAJOR_VERSION` and its siblings, `PHP_SAPI`, `PHP_EOL`, `PHP_OS` and `PHP_OS_FAMILY`, `PHP_INT_MAX` / `PHP_INT_MIN` / `PHP_INT_SIZE`, the `PHP_FLOAT_*` set, `INF` and `NAN`, `DIRECTORY_SEPARATOR` and `PATH_SEPARATOR`, `STDIN` / `STDOUT` / `STDERR`, the `ENT_*` escaping flags, the `FILTER_VALIDATE_*` filters, the `E_*` error levels, and the `T_*` tokenizer ids.

`INF` and `NAN` are the two float values with no numeric spelling. `NAN` compares equal to nothing, itself included and `$x !== $x` is the test for one; `tests/fixtures/arithmetic/float_inf_nan.phpt` holds both to php's output.

`PHP_VERSION` reports the PHP language version whose semantics the fixtures hold phpscript to. The number claims no full compatibility with that release; library code reads it to decide which language features it may use.

### Time layouts

`TIME_RFC3339`, `TIME_RFC3339_NANO`, `TIME_DATETIME`, `TIME_DATE_ONLY` and `TIME_TIME_ONLY` hold the [Go layouts](https://pkg.go.dev/time#pkg-constants) of the same names, for `$t->format()` and `DateTime::parse()`. A layout is an ordinary string and a script can spell any of them out; these are the ones worth naming.

`TIME_RFC3339` is the one to reach for by default. Alone among the layouts here it carries the UTC offset and it survives a round trip through a database column or a JSON payload, and `json_encode` already emits it for a `Time`. See [Dates and times](../../use-cases/database.md#dates-and-times).

The names are flat because a namespaced constant does not parse in this runtime: the classes are `Time\Duration` and `Time\Location`, but a constant spells that prefix `TIME_`.
