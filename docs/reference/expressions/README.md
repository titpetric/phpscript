# Expressions

| PHP language-reference feature | Status                | Notes                                                                                                   |
|--------------------------------|-----------------------|---------------------------------------------------------------------------------------------------------|
| Literals and variables         | Compatibility         | A supported value is an expression.                                                                     |
| Assignment expressions         | Partial compatibility | Plain `=` to a variable returns the assigned value; other expression targets/operators are unavailable. |
| Ternary expressions            | Compatibility         | The runtime evaluates the full and the shorthand (`?:`) form.                                           |
| Function and method calls      | Compatibility         | Calls are expressions.                                                                                  |
| `new` expressions              | Compatibility         | Parentheses are optional when `new` takes no constructor arguments.                                     |
| `include` expressions          | Partial compatibility | Include forms occur as statements or expressions. A file the runtime cannot load fails the request.     |
| `match`, cloning               | Not implemented       | These PHP expression forms are unavailable.                                                             |
| `yield`                        | Won't build           | Generators are a decision. See [Design decisions](../../design.md#wont-build).                          |

## Assignment expressions

Plain assignment to a variable can appear inside larger expressions, including conditions. The linter reports assignment inside `if` conditions, where `=` is easy to mistake for a comparison. Array, property, destructuring, and compound assignment expressions run only as standalone statements.

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
