# Predefined interfaces and classes

| PHP predefined interface or class                | Status                | Notes                                                                                                                    |
|--------------------------------------------------|-----------------------|--------------------------------------------------------------------------------------------------------------------------|
| `Exception`, `Error` and the SPL classes         | Compatibility         | Constructed and thrown; a catch clause filters on the class name recorded.                                               |
| `Closure`                                        | Partial compatibility | Anonymous functions produce callable runtime values, without the PHP class API.                                          |
| `stdClass`                                       | Compatibility         | `new stdClass` and the `(object)` cast both produce one; it declares nothing and every property is added by assignment.  |
| `Throwable`                                      | Partial compatibility | `catch (Throwable $e)` takes any failure. `instanceof Throwable` is false, and the name is not declared as an interface. |
| `Traversable`, `Iterator`, `Countable`           | Not implemented       | The names are not declared, so `implements Countable` is accepted and unchecked, and nothing dispatches through them.    |
| `ArrayAccess`, `Serializable`, `Stringable`      | Not implemented       | As above: the names are not declared. Write the methods and call them by name.                                           |
| `Generator`, `Fiber`, `WeakReference`, `WeakMap` | Not implemented       | Corresponding language/runtime features are unavailable.                                                                 |
| Enum interfaces and predefined attributes        | Not implemented       | Enums and attributes are unavailable.                                                                                    |

## `Exception`

```php
throw new Exception("message");
```

Every SPL exception and error name is registered and constructible, all of them
one type carrying the name the script used. A catch clause filters on that name;
none of them is a subclass of another. See [Exceptions](../exceptions/README.md).

## Runtime class discovery

`get_declared_classes()` returns user-defined and host-registered classes,
including `Exception` and `Database` when the standard library is registered.
