# Exceptions

| PHP language-reference feature | Status                | Notes                                                                                                                 |
|--------------------------------|-----------------------|-----------------------------------------------------------------------------------------------------------------------|
| `throw`                        | Compatibility         | A script throws a value or a constructed throwable object.                                                            |
| `try`, `catch`, `finally`      | Compatibility         | The runtime tries clauses in source order; `finally` always runs.                                                     |
| Catch type filters             | Partial compatibility | A clause filters on the class name a throwable records. There is no hierarchy to descend.                             |
| Exception hierarchy            | Won't build           | Every throwable class is one type; none is a subclass of another. See [Design decisions](../../design.md#wont-build). |
| Extending exceptions           | Won't build           | `extends` confers nothing. See [Design decisions](../../design.md).                                                   |

## Throwing and catching

```php
try {
    throw new Exception("failed");
} catch (Exception $error) {
    echo $error->getMessage();
} finally {
    echo "done";
}
```

The catch variable receives the thrown object and `getMessage()` and `getCode()` report its constructor arguments, and `get_class()` reports the class the `throw` named.

## Which clause takes it

Every throwable class is one type carrying the name a script constructed and the runtime answers a clause from that name, with no hierarchy to descend:

| Clause names                                    | Takes                                                                        |
|-------------------------------------------------|------------------------------------------------------------------------------|
| `Throwable`, or no name                         | everything                                                                   |
| anything, when the error is no PHP class at all | everything and a Go binding's error reaches the catch a script already wrote |
| `Exception`                                     | any class whose name does not end in `Error`                                 |
| `Error`                                         | any class whose name ends in `Error`                                         |
| any other name                                  | that class name, case-insensitively                                          |

The runtime tries clauses in source order, and a union matches on either alternative:

```php
try {
    throw new RuntimeException("no route to host");
} catch (LogicException $e) {
    // not reached: the name does not match
} catch (LogicException|RuntimeException $e) {
    echo $e->getMessage();
}
```

The suffix separates a fault in the program from a condition it raised, and agrees with PHP for every built-in name: `ErrorException` is an `Exception`, `TypeError` and `AssertionError` are `Error`s.

Three things follow from having no hierarchy, and differ from PHP:

- `catch (LogicException $e)` does not take an `InvalidArgumentException`.
- `catch (Exception $e)` takes a class of your own named `NotFound`, and `catch (Error $e)` takes one named `MyError`.
- `$e instanceof Throwable` is false. `Throwable` is a PHP built-in interface, and no declaration in the program lists it.

Where a catch has to find a throw, name the same class at both ends, or catch `Throwable` and branch on `get_class($e)`.

## The Throwable methods

The method set answers on whatever reached the catch, including an error a Go binding returned and a panic recovered at the host boundary:

| Method                             | Value                                                         |
|------------------------------------|---------------------------------------------------------------|
| `getMessage()`                     | The message, which is the Go error text for a binding failure |
| `getCode()`                        | The constructed code, or 0 for an error that carries none     |
| `getPrevious()`                    | The wrapped error, or null                                    |
| `getFile()`, `getLine()`           | Empty and 0; a throwable does not carry a source position     |
| `getTrace()`, `getTraceAsString()` | An empty array and `#0 {main}`; the runtime records no stack  |

## Errors from Go

An error a Go function returned, and a panic recovered at the host-call boundary, enter the same catch path as an explicit `throw`. Neither is an instance of a PHP class and every clause takes one: a binding failure reaches the `catch (Exception $e)` a script already wrote around the call. Without a matching `try`/`catch` the runtime returns the error to the embedding host.

## References

- [Design decisions](../../design.md)
- [tests/fixtures/exceptions/throwable_classes.phpt](../../../tests/fixtures/exceptions/throwable_classes.phpt)
- [tests/fixtures/exceptions/catch_type_matching.phpt](../../../tests/fixtures/exceptions/catch_type_matching.phpt)
