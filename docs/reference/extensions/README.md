# phpscript extensions

| Extension                   | Status              | Notes                                                              |
|-----------------------------|---------------------|--------------------------------------------------------------------|
| `defer()`                   | phpscript extension | Runs callbacks when the current execution frame exits.             |
| Host-backed APIs            | phpscript extension | Bindings such as `Time`, `Database`, `SharedMemory`, and `mail()`. |
| `func` keyword              | phpscript extension | Alias for block-bodied `function`.                                 |
| `fn` keyword                | PHP-incompatible    | Alias for block-bodied `function`, not a PHP arrow function.       |
| Parenthesis-free conditions | PHP-incompatible    | Selected `if` and `foreach` forms can omit parentheses.            |
| `{...}` arrays              | PHP-incompatible    | Braces can delimit an array literal.                               |

These features have no match in the PHP language reference, or use syntax differently. Avoid them when source must also run on PHP.

## Deferred callbacks

`defer()` registers a callback to run when the current function, included file, or top-level file frame exits. Callbacks run in last-in, first-out order, including when the frame exits with an error.

```php
function work() {
    defer(function () { echo "done\n"; });
    echo "working\n";
}
```

`defer()` also takes a bound method:

```php
$db = new Database("app");
$db->begin();
defer($db->rollback);
```

## Host-backed APIs

### `DateTime` and `Time`

`Time`, `Time\Duration`, and `Time\Location` expose Go's `time.Time`, `time.Duration`, and `time.Location` values directly. The registration installs Go's package-level functions, each a static on `DateTime`; the returned `Time` values supply their exported methods automatically:

```php
set_timezone("Europe/Ljubljana");

$start = DateTime::parse("2006-01-02 15:04", "2026-08-26 14:48");
$end = $start->add("30m");

echo $end->format("2006-01-02 15:04 MST");
```

Any PHP argument passed to a Go `time.Duration` parameter may be a duration string accepted by Go's `time.ParseDuration`, such as `"500ms"`, `"30m"`, or `"2h45m"`. A script can construct a reusable value explicitly:

```php
$retention = new Time\Duration("168h");
$expires = $start->add($retention);
```

`new Time\Location($name)` and `Time\Location::load($name)` load an IANA timezone. `set_timezone()` accepts either that value or its name and changes the default used by `new Time`, `DateTime::now()`, `DateTime::parse()`, `DateTime::date()`, and the Unix constructors. The setting belongs to the current runtime; it does not mutate Go's process-wide `time.Local` and cannot leak into another request.

### `Database`

`new Database("name")` connects through the named platform database configured by `PLATFORM_DB_<NAME>` in the process environment or configuration file. It declares `query()`, `get()`, `get_all()`, `insert()`, `replace()`, `update()`, `begin()`, `commit()`, `rollback()`, `insert_id()`, and `rows_affected()`. Database operations automatically add timed database spans to the trace of the request that ran them.

