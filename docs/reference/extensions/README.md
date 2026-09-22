# phpscript extensions

| Extension                   | Status              | Notes                                                              |
|-----------------------------|---------------------|--------------------------------------------------------------------|
| `defer()`                   | phpscript extension | Runs callbacks when the current execution frame exits.             |
| Host-backed APIs            | phpscript extension | Bindings such as `Time`, `Database`, `SharedMemory`, and `mail()`. |
| `func` keyword              | phpscript extension | Alias for block-bodied `function`.                                 |
| `fn` keyword                | PHP-incompatible    | Alias for block-bodied `function`, not a PHP arrow function.       |
| Parenthesis-free conditions | PHP-incompatible    | Selected `if` and `foreach` forms can omit parentheses.            |
| `{...}` arrays              | PHP-incompatible    | Braces can delimit an array literal.                               |

These features have no equivalent in the PHP language reference or deliberately use syntax differently. Avoid them when source must also run on PHP.

## Deferred callbacks

`defer()` registers a callback to run when the current function, included file, or top-level file frame exits. Multiple callbacks run in last-in, first-out order, including when the frame exits with an error.

```php
function work() {
    defer(function () { echo "done\n"; });
    echo "working\n";
}
```

A bound method can also be deferred:

```php
$db = new Database("app");
$db->begin();
defer($db->rollback);
```

## Host-backed APIs

### `DateTime` and `Time`

`Time`, `Time\Duration`, and `Time\Location` expose Go's `time.Time`, `time.Duration`, and `time.Location` values directly. Go's package-level functions are registered as `DateTime` statics; the returned `Time` values supply their exported methods automatically:

```php
set_timezone("Europe/Ljubljana");

$start = DateTime::parse("2006-01-02 15:04", "2026-08-26 14:48");
$end = $start->add("30m");

echo $end->format("2006-01-02 15:04 MST");
```

Any PHP argument passed to a Go `time.Duration` parameter may be a duration string accepted by Go's `time.ParseDuration`, such as `"500ms"`, `"30m"`, or `"2h45m"`. A reusable value can be constructed explicitly:

```php
$retention = new Time\Duration("168h");
$expires = $start->add($retention);
```

`new Time\Location($name)` and `Time\Location::load($name)` load an IANA timezone. `set_timezone()` accepts either that value or its name and changes the default used by `new Time`, `DateTime::now()`, `DateTime::parse()`, `DateTime::date()`, and the Unix constructors. The setting belongs to the current runtime; it does not mutate Go's process-wide `time.Local` and cannot leak into another request.

### `Database`

`new Database("name")` connects through the named platform database configured by `PLATFORM_DB_<NAME>` in the process environment or configuration file. It provides `query()`, `get()`, `get_all()`, `insert()`, `replace()`, `update()`, `begin()`, `commit()`, `rollback()`, `insert_id()`, and `rows_affected()`. Database operations automatically add timed database spans to the trace of the request that ran them.

