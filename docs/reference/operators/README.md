# Operators

| PHP language-reference feature | Status                | Notes                                                                                        |
|--------------------------------|-----------------------|----------------------------------------------------------------------------------------------|
| Arithmetic                     | Compatibility         | `+`, `-`, `*`, `/`, `%`, `**`, and unary `+`/`-`.                                            |
| Increment/decrement            | Compatibility         | Prefix and postfix `++` and `--` work on assignable targets.                                 |
| Assignment                     | Compatibility         | `=` and every compound form; see [Mutation](#mutation).                                      |
| Comparison                     | Partial compatibility | `<`, `<=`, `>`, `>=`, `==`, `!=`, `===`, and `!==` are supported with simplified coercion.   |
| Logical                        | Partial compatibility | `!` and the two symbol forms. The word forms `and`, `or` and `xor` are a parse error.        |
| Bitwise                        | Compatibility         | The five binary operators and `~`, at PHP's precedence and with PHP's string semantics.      |
| String                         | Compatibility         | `.` and `.=` concatenate values.                                                             |
| Type                           | Compatibility         | `instanceof` compares the class name, and an interface name against what the class declared. |
| Array                          | Compatibility         | `+` on two arrays is their union, keeping the left entry where both hold a key.              |
| Null coalescing and spaceship  | Not implemented       | `??`, `??=` and `<=>` are a parse error.                                                     |
| Execution operator             | Not implemented       | Backticks are a parse error; there is no shell escape.                                       |
| Error control (`@`)            | Not implemented       | The token is accepted but does not suppress errors.                                          |

## Precedence

From lower to higher precedence, binary operators are grouped as follows:

1. `||`
2. `&&`
3. `|`
4. `^`
5. `&`
6. `==`, `!=`, `===`, `!==`
7. `<`, `<=`, `>`, `>=`
8. `.`
9. `<<`, `>>`
10. `+`, `-`
11. `*`, `/`, `%`
12. `instanceof`
13. `**`

This is PHP's own grouping: `1 | 2 == 2` folds the comparison first and is 1,
and `1 << 2 + 3` shifts by five and is 32.

`**` is right-associative and binds tighter than unary minus, so `2 ** 3 ** 2`
is 512 and `-2 ** 2` is -4. `instanceof` binds tighter than `!`, so
`!$e instanceof LogicException` negates the test.

## Mutation

Every compound assignment applies its operation to the current value and stores
the result, so `$a = 5; $a *= 3;` leaves 15. The full set is `+=`, `-=`, `*=`,
`/=`, `%=`, `**=`, `.=`, `&=`, `^=`, `<<=`, `>>=` and the bitwise-or form.

```php
$i++;
$total += 2;
$label .= "!";
$flags &= ~2;
$items[] = $label;
```

See
[tests/fixtures/arithmetic/compound_assignment.phpt](../../../tests/fixtures/arithmetic/compound_assignment.phpt).
