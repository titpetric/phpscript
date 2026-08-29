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

`Time`, `Time\Duration`, and `Time\Location` expose Go's `time.Time`,
`time.Duration`, and `time.Location` values directly. Go's package-level
functions are registered as `DateTime` statics; the returned `Time` values
supply their exported methods automatically:

```php
set_timezone("Europe/Ljubljana");

$start = DateTime::parse("2006-01-02 15:04", "2026-08-26 14:48");
$end = $start->add("30m");

echo $end->format("2006-01-02 15:04 MST");
```

Any PHP argument passed to a Go `time.Duration` parameter may be a duration
string accepted by Go's `time.ParseDuration`, such as `"500ms"`, `"30m"`, or
`"2h45m"`. A reusable value can be constructed explicitly:

```php
$retention = new Time\Duration("168h");
$expires = $start->add($retention);
```

`new Time\Location($name)` and `Time\Location::load($name)` load an IANA
timezone. `set_timezone()` accepts either that value or its name and changes
the default used by `new Time`, `DateTime::now()`, `DateTime::parse()`,
`DateTime::date()`, and the Unix constructors. The setting belongs to the
current runtime; it does not mutate Go's process-wide `time.Local` and cannot
leak into another request.

### `Database`

`new Database("name")` connects through the named platform database configured by `PLATFORM_DB_<NAME>` in the process environment or configuration file. It provides `query()`, `get()`, `get_all()`, `insert()`, `replace()`, `update()`, `begin()`, `commit()`, `rollback()`, `insert_id()`, and `rows_affected()`. Database operations automatically add timed database spans to the trace of the request that ran them.

`$db->is_readonly` is a writable property restricting the client to `SELECT`, `SHOW`, `DESCRIBE` and `DESC`: it refuses the write helpers outright and any other statement on the keyword it starts with. It belongs to the client, not the connection, and lives as long as the request does. See the [read-only clients guide](../../use-cases/database.md#read-only-clients).

### `Database\Migrate`

`new Database\Migrate("name")` targets a named platform database, trying the connection `name:migrate` before `name` so migrations can run as a more privileged user without the script naming one. `load($pattern)` selects migration files from the application filesystem, and `run()` applies matching `*.up.sql` files in filename order, recording them under the project `name`. Files are append only: applied statements are recorded by index in a `migrations` table, so statements added at the end of an existing file run on the next start. See the [database guide](../../use-cases/database.md#run-migrations).

### `SharedMemory`

`new SharedMemory` creates a process-local key/value and counter store. An embedding host can place one shared instance in each runtime context to retain state across requests. See the [shared-memory guide](../../use-cases/shared-memory.md).

### `SMTP`

`new SMTP` creates a sender from script-supplied connection settings and delivers with `send($recipient, $subject, $body)`. Authentication (PLAIN) is used when both `username` and `password` are set; `port` defaults to 25. `from` may carry a display name, which becomes the message's `From` header while the bare address is used as the envelope sender.

```php
$smtp = new SMTP(array(
	"host" => "smtp.example.com",
	"port" => 587,
	"username" => "noreply@example.com",
	"password" => "secret",
	"from" => "Example Robot <noreply@example.com>",
));
$smtp->send("hello@example.com", "Subject", "Body");
```

The connection is upgraded with STARTTLS whenever the server offers it. `insecure => true` accepts the certificate without verifying its chain or names, which is what a host with a self-signed certificate requires, or one carrying no `subjectAltName`, the case Go reports as `certificate is not valid for any names`. The session stays encrypted, but it is no longer protected against a man in the middle, so prefer a certificate the host can verify.

A failed delivery throws, so wrap the call in `try`/`catch` when the request should survive an unreachable mail server.

### `mail()`

The optional SMTP binding also exposes the bare `mail($recipient, $subject, $body)` function when an embedding host registers `stdlib/smtp` with a configured sender. Unlike `SMTP`, it is not installed by the standard CLI runtime.

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

- `stdlib.Register(rt)` installs pure standard-library shims, constants, `Exception`, and every binding package contributed through `runner.RegisterBinding`: `Time`, `Database`, `Database\Migrate`, `Session`, `SharedMemory`, `SMTP`, and `start_span`.
- `stdlib.RegisterFS(rt, dir)` adds filesystem operations rooted at `dir`.
- `smtp.Register(rt, sender)` adds the bare `mail()` SMTP binding for a host-configured sender.
- `smtp.SenderContext(ctx, sender)` makes `new SMTP` deliver through `sender` instead of dialing its configured host. `smtp.NewMemory()` is a sender that queues messages in memory, which is how tests and dry runs capture mail without a mail server.
- `runner.Context.Register(rt)` adds the request-aware header functions and seeds `$_GET`, `$_POST`, `$_COOKIE`, `$_SERVER`, `$_ENV`, `$_REQUEST`, `$_FILES`, `$argv` and `$argc`. See [Predefined variables](../predefined-variables/README.md).

Binding packages under `stdlib/` invert the dependency: each has an `init.go` that calls `runner.RegisterBinding(Register)`, and `stdlib/imports.go` blank-imports them. A host that wants a different set builds its runtime without `stdlib`, or imports the packages it needs and passes extra bindings to `stdlib.Register(rt, bindings...)`.
