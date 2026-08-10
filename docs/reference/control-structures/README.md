# Control structures

| PHP language-reference feature                  | Status                | Notes                                                                                              |
|-------------------------------------------------|-----------------------|----------------------------------------------------------------------------------------------------|
| `if`, `elseif`, `else`                          | Compatibility         | Brace-delimited branches are supported.                                                            |
| `while`, `for`, `foreach`                       | Compatibility         | Brace-delimited loops are supported.                                                               |
| `switch`                                        | Compatibility         | Case fallthrough and `break` are supported.                                                        |
| `break`, `continue`                             | Partial compatibility | Only the nearest loop/switch can be targeted; numeric levels are unavailable.                      |
| `return`                                        | Compatibility         | A value is optional.                                                                               |
| `include`, `require`                            | Partial compatibility | All four keywords parse, but `include_once` and `require_once` do not suppress repeated execution. |
| `declare`, `do-while`, `goto`, alternate syntax | Not implemented       | These PHP control structures are unavailable.                                                      |
| Parenthesis-free conditions                     | phpscript extension   | `if $value {}` and `foreach $items as $item {}` are accepted.                                      |

## Conditions

```php
if ($foo) {
    echo "yes";
} elseif ($bar) {
    echo "bar";
} else {
    echo "no";
}
```

An `if` body must be wrapped in braces. Prefer parentheses for PHP-portable
source; phpscript also accepts `if $foo { ... }`.

## Loops

```php
foreach ($users as $id => $user) {
    echo $id . ": " . $user;
}

for ($i = 0; $i < 3; $i++) {
    echo $i;
}

while ($ready) {
    break;
}
```

`foreach` assignment targets may be variables, indexes, or properties.
`list(...)` destructuring works in ordinary assignment but not as a `foreach`
target.

## Switch

Cases fall through until `break`, matching PHP. Both `case value:` and
`case value;` separators are accepted.
