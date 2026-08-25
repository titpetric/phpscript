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

## Strings

A single-quoted literal is the characters between the quotes, and recognises
only `\'` and `\\`. A double-quoted literal decodes the C-style escapes and the
numeric forms (`\x41`, `\101`, `\u{1F600}`), and evaluates the variables written
into it.

Two spellings embed an expression. Simple syntax is a bare `$name`, a
`$name[key]` subscript, or one level of `$name->prop`:

```php
$name = "Ada";
$row = array("id" => 7);
$user = new User("Ada");

echo "hello $name\n";           // hello Ada
echo "row $row[id]\n";          // row 7, the bare word is a string key
echo "user $user->name\n";      // user Ada
```

A subscript in simple syntax is one token: a bare word, a number, or a variable.
Everything else goes in braces, which re-enter PHP and take an ordinary
expression:

```php
echo "first {$row['id']}\n";
echo "deep {$rows['a']['b']}\n";
echo "call {$user->label()}\n";
```

The braces close on the brace that matches, so a key holding one is safe:
`"{$rows['}']}"` reads the key `}`. A `$` that starts no name is literal text,
`\$` is a dollar, and a single-quoted literal never interpolates. The value of
each embedded expression is converted to a string the way `.` converts it, so
an interpolated literal and the equivalent concatenation produce the same
string.

`${name}` is not accepted. PHP deprecated that spelling in 8.2 and removes it in
9; writing it is reported rather than read, so a literal never prints back a
name the author meant to interpolate.

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
