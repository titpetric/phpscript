# Conditions

`if`, `elseif`, and `else` mostly follow PHP block syntax:

```php
if ($foo) {
    echo "yes";
} elseif ($bar === 1) {
    echo "bar";
} else {
    echo "no";
}
```

As a phpscript convenience, `if` conditions may also be unwrapped when the next
token starts the condition and the condition is followed by a block:

```php
if !$foo {
    echo "no";
}
```

Prefer the parenthesized form for portable PHP code. The short unwrapped form is
less portable because standard PHP requires parentheses around `if` conditions.

Assignments inside `if` conditions are parsed for compatibility with existing
PHP code, but they are discouraged. Write the assignment before the condition
instead of using PHP idioms such as assignment-as-test:

```php
// Discouraged; reported by `phpscript lint`:
if ($row = fn()) {
    echo $row;
}

// Use this instead:
$row = fn();
if ($row) {
    echo $row;
}
```

`phpscript lint` reports assignment expressions nested anywhere in an `if`
condition, including checks like `if (($row = fn()) !== false) { ... }`. The
runtime still accepts them so existing PHP code can run while projects migrate
away from the pattern.

## Ternary expressions

Full PHP ternary expressions are supported in conditions and other expression
contexts:

```php
echo $a ? $a : $b;
```

The shorthand/elvis form is also supported. It evaluates to the condition value
when truthy, otherwise to the fallback expression:

```php
echo $a ?: $b;
```
