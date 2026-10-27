# Classes and objects

| PHP language-reference feature | Status                | Notes                                                                                                                                                                            |
|--------------------------------|-----------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Classes, properties, methods   | Partial compatibility | The runtime supports basic declarations, construction, `$this`, fields, and method calls.                                                                                        |
| Constructors                   | Compatibility         | The runtime calls `__construct` when present.                                                                                                                                    |
| Class constants                | Compatibility         | The runtime supports declaration and `Class::NAME` access.                                                                                                                       |
| Visibility, final, abstract    | Not enforced          | The parser accepts modifiers, which carry no PHP semantics.                                                                                                                      |
| Inheritance                    | Partial compatibility | `extends` names a parent that a catch clause and `instanceof` filter on; a class receives no member through it.                                                                  |
| Interfaces and traits          | Partial compatibility | `interface` declarations and `implements` are a contract: a class must declare every method its interfaces name, and inherits nothing from them. Traits are unavailable.         |
| Static methods                 | Compatibility         | The runtime supports `Class::method()`, `self::method()` and `static::method()`.                                                                                                 |
| Static properties              | Compatibility         | The runtime supports `static $name` declarations and `Class::$name` read/write.                                                                                                  |
| `Class::class`                 | Compatibility         | Resolves to the fully-qualified class name without requiring the class to exist.                                                                                                 |
| Magic methods                  | Partial compatibility | The runtime supports `__construct` and `__invoke`, and no other magic method.                                                                                                    |
| `instanceof`                   | Compatibility         | Tests a value against a class name, following `extends`.                                                                                                                         |
| Anonymous classes              | Compatibility         | `new class [(args)] [implements ...] { ... }` declares and constructs in one expression. `get_class()` returns a synthesized name, and the bytecode engine does not compile one. |
| Enums, cloning                 | Not implemented       | These PHP object features are unavailable.                                                                                                                                       |

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

A class declares a property with `var` or with a visibility modifier, and the declaration may carry a type hint, but visibility is not enforced and the type is not checked.

## Static members

A `static` property is storage on the class, not on an instance. Every instance, and every static call, reads and writes the same value, and it outlives the object that first set it.

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

`self::` and `static::` both resolve to the class of the running method. There is no inheritance: late static binding has nothing to bind late to, and the two spellings resolve to the same class. Inside an instance method, calling a non-static method through `self::method()` forwards the current instance just like `$this->method()` and the called method can read and write `$this`. A genuinely static method still runs without a receiver: `$this` is unbound inside it, as in PHP.

`new self()` and `new static()` are not resolved: both fail loudly as an undefined class, a won't-build recorded in [Design decisions](../../design.md). Spell the class name, `new Registry()`, which is the one class either keyword could ever name here.

`Class::method` is also a callable value and `array($object, "method")`, `"Class::method"` and `Closure::fromCallable(...)` all resolve through the same lookup; see [Functions](../functions/README.md).

## Interfaces

An interface names method signatures, and a class that declares `implements` must declare every one of them itself. The check runs before the program does, and a class that passes it has exactly the members it wrote.

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

`interface A extends B, C` widens the contract: the check tests a class against the union of the names every listed interface declares. A class receives nothing from them, because an interface declares no body and holds no storage.

`phpscript lint` reports a missing method, which raises a `RuntimeException` at run time, naming the class, the interface and the method. A name no `interface` declaration in the same file defines is not a contract and is not checked and `implements Countable` loads: phpscript does not declare PHP's built-in interfaces.

`instanceof` tests the class name and every interface name the class declares, following `extends` on an interface, and `$store instanceof Reader` is true for the example above. A constant declared on an interface reads through `Interface::NAME`; a class that implements the interface acquires none of its constants.

## Host-backed objects

An embedding Go application can register constructors. Returned Go values are then exposed through the same `new`, method-call, and property-access syntax. `Database` is the standard example.

## References

- [Design decisions](../../design.md), for why there is no inheritance