`$db->is_readonly` is a writable property restricting the client to `SELECT`, `SHOW`, `DESCRIBE` and `DESC`: it refuses the write helpers outright and any other statement on the keyword it starts with. It belongs to the client, not the connection, and lives as long as the request does. See the [read-only clients guide](../../use-cases/database.md#read-only-clients).

### `Database\Migrate`

`new Database\Migrate("name")` targets a named platform database, trying the connection `name:migrate` before `name` so migrations can run as a more privileged user without the script naming one. `load($pattern)` selects migration files from the application filesystem, and `run()` applies matching `*.up.sql` files in filename order, recording them under the project `name`. Files are append only: applied statements are recorded by index in a `migrations` table, so statements added at the end of an existing file run on the next start. See the [database guide](../../use-cases/database.md#run-migrations).

### `SharedMemory`

`new SharedMemory` creates a process-local key/value and counter store. An embedding host can place one shared instance in each runtime context to retain state across requests. See the [shared-memory guide](../../use-cases/shared-memory.md).

### `Mail`

`new Mail($name)` selects one of the mail servers the host configured and delivers with `send($recipient, $subject, $body)`. `new Mail` selects `default`.

```php
$mail = new Mail;
$mail->send("hello@example.com", "Subject", "Body");

$campaigns = new Mail("marketing");
$campaigns->send("list@example.com", "Newsletter", "Issue 1");
```

The name is the whole of what a script says about a server. Where the servers come from, what keys they take, and why a script can neither supply nor read a credential are in [Mail servers](../../configuration.md#mail-servers).

Constructing throws when the name is not a configured server, so a typo is caught before a message is composed rather than at the first delivery. A failed delivery throws too, so wrap `send()` in `try`/`catch` when the request should survive an unreachable mail server.

### `mail()`

`mail($recipient, $subject, $body)` sends through the host's `default` server. It is installed on every runtime: with no server configured it still exists and throws catchably, so calling code keeps one spelling and its own fallback.

PHP's `$additional_headers` and `$additional_params` are not accepted.

## Function keyword aliases

All three declarations below mean the same thing to phpscript:

```php
function one() { return 1; }
func two() { return 2; }
fn three() { return 3; }
```

This differs from PHP, where `fn ($x) => $x * 2` is expression-bodied arrow function syntax and `func` is not a keyword. phpscript does not accept the PHP arrow body (`=> expression`). Use `function` for portable source.

## Parenthesis-free conditions

```php
if $ready {
    echo "ready";
}

foreach $items as $item {
    echo $item;
}
```

Standard PHP requires the parentheses. Parenthesized forms remain supported and are recommended for portable scripts.

## Implemented APIs

See the generated [implemented API inventory](implemented-apis.md) for the functions and classes in the standard CLI runtime. Regular-expression shims use Go's RE2 engine rather than PCRE, and filesystem shims are rooted in the host-provided filesystem.

Use `get_defined_functions()`, `get_declared_classes()`, and `get_defined_constants()` to inspect the exact APIs registered by a host.

New bindings follow the [naming conventions](../../naming-conventions.md): PHP's own spelling where PHP defines the call, otherwise a class named for the subject a script works with, as `Database` and `Session\Manager` are.

## Runtime registration

Embedding hosts opt into runtime services separately:

- `stdlib.Register(rt)` installs pure standard-library shims, constants, `Exception`, and every binding package contributed through `runner.RegisterBinding`: `Time`, `Database`, `Database\Migrate`, `Session`, `SharedMemory`, `Mail`, and `start_span`.
- `stdlib.RegisterFS(rt, dir)` adds filesystem operations rooted at `dir`.
- `runner.Options.Mail` names the `model.MailProvider` `Mail` and `mail()` resolve through, the way `Options.Database` names the connections. `mail.NewProvider(servers)` builds one from a configuration block; nil leaves both bindings on a provider holding no servers, which refuses catchably.
- `mail.NewProviderFunc(servers, deliver)` replaces the transport, the way `database.NewDatabaseProvider` takes the connector its pools are opened with. Name resolution and the rule that a script cannot read a credential sit above the seam and are unaffected.
- `mail.NewMemory(names...)` is a provider that queues messages in memory instead of delivering them, which is how tests and dry runs capture mail without a mail server. Naming no servers configures every name.
- `runner.Context.Register(rt)` adds the request-aware header functions and seeds `$_GET`, `$_POST`, `$_COOKIE`, `$_SERVER`, `$_ENV`, `$_REQUEST`, `$_FILES`, `$argv` and `$argc`. See [Predefined variables](../predefined-variables/README.md).

Binding packages under `stdlib/` invert the dependency: each has an `init.go` that calls `runner.RegisterBinding(Register)`, and `stdlib/imports.go` blank-imports them. A host that wants a different set builds its runtime without `stdlib`, or imports the packages it needs and passes extra bindings to `stdlib.Register(rt, bindings...)`.
