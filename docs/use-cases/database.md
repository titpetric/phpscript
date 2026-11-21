# Database bindings

The standard runtime registers the Go-backed `Database` class and imports these drivers:

- sqlite - `modernc.org/sqlite`
- mysql - `github.com/go-sql-driver/mysql`
- postgres - `github.com/jackc/pgx/v5/stdlib`

## Configure a connection

Configure a connection in `config.yml` and pass the file to phpscript:

```yaml
env:
  - "PLATFORM_DB_APP=sqlite://app.db"
```

```bash
phpscript -f config.yml script.php
```

The runtime normalizes the part after `PLATFORM_DB_` to a lowercase connection name. A script requests `PLATFORM_DB_APP` as `"app"`. The value uses `<driver>://<dsn>` syntax. Process environment variables with the same prefix are also registered when phpscript starts.

See [Configuration](../configuration.md#database-connections) for more details.

## Create a client

Pass a registered connection name to the constructor. Each `Database` instance uses the shared pool for that connection:

```php
$db = new Database("app");
```

The constructor throws an exception when the connection name is not registered or when opening the pool fails.

## Connections an application owns

Connection names normally come from the environment the host started with, which is right when the operator owns the list. An application that keeps its connections in a table owns the list. `putenv` will not do: it writes to the script environment. The host builds the provider once, before the first request, from the host environment.

`Database::register()` adds one:

```php
Database::register("reports", "postgres://user:pass@host/reports?sslmode=disable");

$db = new Database("reports");
```

`Database::register()` lowercases the name, as the runtime does for a name from `PLATFORM_DB_REPORTS`. Registering a name that already means the same thing does nothing. Registering it with a different DSN closes the old pool. An edited connection takes effect on the next request, without a restart.

Each virtual host has a separate provider. One site cannot reach another site's databases by naming them.

`Database::connections()` returns the names the script can open, sorted:

```php
foreach (Database::connections() as $name) {
	echo $name, "\n";
}
```

The [dbadmin demo](../../demos/dbadmin) uses this. Each row holds a name plus a DSN. `connection_dao::open()` registers the DSN before it constructs a client.

## Read-only clients

`$db->is_readonly` restricts a client to statements that only read. A page that only displays rows cannot accidentally write them:

```php
$db = new Database("app");
$db->is_readonly = true;

$users = $db->get_all("select id, name from users");   // runs
$db->query("delete from users");                       // throws
```

`is_readonly` belongs to the client, not to the connection. All clients share the pool. This client controls the statements it sends down that pool. The property has request scope, like the client holding it. Set the property where a request stops writing. Read it back anywhere:

```php
if (!$db->is_readonly) {
	$db->insert("users", array("name" => "Ada"));
}
```

A read-only client refuses `insert`, `replace` and `update` outright, which write by definition. It refuses `query`, `get` and `get_all` on the keyword the statement starts with, past any comment in front of it. These statements run:

| Statement             | Why                                          |
|-----------------------|----------------------------------------------|
| `SELECT`              | the read                                     |
| `SHOW`                | schema and server introspection              |
| `DESCRIBE` and `DESC` | table introspection, where the engine has it |

The client refuses everything else, including `EXPLAIN`: `EXPLAIN ANALYZE INSERT ...` runs the statement it reports on in PostgreSQL and an explain is not reliably a read. It refuses `PRAGMA` for the same reason: `PRAGMA journal_mode = WAL` writes.

The client throws the refusal as an exception naming the statement. A script can catch the exception and report which call it lost:

```php
try {
	$db->query("drop table users");
} catch (Exception $e) {
	echo $e;   // database is read-only: drop is not allowed
}
```

`begin`, `commit`, `rollback`, `connect` and `close` stay available, since a read-only transaction is a read.

Classification reads the start of the statement alone. A statement must begin with its keyword. The client refuses `(SELECT 1) UNION (SELECT 2)`. A driver that allows more than one statement each call lets a second statement through behind a semicolon.

That is the shape of the whole feature: a boundary for the code holding the client. The script gets no sandbox. The script that set the property can clear it, just as it set the property. A connection that must not write belongs to a database user without the grant. That grant keeps an application's code on the right side, and makes a page that only displays rows say so.

## Run migrations

`Database\Migrate` applies `*.up.sql` migration files to a named connection. This section sets up migrations for an application from scratch. The [dbadmin demo](../../demos/dbadmin) is a working copy of the result.

### Lay out the schema

Keep one migration file per table, named after the table, in a `schema/` directory next to the application:

```text
my-app/
├── migrate.php
├── public/
│   └── index.php
└── schema/
    ├── catalogue.up.sql
    └── users.up.sql
```

`Database\Migrate` applies only files ending in `.up.sql`, in filename order. A file holds the `CREATE TABLE` statement for its table, and any rows the application needs to start with:

```sql
-- catalogue holds the records the application starts with.

CREATE TABLE catalogue (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	category TEXT NOT NULL DEFAULT 'General',
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO catalogue (name, category) VALUES ('SQLite Handbook', 'Books');
```

A semicolon at the end of a line ends a statement, and `--` starts a comment. The migration strips comments before it splits the file and avoid `--` inside string values.

### Run them at startup

Migrations belong in an [`@startup`](../cli/server.md) file and they finish before the server accepts requests. Create `migrate.php`:

```php
<?php

// @startup

$migrate = new Database\Migrate("app");
$migrate->load("./schema/*.up.sql");
$migrate->run();
```

The constructor takes the same connection name as `new Database("app")`, which is `PLATFORM_DB_APP` in the environment. Omitting it selects the connection named `default`. `run()` records the run under that name as its project. Two applications sharing one database keep separate records of their applied statements.

Before `app`, the constructor tries the connection `app:migrate`. A deployment that registers one points migrations at a user allowed to alter tables while the application queries through a user that is not. The script names neither user. An environment variable name holds no colon and `PLATFORM_DB_APP:MIGRATE` will not define `app:migrate`. `Database::register("app:migrate", $dsn)` registers it. Whichever of the two answers, the project stays `app`.

`load()` accepts a glob relative to the runtime working directory, selecting the migration files in the application filesystem. `run()` applies them, recording each migration in a `migrations` table. Later server starts skip statements that already ran.

Loading or applying a migration can raise a PHP exception. An uncaught exception aborts startup, and with telemetry on, the runtime records the startup step and its error as a background trace.

### Change a schema later

Migration files are append only. `run()` records progress per statement index. Adding statements at the end of an existing file changes a table:

```sql
CREATE TABLE catalogue (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	category TEXT NOT NULL DEFAULT 'General',
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO catalogue (name, category) VALUES ('SQLite Handbook', 'Books');

ALTER TABLE catalogue ADD COLUMN notes TEXT;
```

On the next start, only the `ALTER TABLE` runs. Editing or removing a statement that already ran does not re-run it. It leaves databases that applied the old version inconsistent with new ones. Add a table by adding a file.

## Execute queries

Use `query` for statements that do not return rows. Positional arguments bind to `?` placeholders:

```php
$db->query("create table if not exists users (id integer primary key, name text)");
$db->query("delete from users where id = ?", 10);
```

A `?` is the placeholder whatever the driver is. phpscript rebinds a statement with arguments to the style the driver reads. The same PHP runs on sqlite, mysql and postgres, where postgres numbers placeholders `$1`, `$2`. A statement without arguments passes through unchanged. Rebinding scans, not parses. A `?` inside a string literal is not a placeholder.

**A single value that is an array binds by name, not by position.** phpscript takes a single argument that is an array or a map as a set of named parameters for a `:name` query. `insert()` and friends build on that. A statement that binds exactly one value, where that value is an array, becomes a named query and fails somewhere else:

```php
// Refused: one argument, and it is an array.
$db->get("select * from users where id = ?", $ids);

// Bind the scalar.
$db->get("select * from users where id = ?", $ids[0]);
```

Code that builds its argument list at runtime checks for it, as dbadmin's `driver_dao::bind()` does, ahead of the pdo client reporting it.

A value the driver binds unaided is exempt. A lone `Time` from a date column binds by position like any scalar. See [Dates and times](#dates-and-times).

`get` returns the first row as an associative array, or `false` when there are no rows. `get_all` returns every row as an array of associative arrays:

```php
$user = $db->get("select id, name from users where name = ?", "Ada");
$users = $db->get_all("select id, name from users order by id");
```

`Database` raises database errors as PHP exceptions.

## CRUD helpers

`insert`, `replace`, and `update` build statements from associative arrays:

```php
$db->insert("users", array("name" => "Ada"));
$id = $db->insert_id();

$db->replace("users", array("id" => $id, "name" => "Ada Lovelace"));
$db->update("users", array("id" => $id, "name" => "Ada"), "id");

$changed = $db->rows_affected();
```

The final arguments to `update` name the columns used in its `WHERE` clause. Those columns must also be present in the value array. The write helpers and `query` return `true` on success.

## Transactions and pinned connections

`begin` starts a transaction. Every operation on that client after `begin` uses the transaction until `commit` or `rollback`:

```php
$db->begin();
$db->insert("users", array("name" => "Grace"));
$db->commit();
```

Nested transactions are not supported. `commit` and `rollback` are safe to call when no transaction is active.

The constructor is enough for ordinary operations, which borrow connections from the shared pool. When two non-transactional operations must use the same physical connection, call the zero-argument `connect()` method to pin one. Call `close()` afterward to return it to the pool:

```php
$db = new Database("app");
$db->connect();
$db->query("create temporary table selected_users (id integer)");
$db->query("insert into selected_users values (?)", 10);
$db->close();
```

`connect()` does not select or configure a named database. The connection name is always passed to `new Database("name")`.

## Dates and times

A date column is the sole place a Go value crosses into PHP without a written conversion: the driver scans it into a `time.Time`. The [stdlib/ps/time](../design.md#dates-and-times) binding returns the same value to a script. One rule follows:

**store the offset.** Prefer the column type that stores one. Write RFC 3339 (`2026-08-26T14:48:00+02:00`) wherever the column is text. `json_encode` already emits RFC 3339 for a `Time`. One spelling covers database columns, JSON payloads and Go bindings. The layout has a name: `$t->format(TIME_RFC3339)`.

### Column types, per dialect

Measured against the drivers this runtime ships, writing `2026-08-26T14:48:00Z` and reading it back:

| Dialect  | Column type            | Stored as                       | Read back as                       |    |
|----------|------------------------|---------------------------------|------------------------------------|----|
| postgres | `TIMESTAMPTZ`          | `2026-08-26 14:48:00+00`        | `Time`, in the session zone        | ✅ |
| postgres | `TIMESTAMP`            | wall clock, no offset           | `Time`, labelled UTC               | ⚠️  |
| postgres | `DATE`                 | `2026-08-26`                    | `Time` at midnight UTC             |    |
| mysql    | `TIMESTAMP`            | `2026-08-26 16:48:00`, in `loc` | `Time`, in `loc` (the DSN's)       | ✅ |
| mysql    | `DATETIME`             | as above                        | as above                           | ⚠️  |
| sqlite   | `TIMESTAMP`/`DATETIME` | the stored text                 | `Time`, offset read from that text | ✅ |
| any      | `TEXT`/`VARCHAR`       | see below                       | a plain string                     | ❌ |

A `TIMESTAMPTZ` stores an instant. A mysql `TIMESTAMP` also stores an instant. The rows marked ⚠️ store a wall clock instead. They round trip correctly only while all writers and readers agree on one zone. The column type cannot name that zone. The rows marked ❌ are not date columns, which the driver has no reason to parse. A script gets the text back.

sqlite has no date type. The **declared** column type decides whether sqlite treats it as a date. The driver scans a `TIMESTAMP` or `DATETIME` column into a `Time`. A `TEXT` column stays a string. The text stored in the column decides the zone. `2026-08-26T14:48:00+02:00` keeps the offset. The driver reads `2026-08-26 14:48:00` as UTC whatever it meant.

### Write the layout, do not bind the value

Binding a `Time` straight into a statement works for dialects that have a date type. It produces something unusable for sqlite:

```php
// sqlite stores Go's own rendering, which its date functions cannot read:
//   2026-08-26 14:48:00 +0000 UTC
$db->query("insert into events (id, at) values (?, ?)", 1, $t);

// Portable, and readable by strftime() and datetime() alike:
//   2026-08-26T16:48:00+02:00
$db->query("insert into events (id, at) values (?, ?)", 1, $t->format(TIME_RFC3339));
```

The same call against mysql stores `2026-08-26 16:48:00`. Against postgres it stores `2026-08-26 14:48:00 +0000 UTC`. A bound `Time` in a text column has three formats, depending on the driver. Formatting first produces a single format.

Reading back takes no conversion. The column is a `Time`, which `$row["at"]->format(TIME_RFC3339)` renders. `$row["at"]->unix()` compares it.

### What the zone does to a printed time

**`echo` prints a wall clock, and the driver chooses the zone.** A value written as `14:48:00Z` echoes as `2026-08-26 16:48:00` when it comes back from mysql or from a `TIMESTAMPTZ`, both returning it in the local zone. Same instant, different zone. Compare with `->unix()` or `->equal()`, never by echoing two values and reading them. Format with `TIME_RFC3339` when the reading must be unambiguous.

A `Time` is exempt from the named-binding rule under [Execute queries](#execute-queries). A statement whose only bound value is a `Time` works as written:

```php
$db->query("insert into events (at) values (?)", $t);
```

That needs pdo v0.2.4 or later. An earlier pdo reads a lone `Time` as a bag of named parameters. The statement reaches the server with its `?` unbound.

## Tracing

Database operations automatically contribute database spans to the trace of the request that ran them. Each query span measures the call and records the SQL statement, the transaction depth and any error. A transaction is a single span from `begin()` to `commit()` or `rollback()`. See [Telemetry](../telemetry.md).

A span also records the statement kind. `/* userGet */ select * from user` records `query_type` as `select`. It records `query_comment` as `userGet`. The comment stays in the statement that reaches the server, where `SHOW PROCESSLIST` shows the same tag. The tracer records a statement a read-only client refused the same way, with the refusal as its error. The statement never reaches the query log.

## API summary

The binding registers these methods:

`Database`:

- `Database::register($name, $dsn)` and `Database::connections()`, the statics
- `$db->is_readonly`, a readable and writable property restricting the client to reads
- `insert($table, $values)`
- `replace($table, $values)`
- `update($table, $values, ...$key_columns)`
- `query($sql, ...$arguments)`
- `get($sql, ...$arguments)`
- `get_all($sql, ...$arguments)`
- `connect()`, `close()` for optional connection pinning
- `begin()`, `commit()`, `rollback()`
- `insert_id()` and `rows_affected()`

`Database\Migrate`:

- `load($pattern)`
- `run()`

## Complete example

```php
<?php

$db = new Database("app");

$db->query("create table if not exists users (id integer primary key, name text)");
$db->insert("users", array("name" => "Ada"));

$user = $db->get("select id, name from users where name = ?", "Ada");
$users = $db->get_all("select id, name from users order by id");

foreach ($users as $row) {
	echo $row["name"] . "\n";
}
```

## References

- [Go `stdlib/ps/database.Database`](https://pkg.go.dev/github.com/titpetric/phpscript@main/stdlib/ps/database#Database)
- [Go `stdlib/ps/database.DatabaseMigrate`](https://pkg.go.dev/github.com/titpetric/phpscript@main/stdlib/ps/database#DatabaseMigrate)
- [tests/fixtures/database_migrate.phpt](../../tests/fixtures/database_migrate.phpt)
- [tests/fixtures/database_readonly.phpt](../../tests/fixtures/database_readonly.phpt)
- [tests/fixtures/platform_database.phpt](../../tests/fixtures/platform_database.phpt)
- [tests/fixtures/test-database.php](../../tests/fixtures/test-database.php)