`$db->is_readonly` is a writable property restricting the client to `SELECT`, `SHOW`, `DESCRIBE` and `DESC`: it refuses the write helpers outright and any other statement on the keyword it starts with. It belongs to the client, not the connection, and lives as long as the request does. See the [read-only clients guide](../../use-cases/database.md#read-only-clients).

### `Database\Migrate`

`new Database\Migrate("name")` targets a named platform database, trying the connection `name:migrate` before `name` so migrations can run as a more privileged user without the script naming one. `load($pattern)` selects migration files from the application filesystem, and `run()` applies matching `*.up.sql` files in filename order, recording them under the project `name`. Files are append only: `run()` records applied statements by index in a `migrations` table, so statements added at the end of an existing file run on the next start. See the [database guide](../../use-cases/database.md#run-migrations).

### `SharedMemory`

`new SharedMemory` creates a process-local key/value and counter store. An embedding host can place one shared instance in each runtime context to keep state across requests. See the [shared-memory guide](../../use-cases/shared-memory.md).

### `Mail`

`new Mail($name)` selects one of the mail servers the host configured and sends with `send($recipient, $subject, $body, $html, $reply_to)`, the last two optional. `new Mail` selects `default`.

```php
$mail = new Mail;
$mail->send("hello@example.com", "Subject", "Body");

$campaigns = new Mail("marketing");
$campaigns->send("list@example.com", "Newsletter", "Issue 1", "<h1>Issue 1</h1>", "editor@example.com");
```

A non-empty `$html` sends a `multipart/alternative` message with `$body` as its text part; `$reply_to` is one address and throws on anything else. [HTML and a Reply-To](../../use-cases/mail.md#html-and-a-reply-to) has the details.

The name is the whole of what a script says about a server. Where the servers come from, what keys they take, and why a script can neither supply nor read a credential are in [Mail servers](../../configuration.md#mail-servers).

Constructing throws when the name is not a configured server, so the constructor catches a typo before a script composes a message, ahead of the first delivery. A failed delivery throws too, so wrap `send()` in `try`/`catch` when the request should survive an unreachable mail server.

### `mail()`

`mail($recipient, $subject, $body)` sends through the host's `default` server. The registration installs it on every runtime: with no server configured it still exists and throws catchably, so calling code keeps one spelling and its own fallback.

PHP's `$additional_headers` and `$additional_params` are not accepted.

### `Session\Manager`

`new Session\Manager` is a login, a session cookie and the storage behind it. It replaces PHP's `session_start` family, which this runtime leaves out; Go writes the cookie with `HttpOnly`, `Path=/` and `SameSite=Lax` fixed, so a script neither chooses the value nor weakens the attributes. [Cookies](../../design.md#cookies) is why `setcookie` is absent.

With no arguments it takes the two defaults a site wants: `Session\Storage\Disk` for the sessions, and `Auth\Storage\PasswordFile` over the `.htpasswd` in the application root for the accounts. A whole login is then the manager:

```php
$session = new Session\Manager;

if ($session->login($_POST["username"], $_POST["password"])) {
    header("Location: /");   // the session now carries the user's id
}

if (!$session->valid()) {
    header("Location: /login");
}

echo $session->get();        // the signed-in user's primary key
$session->destroy();         // log out: the stored session is deleted, the cookie expires
```

`login()` reports whether the credential held, and a wrong password is `false` and no error. `start($id)` is the other way in, for an application that identifies its visitor by some other means. `get()` answers the stored id, `valid()` whether the request carries a live session, and `destroy()` ends it.

Either half is replaceable, by constructor argument or by setter:

```php
$session_store = new Session\Storage\Memory;
$user_store    = new Auth\Storage\PasswordFile("config/users");

$session = new Session\Manager($session_store, $user_store);

$other = new Session\Manager;
$other->set_storage($session_store);
$other->set_auth($user_store);
```

`Session\Storage\Disk($path)` keeps a file per session under a directory inside the application root; with no path it uses one of the application's own below the host's temporary directory, which survives a reload. `Session\Storage\Memory` lives as long as the runtime, and a server forks one per worker, so it belongs to a CLI run or a test; a served site takes the disk one. A disk storage the runtime cannot create falls back to memory with a warning naming the path, and every session span carries the backend that answered.

### `Auth\Storage\PasswordFile`

`new Auth\Storage\PasswordFile($file)` is the user database a login puts its credential to: an apache-format `htpasswd` file of `username:hash` lines, resolved inside the application root, `.htpasswd` when the script names no path. Usernames are lowercase and compared as written.

```php
$users = new Auth\Storage\PasswordFile;

if (!$users->exists()) {
    echo "No accounts: write .htpasswd with htpasswd -B -C 10 .htpasswd admin";
}

$user = $users->authenticate("admin", $_POST["password"]);
if ($user !== null) {
    echo "signed in as $user, key " . $user->get_id();
}

foreach ($users->users() as $row) {
    echo $row->get_id(), " ", $row, "\n";
}
```

`authenticate()` answers a record or `null`, and a wrong password, an unknown name and a name in the wrong case are the same `null`. A record carries two things: `get_id()`, a ULID computed from the username, and its string form, which is the username - so `echo $user` and a `string` parameter both read the name with nothing spelled at the call site.

The key follows from the name and sits in no column, so the file is a table other tables can reference: removing an account takes its row and nothing else, and adding the same name back answers the same key. `exists()` reports whether there is a file to read at all, so a default store does not fail a tree that has no accounts, and `users()` is the file as a listing of keys and names.

No hash reaches a script, by construction: the store holds a path, every call reads the file, and no method returns a stored value. The formats a line may carry, the ones the parse refuses, and what each costs an attacker are in [security.md](../../security.md#the-user-database).

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

Standard PHP requires the parentheses. Parenthesized forms remain supported; use them for portable scripts.

## Implemented APIs

See the generated [implemented API inventory](implemented-apis.md) for the functions and classes in the standard CLI runtime. Regular-expression shims use Go's RE2 engine where PHP uses PCRE, and filesystem shims take the host-provided filesystem as their root.

Use `get_defined_functions()`, `get_declared_classes()`, and `get_defined_constants()` to inspect the exact APIs registered by a host.

New bindings follow the [naming conventions](../../naming-conventions.md): PHP's own spelling where PHP defines the call, otherwise a class named for the subject a script works with, as `Database` and `Session\Manager` are.

## Runtime registration

Embedding hosts opt into runtime services separately:

- `stdlib.Register(rt)` installs pure standard-library shims, constants, `Exception`, and every binding package contributed through `runner.RegisterBinding`: `Time`, `Database`, `Database\Migrate`, `Session`, `SharedMemory`, `Mail`, and `start_span`.
- `stdlib.RegisterFS(rt, dir)` adds filesystem operations rooted at `dir`.
- `runner.Options.Mail` names the `model.MailProvider` `Mail` and `mail()` resolve through, the way `Options.Database` names the connections. `mail.NewProvider(servers)` builds one from a configuration block; nil leaves both bindings on a provider holding no servers, which refuses catchably.
- `mail.NewProviderFunc(deliver, servers)` replaces the transport, the way `database.NewDatabaseProvider` takes the connector that opens its pools. Name resolution and the rule that a script cannot read a credential sit above the seam, so neither changes.
- `mail.NewMemory(names...)` is a provider that queues messages in memory and sends nothing, so tests and dry runs capture mail without a mail server. Naming no servers configures every name.
- `runner.Context.Register(rt)` adds the request-aware header functions and seeds `$_GET`, `$_POST`, `$_COOKIE`, `$_SERVER`, `$_ENV`, `$_REQUEST`, `$_FILES`, `$argv` and `$argc`. See [Predefined variables](../predefined-variables/README.md).

Binding packages under `stdlib/` invert the dependency: each has an `init.go` that calls `runner.RegisterBinding(Register)`, and `stdlib/imports.go` blank-imports them. A host needing a different set builds its runtime without `stdlib`, or imports the packages it needs and passes extra bindings to `stdlib.Register(rt, bindings...)`.
