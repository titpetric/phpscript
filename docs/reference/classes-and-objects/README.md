# Classes and objects

| PHP language-reference feature    | Status                | Notes                                                                               |
|-----------------------------------|-----------------------|-------------------------------------------------------------------------------------|
| Classes, properties, methods      | Partial compatibility | Basic declarations, construction, `$this`, fields, and method calls are supported.  |
| Constructors                      | Compatibility         | `__construct` is called when present.                                               |
| Class constants                   | Compatibility         | Declaration and `Class::NAME` access are supported.                                 |
| Visibility, final, abstract       | Not enforced          | Modifiers are accepted but do not provide PHP semantics.                            |
| Inheritance and interfaces        | Not implemented       | `extends`, `implements`, interfaces, and traits are unavailable.                    |
| Static methods                    | Compatibility         | `Class::method()`, `self::method()` and `static::method()` are supported.           |
| Static properties                 | Compatibility         | `static $name` declarations and `Class::$name` read/write are supported.            |
| `Class::class`                    | Compatibility         | Resolves to the fully-qualified class name without requiring the class to exist.    |
| Magic methods                     | Partial compatibility | `__construct` and `__invoke` are supported; the wider magic-method contract is not. |
| Enums, anonymous classes, cloning | Not implemented       | These PHP object features are unavailable.                                          |

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

## Host-backed objects

An embedding Go application can register constructors. Returned Go values are
then exposed through the same `new`, method-call, and property-access syntax.
`Database` is the standard example.
