# Constants

| PHP language-reference feature | Status | Notes |
| --- | --- | --- |
| Global constant declarations | Not implemented | `const` and `define()` cannot declare global constants in script code. |
| Host-registered constants | phpscript extension | A Go host can register constants with `Runtime.SetConst`. |
| Class constants | Compatibility | `const NAME = value` and `Class::NAME` are supported. |
| Predefined constants | Partial compatibility | Standard-library registration installs a small runtime-defined set. |
| Magic constants | Partial compatibility | Only `__NAMESPACE__` is implemented. |

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

The script can then read `APP_ENV` as a bare identifier. Use
`get_defined_constants()` to inspect registered constants.
