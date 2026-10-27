# Control structures

| PHP language-reference feature | Status                | Notes                                                                                                                     |
|--------------------------------|-----------------------|---------------------------------------------------------------------------------------------------------------------------|
| `if`, `elseif`, `else`         | Compatibility         | Brace-delimited branches run.                                                                                             |
| `while`, `for`, `foreach`      | Compatibility         | Brace-delimited loops run, including `foreach ($a as &$v)`.                                                               |
| `do-while`                     | Compatibility         | The body runs before the condition is first checked; `break` and `continue` behave as in the other loops.                 |
| `switch`                       | Compatibility         | Cases fall through until `break`.                                                                                         |
| `break`, `continue`            | Partial compatibility | Each targets the nearest loop/switch only; numeric levels are unavailable.                                                |
| `return`                       | Compatibility         | A value is optional.                                                                                                      |
| `include`, `require`           | Compatibility         | All four keywords run, and the `*_once` pair executes a file only when it has not run yet.                                |
| `declare`                      | Parsed, ignored       | The parser drops a directive it reads; a block form runs its body. See [Basic syntax](../basic-syntax/README.md#declare). |
| `goto`                         | Won't build           | A decision. See [Design decisions](../../design.md#wont-build).                                                           |
| Alternate syntax               | Not implemented       | `endif`, `endforeach` and the rest are a parse error.                                                                     |
| Parenthesis-free conditions    | phpscript extension   | The parser accepts `if $value {}` and `foreach $items as $item {}`.                                                       |

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

An `if` body requires braces. Prefer parentheses for PHP-portable source; phpscript also accepts `if $foo { ... }`.

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

do {
    $attempts++;
} while ($attempts < 3);
```

`do-while` runs the body before the condition is first checked and it executes at least once. `continue` jumps to the condition check.

`foreach` assignment targets may be variables, indexes, or properties. `list(...)` destructuring works in ordinary assignment but not as a `foreach` target.

`foreach ($a as &$v)` binds the element itself and writing to `$v` edits `$a`. Only the value half may take `&`; a key cannot. See [Value semantics](../types/value-semantics.md#foreach-binds-a-copy-or-the-element) for what the two spellings cost and where they stop matching PHP.

## Switch

Cases fall through until `break`, matching PHP. The parser accepts both `case value:` and `case value;` separators.
