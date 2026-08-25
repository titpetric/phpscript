# Control structures

| PHP language-reference feature       | Status                | Notes                                                                                                               |
|--------------------------------------|-----------------------|---------------------------------------------------------------------------------------------------------------------|
| `if`, `elseif`, `else`               | Compatibility         | Brace-delimited branches are supported.                                                                             |
| `while`, `for`, `foreach`            | Compatibility         | Brace-delimited loops are supported, including `foreach ($a as &$v)`.                                               |
| `switch`                             | Compatibility         | Case fallthrough and `break` are supported.                                                                         |
| `break`, `continue`                  | Partial compatibility | Only the nearest loop/switch can be targeted; numeric levels are unavailable.                                       |
| `return`                             | Compatibility         | A value is optional.                                                                                                |
| `include`, `require`                 | Compatibility         | All four keywords run, and the `*_once` pair executes a file only when it has not run yet.                          |
| `declare`                            | Parsed, ignored       | Directives are read and dropped; a block form runs its body. See [Basic syntax](../basic-syntax/README.md#declare). |
| `do-while`, `goto`, alternate syntax | Not implemented       | These PHP control structures are a parse error.                                                                     |
| Parenthesis-free conditions          | phpscript extension   | `if $value {}` and `foreach $items as $item {}` are accepted.                                                       |

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

`foreach ($a as &$v)` binds the element rather than a copy of it, so writing to
`$v` edits `$a`. Only the value half may take `&`; a key cannot. See
[Value semantics](../types/value-semantics.md#foreach-binds-a-copy-or-the-element)
for what the two spellings cost and where they stop matching PHP.

## Switch

Cases fall through until `break`, matching PHP. Both `case value:` and
`case value;` separators are accepted.
