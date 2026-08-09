# Exceptions

| PHP language-reference feature | Status | Notes |
| --- | --- | --- |
| `throw` | Compatibility | Values and constructed `Exception` objects can be thrown. |
| `try`, `catch`, `finally` | Partial compatibility | The first catch handles every failure; `finally` always runs. |
| Catch type filters | Not implemented | Parsed exception types and union filters are ignored. |
| Exception hierarchy | Not implemented | There is no PHP `Throwable` hierarchy or type-based dispatch. |
| Extending exceptions | Not implemented | Class inheritance is unavailable. |

## Throwing and catching

```php
try {
    throw new Exception("failed");
} catch (Exception $error) {
    echo $error;
} finally {
    echo "done";
}
```

The catch variable receives the runtime error value. A catch type is accepted
for PHP-like syntax but is not checked; when several catches are present, only
the first can handle the error.

Errors returned by a Go function invoked through a runtime binding enter the
same catch path as an explicit `throw`.

Panics raised by registered Go constructors, functions, or methods are recovered
at the host-call boundary and enter this same catch path as `HostPanicError`.
Without a matching `try`/`catch`, the error is returned to the embedding host.
