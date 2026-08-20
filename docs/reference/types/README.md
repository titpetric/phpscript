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

Integer literals are written in any of PHP's bases, and the underscore
separator groups digits in all of them. A float takes a fractional part, an
exponent, or both. An integer literal too large for an int becomes a float, as
it does in PHP.

```php
$decimal = 1_000_000;
$hex = 0x1F;        // 31
$binary = 0b1010;   // 10
$octal = 0o17;      // 15, also written 017
$mode = 0644;       // 420, the form a chmod() argument takes
$float = 1.5e3;     // 1500.0
```

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
