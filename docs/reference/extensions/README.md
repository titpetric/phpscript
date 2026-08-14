# phpscript extensions

| Extension                   | Status              | Notes                                                        |
|-----------------------------|---------------------|--------------------------------------------------------------|
| `defer()`                   | phpscript extension | Runs callbacks when the current execution frame exits.       |
| Host-backed APIs            | phpscript extension | Contains runtime services such as `Database`.                 |
| `func` keyword              | phpscript extension | Alias for block-bodied `function`.                           |
| `fn` keyword                | PHP-incompatible    | Alias for block-bodied `function`, not a PHP arrow function. |
| Parenthesis-free conditions | PHP-incompatible    | Selected `if` and `foreach` forms can omit parentheses.      |
| `{...}` arrays              | PHP-incompatible    | Braces can delimit an array literal.                         |

These features have no equivalent in the PHP language reference or deliberately
use syntax differently. Avoid them when source must also run on PHP.

## Deferred callbacks

`defer()` registers a callback to run when the current function, included file,
or top-level file frame exits. Multiple callbacks run in last-in, first-out
order, including when the frame exits with an error.

```php
function work() {
    defer(function () { echo "done\n"; });
    echo "working\n";
}
```

A bound method can also be deferred:

```php
$db = new Database("app");
defer($db->close);
```

## Host-backed APIs

### `Database`

`new Database("name")` connects through the named platform database
configured by `PLATFORM_DB_<NAME>` in the process environment or configuration
file. It provides `query()`, `get()`, `get_all()`, `insert()`, `replace()`,
`update()`, `begin()`, `commit()`, `rollback()`, `insert_id()`, and
`rows_affected()`. Database operations automatically add timed database spans to
server-status request traces.

## Function keyword aliases

All three declarations below mean the same thing to phpscript:

```php
function one() { return 1; }
func two() { return 2; }
fn three() { return 3; }
```

This differs from PHP, where `fn ($x) => $x * 2` is expression-bodied arrow
function syntax and `func` is not a keyword. phpscript does not accept the PHP
arrow body (`=> expression`). Use `function` for portable source.

## Parenthesis-free conditions

```php
if $ready {
    echo "ready";
}

foreach $items as $item {
    echo $item;
}
```

Standard PHP requires the parentheses. Parenthesized forms remain supported and
are recommended for portable scripts.

## Implemented APIs

See the generated [implemented API inventory](implemented-apis.md) for the
functions and classes in the standard CLI runtime. Regular-expression shims use
Go's RE2 engine rather than PCRE, and filesystem shims are rooted in the
host-provided filesystem.

Use `get_defined_functions()`, `get_declared_classes()`, and
`get_defined_constants()` to inspect the exact APIs registered by a host.

## Runtime registration

Embedding hosts opt into runtime services separately:

- `stdlib.Register(rt)` installs pure standard-library shims, constants,
  `Exception`, and the `PS` namespace APIs.
- `stdlib.RegisterFS(rt, dir)` adds filesystem operations rooted at `dir`.
- `runner.Context.Register(rt)` adds request-aware header functions and seeds
  `$_GET`, `$_POST`, and `$_PATH`.
