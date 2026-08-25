# Classes and objects

| PHP language-reference feature    | Status                | Notes                                                                                                                                                                    |
|-----------------------------------|-----------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Classes, properties, methods      | Partial compatibility | Basic declarations, construction, `$this`, fields, and method calls are supported.                                                                                       |
| Constructors                      | Compatibility         | `__construct` is called when present.                                                                                                                                    |
| Class constants                   | Compatibility         | Declaration and `Class::NAME` access are supported.                                                                                                                      |
| Visibility, final, abstract       | Not enforced          | Modifiers are accepted but do not provide PHP semantics.                                                                                                                 |
| Inheritance                       | Partial compatibility | `extends` names a parent that a catch clause and `instanceof` filter on; no member is inherited through it.                                                              |
| Interfaces and traits             | Partial compatibility | `interface` declarations and `implements` are a contract: a class must declare every method its interfaces name, and inherits nothing from them. Traits are unavailable. |
| Static methods                    | Compatibility         | `Class::method()`, `self::method()` and `static::method()` are supported.                                                                                                |
| Static properties                 | Compatibility         | `static $name` declarations and `Class::$name` read/write are supported.                                                                                                 |
| `Class::class`                    | Compatibility         | Resolves to the fully-qualified class name without requiring the class to exist.                                                                                         |
| Magic methods                     | Partial compatibility | `__construct` and `__invoke` are supported; the wider magic-method contract is not.                                                                                      |
| `instanceof`                      | Compatibility         | Tests a value against a class name, following `extends`.                                                                                                                 |
| Enums, anonymous classes, cloning | Not implemented       | These PHP object features are unavailable.                                                                                                                               |

## Declaring a class

```php
class User
{
    var $name = "";
    const KIND = "user";

    function __construct($name) {
        $this->name = $name;
    }

    function label() {
        return $this->name;
    }
}

$user = new User("Ada");
echo $user->label();
```

Properties can be declared with `var` or with a visibility modifier, and may
carry a type hint, but visibility is not enforced and the type is not checked.

## Static members

A `static` property is storage on the class rather than on an instance. Every
instance, and every static call, reads and writes the same value, and it
outlives the object that first set it.

```php
class Registry
{
    private static $entries = array();

    public static function add($name, $value) {
        self::$entries[$name] = $value;
    }

    public static function all() {
        return self::$entries;
    }
}

Registry::add("driver", "sqlite");
echo count(Registry::all());          // 1
echo Registry::class;                 // Registry
```

`self::` and `static::` both resolve to the class of the running method. There
is no inheritance, so late static binding has nothing to bind late to and the
two spellings are equivalent. A static method runs without a receiver: `$this`
is unbound inside it, as it is in PHP.

`Class::method` is also a callable value, so `array($object, "method")`,
`"Class::method"` and `Closure::fromCallable(...)` all resolve through the same
lookup; see [Functions](../functions/README.md).

## Interfaces

An interface names method signatures, and a class that declares `implements`
must declare every one of them itself. That is the whole of it: the check runs
before the program does, and a class that passes it has exactly the members it
wrote.

```php
interface Reader
{
    function get($key);
    function has($key);
}

interface Listing extends Reader
{
    function keys();
}

class Store implements Listing
{
    function get($key) { return ""; }
    function has($key) { return false; }
    function keys() { return array(); }
}
```

`interface A extends B, C` widens the contract: the names a class is checked
against are the union of what every listed interface declares. Nothing is
inherited, because an interface declares no body and holds no storage.

A missing method is reported by `phpscript lint` and raises a
`RuntimeException` at run time, naming the class, the interface and the method.
A name no `interface` declaration in the same file defines is not a contract and
is not checked, which is what makes `implements Countable` load: phpscript does
not declare PHP's built-in interfaces.

`instanceof` does not consult an interface. It is class-name equality, so
`$store instanceof Reader` is false and `$store instanceof Store` is true.
Constants declared on an interface are parsed and printed back, and reading one
through `Interface::NAME` is not implemented.

## Host-backed objects

An embedding Go application can register constructors. Returned Go values are
then exposed through the same `new`, method-call, and property-access syntax.
`Database` is the standard example.

## References

- [Design decisions](../../design.md), for why there is no inheritance
