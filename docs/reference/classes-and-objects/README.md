# Classes and objects

| PHP language-reference feature | Status | Notes |
| --- | --- | --- |
| Classes, properties, methods | Partial compatibility | Basic declarations, construction, `$this`, fields, and method calls are supported. |
| Constructors | Compatibility | `__construct` is called when present. |
| Class constants | Compatibility | Declaration and `Class::NAME` access are supported. |
| Visibility, static, final, abstract | Not enforced | Modifiers may be accepted but do not provide PHP semantics. |
| Inheritance and interfaces | Not implemented | `extends`, `implements`, interfaces, and traits are unavailable. |
| Static members and methods | Not implemented | `Class::method()` and static properties are unavailable. |
| Magic methods | Partial compatibility | `__construct` is supported; the wider PHP magic-method contract is not. |
| Enums, anonymous classes, cloning | Not implemented | These PHP object features are unavailable. |

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

Properties can be declared with `var` or with a visibility modifier, but
visibility is not enforced. Methods always behave as instance methods.

## Host-backed objects

An embedding Go application can register constructors. Returned Go values are
then exposed through the same `new`, method-call, and property-access syntax.
`PS\Database` is the standard example.
