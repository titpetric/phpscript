# Operators

| PHP language-reference feature | Status                | Notes                                                                                        |
|--------------------------------|-----------------------|----------------------------------------------------------------------------------------------|
| Arithmetic                     | Compatibility         | `+`, `-`, `*`, `/`, `%`, `**`, and unary `+`/`-`.                                            |
| Increment/decrement            | Compatibility         | Prefix and postfix `++` and `--` work on assignable targets.                                 |
| Assignment                     | Compatibility         | `=` and every compound form; see [Mutation](#mutation).                                      |
| Comparison                     | Partial compatibility | `<`, `<=`, `>`, `>=`, `==`, `!=`, `===`, `!==` and `<=>` work, with simplified coercion.     |
| Logical                        | Compatibility         | `!`, `&&`, `||` and the word forms `and`, `or` and `xor`, which bind looser than assignment. |
| Bitwise                        | Compatibility         | The five binary operators and `~`, at PHP's precedence and with PHP's string semantics.      |
| String                         | Compatibility         | `.` and `.=` concatenate values.                                                             |
| Type                           | Compatibility         | `instanceof` compares the class name, and an interface name against what the class declared. |
| Array                          | Compatibility         | `+` on two arrays is their union, keeping the left entry where both hold a key.              |
| Null coalescing                | Compatibility         | `??` and `??=`; see [Null coalescing](#null-coalescing).                                     |
| Nullsafe (`?->`)               | Not implemented       | `?->` is a parse error.                                                                      |
| Execution operator             | Not implemented       | Backticks are a parse error; there is no shell escape.                                       |
| Error control (`@`)            | Not implemented       | The parser accepts the token, which suppresses no errors.                                    |

## Precedence

From lower to higher precedence, the parser groups operators as follows:

1. `or`
2. `xor`
3. `and`
4. assignment, including `??=`
5. `? :`
6. `??`
7. `||`
8. `&&`
9. `|`
10. `^`
11. `&`
12. `==`, `!=`, `===`, `!==`, `<=>`
13. `<`, `<=`, `>`, `>=`
14. `.`
15. `<<`, `>>`
16. `+`, `-`
17. `*`, `/`, `%`
18. `instanceof`
19. `**`

This is PHP's own grouping: `1 | 2 == 2` folds the comparison first and is 1, and `1 << 2 + 3` shifts by five and is 32.

`**` is right-associative and binds tighter than unary minus and `2 ** 3 ** 2` is 512 and `-2 ** 2` is -4. `instanceof` binds tighter than `!` and `!$e instanceof LogicException` negates the test.

`$ok = f() or g()` assigns first and then tests the result, because the word forms are looser than `=`. `and` and `or` stop at the left operand when it decides the result and `xor` always runs both. All three answer a bool.

## Null coalescing

`$a ?? $b` is `$a` unless it is null or unset, and `$b` is read only then. A missing array key, a missing property and a property of null all count as unset, at any depth, and `false`, `0` and `""` are values. The operator is right associative, so `$a ?? $b ?? $c` reads the first one that is set.

`$a ??= $b` assigns `$b` while `$a` is null or unset and leaves `$a` alone otherwise. The target is read twice, so one that holds a call, `f()['k'] ??= 1`, is a parse error. As with every assignment in expression position, a `??=` used as a value takes a variable on its left.

The bytecode engine declines a program that uses `??`, and the interpreter runs the whole of it. `tests/fixtures/syntax/null_coalescing.phpt` pins the behavior.

## Spaceship

`$a <=> $b` is -1, 0 or 1 by the same ordering `<` reads, with PHP's results for arrays, numeric strings, null and bool. A NAN operand answers 1 from either side. Both operands run once, left to right. See [tests/fixtures/syntax/spaceship_operator.phpt](../../../tests/fixtures/syntax/spaceship_operator.phpt).

## Mutation

Every compound assignment applies its operation to the current value and stores the result and `$a = 5; $a *= 3;` leaves 15. The full set is `+=`, `-=`, `*=`, `/=`, `%=`, `**=`, `.=`, `&=`, `^=`, `<<=`, `>>=` and the bitwise-or form.

```php
$i++;
$total += 2;
$label .= "!";
$flags &= ~2;
$items[] = $label;
```

See [tests/fixtures/arithmetic/compound_assignment.phpt](../../../tests/fixtures/arithmetic/compound_assignment.phpt).
