# Expressions

| PHP language-reference feature | Status                | Notes                                                                                               |
|--------------------------------|-----------------------|-----------------------------------------------------------------------------------------------------|
| Literals and variables         | Compatibility         | A supported value is an expression.                                                                 |
| Assignment expressions         | Compatibility         | An assignment answers what it stored, over every target and operator.                               |
| Ternary expressions            | Compatibility         | The runtime evaluates the full and the shorthand (`?:`) form.                                       |
| Function and method calls      | Compatibility         | Calls are expressions.                                                                              |
| `new` expressions              | Compatibility         | Parentheses are optional when `new` takes no constructor arguments.                                 |
| `include` expressions          | Partial compatibility | Include forms occur as statements or expressions. A file the runtime cannot load fails the request. |
| `match`, cloning               | Not implemented       | These PHP expression forms are unavailable.                                                         |
| `yield`                        | Won't build           | Generators are a decision. See [Design decisions](../../design.md#wont-build).                      |

## Assignment expressions

An assignment can appear inside a larger expression, including a condition, and answers what it stored: `$x = $a['k'] = 1` leaves 1 in both, `$x = $counts['k'] += 2` answers the updated count, and a chain stores outside in. The linter reports assignment inside `if` conditions, where `=` is easy to mistake for a comparison.

The target resolves before the value, php's order: `$m[$i++] = $i` reads the key first and stores the stepped `$i` under it. See [tests/fixtures/syntax/assignment_as_value.phpt](../../../tests/fixtures/syntax/assignment_as_value.phpt).

```php
if (($row = load_row()) !== false) {
    echo $row;
}
```

Assigning first reads the same and the linter has nothing to report:

```php
$row = load_row();
if ($row !== false) {
    echo $row;
}
```

## Ternary expressions

```php
$label = $name ? $name : "anonymous";
$label = $name ?: "anonymous";
```

## Includes

The parser recognizes `include`, `include_once`, `require`, and `require_once`. The `*_once` keywords execute a file only when it has not run yet, and the record they consult covers every include: a plain `include` earlier in the request already satisfies a later `include_once`.
