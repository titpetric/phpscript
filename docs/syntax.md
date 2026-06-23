# Syntax

The phpscript runtime implements a subset of PHP syntax. The syntax
allows for usage of `class`, `new`, `throw`, `catch` and generally
supports PHP expression syntax for conditions, loops, ternary operators
and more.

This is a list of unsupported syntax:

- Namespaces
- Inheritance
- Interfaces, traits, implements
- Public / private variables
- Public / private class methods

Supported syntax:

- Statements in file
- Class + statements in file
- Class in file
- Composition with `include`
- Field and method access

The syntax is in essence a non-OOP version of PHP4 with the extension of
a built in `Exception` type and `try` + `catch` statements for error
handling. This is in contrast with PHP4 `set_error_handler` and
`trigger_error` which are unimplemented APIs.

# Usage examples

Using `phpscript` comes with several syntax adjustments to make code
authorship life a little bit easier. The extensions are incompatible
with PHP, using the form couples you to the runtime.

## Optionals

Parenthesis around conditional and loop statements are removed.

```php
if $id {
	// ...
}

foreach $users as $user {
	// ...
}
```

## Conditions

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

An `if` statement must be followed by a code block wrapped in braces.

```php
// valid php, invalid phpscript
if (!$foo) echo "no";

// invalid php, valid phpscript
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
