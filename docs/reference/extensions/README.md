# phpscript extensions

| Extension                   | Status              | Notes                                                                               |
|-----------------------------|---------------------|-------------------------------------------------------------------------------------|
| `defer()`                   | phpscript extension | Runs callbacks when the current execution frame exits.                              |
| Host-backed APIs            | phpscript extension | Bare bindings such as `Database`, `Database\Migrate`, `SharedMemory`, and `mail()`. |
| `func` keyword              | phpscript extension | Alias for block-bodied `function`.                                                  |
| `fn` keyword                | PHP-incompatible    | Alias for block-bodied `function`, not a PHP arrow function.                        |
| Parenthesis-free conditions | PHP-incompatible    | Selected `if` and `foreach` forms can omit parentheses.                             |
| `{...}` arrays              | PHP-incompatible    | Braces can delimit an array literal.                                                |

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

### `Database`

`new Database("name")` connects through the named platform database configured by `PLATFORM_DB_<NAME>` in the process environment or configuration file. It provides `query()`, `get()`, `get_all()`, `insert()`, `replace()`, `update()`, `begin()`, `commit()`, `rollback()`, `insert_id()`, and `rows_affected()`. Database operations automatically add timed database spans to the trace of the request that ran them.

`$db->is_readonly` is a writable property restricting the client to `SELECT`, `SHOW`, `DESCRIBE` and `DESC`: it refuses the write helpers outright and any other statement on the keyword it starts with. It belongs to the client, not the connection, and lives as long as the request does. See the [read-only clients guide](../../use-cases/database.md#read-only-clients).

### `Database\Migrate`

`new Database\Migrate("name")` targets a named platform database. `load($pattern)` reads migration files from the application filesystem, and `run()` applies matching `*.up.sql` files in filename order. Files are append only: applied statements are recorded by index in a `migrations` table, so statements added at the end of an existing file run on the next start. See the [database guide](../../use-cases/database.md#run-migrations).

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

New bindings follow [Standard library design conventions](../../stdlib-conventions.md): PHP function names when the call already exists, a single top-level namespace (`HTTP`, `SQL`, `Telemetry`) when the API is ours. `PS\` is not extended.

## Runtime registration

Embedding hosts opt into runtime services separately:

- `stdlib.Register(rt)` installs pure standard-library shims, constants, `Exception`, and every binding package contributed through `runner.RegisterBinding`: `Database`, `Database\Migrate`, `Session`, `SharedMemory`, `SMTP`, and `start_span`.
- `stdlib.RegisterFS(rt, dir)` adds filesystem operations rooted at `dir`.
- `smtp.Register(rt, sender)` adds the bare `mail()` SMTP binding for a host-configured sender.
- `smtp.SenderContext(ctx, sender)` makes `new SMTP` deliver through `sender` instead of dialing its configured host. `smtp.NewMemory()` is a sender that queues messages in memory, which is how tests and dry runs capture mail without a mail server.
- `runner.Context.Register(rt)` adds request-aware header functions and seeds `$_GET`, `$_POST`, and `$_PATH`.

Binding packages under `stdlib/` invert the dependency: each has an `init.go` that calls `runner.RegisterBinding(Register)`, and `stdlib/imports.go` blank-imports them. A host that wants a different set builds its runtime without `stdlib`, or imports the packages it needs and passes extra bindings to `stdlib.Register(rt, bindings...)`.
