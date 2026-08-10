# Predefined interfaces and classes

| PHP predefined interface or class                   | Status                | Notes                                                                           |
|-----------------------------------------------------|-----------------------|---------------------------------------------------------------------------------|
| `Exception`                                         | Partial compatibility | A host-backed exception value can be constructed and thrown.                    |
| `Closure`                                           | Partial compatibility | Anonymous functions produce callable runtime values, without the PHP class API. |
| `stdClass`                                          | Not implemented       | `(object)` is parsed but currently leaves its operand unchanged.                |
| `Throwable`, `Traversable`, `Iterator`, `Countable` | Not implemented       | Interfaces and PHP's exception hierarchy are unavailable.                       |
| `ArrayAccess`, `Serializable`, `Stringable`         | Not implemented       | Interfaces are unavailable.                                                     |
| `Generator`, `Fiber`, `WeakReference`, `WeakMap`    | Not implemented       | Corresponding language/runtime features are unavailable.                        |
| Enum interfaces and predefined attributes           | Not implemented       | Enums and attributes are unavailable.                                           |

## `Exception`

```php
throw new Exception("message");
```

`Exception` exists to carry an error through phpscript's unified runtime error
path. It does not implement PHP's complete exception methods or inheritance
model.

## Runtime class discovery

`get_declared_classes()` returns user-defined and host-registered classes,
including `Exception` and `PS\Database` when the standard library is registered.
