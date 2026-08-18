# Exceptions

| PHP language-reference feature | Status                | Notes                                                                                                                    |
|--------------------------------|-----------------------|--------------------------------------------------------------------------------------------------------------------------|
| `throw`                        | Compatibility         | Values and constructed `Exception` objects can be thrown.                                                                |
| `try`, `catch`, `finally`      | Partial compatibility | The first catch handles every failure; `finally` always runs.                                                            |
| Catch type filters             | Not implemented       | Parsed exception types and union filters are ignored.                                                                    |
| Exception hierarchy            | Partial compatibility | Every throwable class name is one type, so any catch catches any failure; the `Throwable` methods answer on all of them. |
| Extending exceptions           | Not implemented       | Class inheritance is unavailable.                                                                                        |

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

The catch variable receives the object that was thrown, so `getMessage()` and
`getCode()` report what it was constructed with. A catch type is accepted for
PHP-like syntax but is not checked; when several catches are present, only the
first can handle the error.

Every throwable class name (`Exception`, `Error`, `RuntimeException`,
`ArgumentCountError` and the rest of the SPL set) is registered to one type.
Naming one of them in a catch therefore narrows nothing, and a script that
catches `Exception` also catches what PHP would raise as an `Error`.

The `Throwable` method set answers on whatever reached the catch, including an
error a Go binding returned and a panic recovered at the host boundary:

| Method                             | Value                                                         |
|------------------------------------|---------------------------------------------------------------|
| `getMessage()`                     | The message, which is the Go error text for a binding failure |
| `getCode()`                        | The constructed code, or 0 for an error that carries none     |
| `getPrevious()`                    | The wrapped error, or null                                    |
| `getFile()`, `getLine()`           | Empty and 0; a throwable does not carry a source position     |
| `getTrace()`, `getTraceAsString()` | An empty array and `#0 {main}`; no stack is recorded          |

Errors returned by a Go function invoked through a runtime binding enter the
same catch path as an explicit `throw`.

Panics raised by registered Go constructors, functions, or methods are recovered
at the host-call boundary and enter this same catch path as `HostPanicError`.
Without a matching `try`/`catch`, the error is returned to the embedding host.
