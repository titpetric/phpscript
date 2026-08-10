# Operators

| PHP language-reference feature              | Status                | Notes                                                                                      |
|---------------------------------------------|-----------------------|--------------------------------------------------------------------------------------------|
| Arithmetic                                  | Partial compatibility | `+`, `-`, `*`, `/`, `%`, and unary `+`/`-` are supported. Exponentiation is unavailable.   |
| Increment/decrement                         | Compatibility         | Prefix and postfix `++` and `--` work on assignable targets.                               |
| Assignment                                  | Partial compatibility | `=`, `+=`, `-=`, and `.=` are supported; other compound forms are unavailable.             |
| Comparison                                  | Partial compatibility | `<`, `<=`, `>`, `>=`, `==`, `!=`, `===`, and `!==` are supported with simplified coercion. |
| Logical                                     | Partial compatibility | `!`, `&&`, and `                                                                           |
| String                                      | Compatibility         | `.` and `.=` concatenate values.                                                           |
| Array, bitwise, execution, type, functional | Not implemented       | PHP's operators in these groups are unavailable.                                           |
| Error control (`@`)                         | Not implemented       | The token is accepted but does not suppress errors.                                        |

## Precedence

From lower to higher precedence, binary operators are grouped as follows:

1. `||`
2. `&&`
3. `==`, `!=`, `===`, `!==`
4. `<`, `<=`, `>`, `>=`
5. `.`
6. `+`, `-`
7. `*`, `/`, `%`

Use parentheses whenever PHP and phpscript precedence may differ.

## Mutation

```php
$i++;
$total += 2;
$label .= "!";
$items[] = $label;
```

`*=`, `/=`, and `%=` are accepted but do not apply their arithmetic operation;
do not use them. Write the explicit assignment instead.
