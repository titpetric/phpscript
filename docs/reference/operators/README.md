# Operators

| PHP language-reference feature | Status                | Notes                                                                                        |
|--------------------------------|-----------------------|----------------------------------------------------------------------------------------------|
| Arithmetic                     | Compatibility         | `+`, `-`, `*`, `/`, `%`, `**`, and unary `+`/`-`.                                            |
| Increment/decrement            | Compatibility         | Prefix and postfix `++` and `--` work on assignable targets.                                 |
| Assignment                     | Compatibility         | `=` and every compound form; see [Mutation](#mutation).                                      |
| Comparison                     | Partial compatibility | `<`, `<=`, `>`, `>=`, `==`, `!=`, `===`, `!==` and `<=>` work, with simplified coercion.     |
| Logical                        | Compatibility         | `!`, `&&`, `\|\|` and the word forms `and`, `or` and `xor`, which bind looser than assignment. |
| Bitwise                        | Compatibility         | The five binary operators and `~`, at PHP's precedence and with PHP's string semantics.      |
| String                         | Compatibility         | `.` and `.=` concatenate values.                                                             |
| Type                           | Compatibility         | `instanceof` compares the class name, and an interface name against what the class declared. |
| Array                          | Compatibility         | `+` on two arrays is their union, keeping the left entry where both hold a key.              |
| Null coalescing                | Partial compatibility | `??` and `??=`; a write to a string offset is refused. [Null coalescing](#null-coalescing). |
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

`$a ?? $b` is `$a` unless it is null or unset, and `$b` is read only then. A missing array key, a missing property, a property of null and a string offset no byte sits at all count as unset, at any depth, and `false`, `0` and `""` are values. The operator is right associative, and `$a ?? $b ?? $c` reads the first one that is set.

An offset past the end of a string, a negative one that reaches before its start and a key that spells no integer are the three a string holds nothing at. `isset()`, `empty()` and `??` read them all as unset, and a plain read of one answers null where php answers the empty string and raises a warning.

`$a ??= $b` assigns `$b` while `$a` is null or unset and leaves `$a` alone otherwise, with no store over a target that holds something: `$s[0] ??= "z"` leaves the string as it stands. The target resolves once on both engines, whether the store happens or not: `$a[k()] ??= 1` calls `k` once, `$t[$i++] ??= 1` steps `$i` once, and `f()->p ??= 1` calls `f` once. `$a[] ??= 1` is a parse error here and in php, where an append slot is write-only. As with every assignment in expression position, a `??=` used as a value takes a variable on its left.

A write to a string offset is not implemented, `??=` included: `$s[1] = "Z"` and `$s[10] ??= "z"` both end the run with `assign: target is not an array`, where php writes the byte and pads the string out to the offset. `tests/fixtures/syntax/null_coalescing.phpt` pins what works.

## Spaceship

`$a <=> $b` is -1, 0 or 1 by the same ordering `<` reads, with PHP's results for arrays, numeric strings, null and bool. A NAN operand answers 1 from either side. Both operands run once, left to right. See [tests/fixtures/syntax/spaceship_operator.phpt](../../../tests/fixtures/syntax/spaceship_operator.phpt).

Two instances of one class compare property by property over the left's own order, and two classes that differ answer 1, php's answer for a pair it cannot order. An instance is greater than an array or a string, and a null on the other side takes the pair into the bool domain. An array against a bool or a null reads as its own emptiness, and `[] == false` and `[1] == true`. [tests/fixtures/comparison/objects.phpt](../../../tests/fixtures/comparison/objects.phpt) pins the two kinds, and `==`, `<` and `sort` read the same table.

One pairing still differs: `1 <=> $instance` answers -1 where php casts the instance to int and answers 0, under a notice of its own.

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
