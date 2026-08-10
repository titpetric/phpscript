# Types

| PHP language-reference feature           | Status                | Notes                                                                                                |
|------------------------------------------|-----------------------|------------------------------------------------------------------------------------------------------|
| `null`, `bool`, `int`, `float`, `string` | Compatibility         | Scalar values are dynamically typed.                                                                 |
| Arrays                                   | Partial compatibility | Ordered list and associative-array behavior is supported by one runtime array type.                  |
| Objects                                  | Partial compatibility | phpscript and Go-backed objects are supported.                                                       |
| Callable values                          | Partial compatibility | Closures, function names, and bound methods can be invoked by supported APIs.                        |
| Type declarations                        | Not implemented       | Parameter, return, property, union, intersection, and `mixed` declarations are unavailable.          |
| Resources                                | Partial compatibility | Some filesystem APIs return host-backed file values; PHP resource semantics are not general-purpose. |

Values are dynamically typed. The runtime performs PHP-like truthiness and
numeric/string coercion for its supported operators, but it is not a complete
implementation of PHP's conversion rules.

## Scalar types

```php
$nothing = null;
$enabled = true;
$count = 10;
$ratio = 1.5;
$name = "Ada";
```

Integer literals are decimal. PHP hexadecimal, binary, octal, exponent, and
numeric-separator syntax is not implemented.

## Arrays

Both PHP array forms are accepted. phpscript additionally accepts `{...}` as an
array literal.

```php
$list = ["a", "b"];
$map = array("name" => "Ada");
$extended = {"name" => "Ada"}; // phpscript only
```

## Type casting

The casts `(bool)`, `(boolean)`, `(int)`, `(integer)`, `(float)`, `(double)`,
`(real)`, `(string)`, and `(array)` are implemented. `(object)` is parsed but
currently leaves its operand unchanged. PHP's `(unset)` cast is not implemented.
