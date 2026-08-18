# Constants

| PHP language-reference feature | Status                | Notes                                                                   |
|--------------------------------|-----------------------|-------------------------------------------------------------------------|
| Global constant declarations   | Partial compatibility | `define()` declares a runtime constant; the `const` statement does not. |
| Host-registered constants      | phpscript extension   | A Go host can register constants with `Runtime.SetConst`.               |
| Class constants                | Compatibility         | `const NAME = value` and `Class::NAME` are supported.                   |
| Predefined constants           | Partial compatibility | Standard-library registration installs a small runtime-defined set.     |
| Magic constants                | Partial compatibility | `__NAMESPACE__`, `__FILE__`, `__DIR__`, and `__LINE__` are implemented. |

## Class constants

```php
class Status
{
    const READY = "ready";
}

echo Status::READY;
```

Multiple constants may be declared in one class declaration. Visibility on
class constants is not enforced.

## Runtime constants

Embedded applications can expose a value before execution:

```go
rt.SetConst("APP_ENV", "production")
```

The script can then read `APP_ENV` as a bare identifier. `define()`,
`defined()`, and `constant()` do the same from PHP, and
`get_defined_constants()` inspects the whole set.

```php
define("APP_ENV", "production");
echo defined("APP_ENV") ? constant("APP_ENV") : "unset";
```

A constant is visible in every scope, including inside functions and methods.
A bare identifier resolves from the current scope first and the constant table
second, which is how the magic constants, set per frame, take precedence.

## Predefined constants

Registering the standard library installs the platform constants a PHP library
expects to branch on: `PHP_VERSION` and `PHP_VERSION_ID`, `PHP_MAJOR_VERSION`
and its siblings, `PHP_SAPI`, `PHP_EOL`, `PHP_OS` and `PHP_OS_FAMILY`,
`PHP_INT_MAX` / `PHP_INT_MIN` / `PHP_INT_SIZE`, the `PHP_FLOAT_*` set,
`DIRECTORY_SEPARATOR` and `PATH_SEPARATOR`, `STDIN` / `STDOUT` / `STDERR`, the
`ENT_*` escaping flags, the `FILTER_VALIDATE_*` filters, the `E_*` error levels,
and the `T_*` tokenizer ids.

`PHP_VERSION` reports the PHP language version whose semantics phpscript is
tested against. It is not a claim of full compatibility with that release; it is
the number library code reads when it decides which language features it may
use.
