# Expressions

| PHP language-reference feature | Status                | Notes                                                                                                   |
|--------------------------------|-----------------------|---------------------------------------------------------------------------------------------------------|
| Literals and variables         | Compatibility         | Supported values can be used as expressions.                                                            |
| Assignment expressions         | Partial compatibility | Plain `=` to a variable returns the assigned value; other expression targets/operators are unavailable. |
| Ternary expressions            | Compatibility         | Full and shorthand (`?:`) forms are supported.                                                          |
| Function and method calls      | Compatibility         | Calls are expressions.                                                                                  |
| `new` expressions              | Compatibility         | Parentheses are optional when no constructor arguments are supplied.                                    |
| `include` expressions          | Partial compatibility | Include forms occur as statements or expressions. A file that cannot be loaded fails the request.       |
| `match`, `yield`, cloning      | Not implemented       | These PHP expression forms are unavailable.                                                             |

## Assignment expressions

Plain assignment to a variable can appear inside larger expressions, including
conditions. The linter reports assignment inside `if` conditions because it is
easy to mistake for comparison. Array, property, destructuring, and compound
assignment expressions are supported only as standalone statements.

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

`include`, `include_once`, `require`, and `require_once` are recognized. The
`*_once` keywords execute a file only when it has not run yet, and the record
they consult covers every include: a plain `include` earlier in the request
already satisfies a later `include_once`.
