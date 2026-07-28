# Expressions

| PHP language-reference feature | Status | Notes |
| --- | --- | --- |
| Literals and variables | Compatibility | Supported values can be used as expressions. |
| Assignment expressions | Partial compatibility | Plain `=` to a variable returns the assigned value; other expression targets/operators are unavailable. |
| Ternary expressions | Compatibility | Full and shorthand (`?:`) forms are supported. |
| Function and method calls | Compatibility | Calls are expressions. |
| `new` expressions | Compatibility | Parentheses are optional when no constructor arguments are supplied. |
| `include` expressions | Partial compatibility | Include forms can occur as statements or expressions; failure behavior is simplified. |
| `match`, `yield`, cloning | Not implemented | These PHP expression forms are unavailable. |

## Assignment expressions

Plain assignment to a variable can appear inside larger expressions, including
conditions. The linter reports assignment inside `if` conditions because it is
easy to mistake for comparison. Array, property, destructuring, and compound
assignment expressions are supported only as standalone statements.

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
`*_once` keywords currently execute the file on every evaluation, so they do
not yet provide PHP's once-only behavior.
