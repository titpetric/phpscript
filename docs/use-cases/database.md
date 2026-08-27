# Database bindings

The standard runtime provides the Go-backed `Database` class and imports these
drivers:

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

The part after `PLATFORM_DB_` is normalized to a lowercase connection name, so
PHP should request `PLATFORM_DB_APP` as `"app"`. The value uses
`<driver>://<dsn>` syntax. Process environment variables with the same prefix
are also registered when phpscript starts.

See [Configuration](../configuration.md#database-connections) for more details.

## Create a client

Pass a registered connection name to the constructor. Each `Database` instance
uses the shared pool for that connection:

```php
$db = new Database("app");
```

The constructor throws an exception if the connection name is not registered or
the pool cannot be opened.

## Connections an application owns

Connection names normally come from the environment the host started with,
which is right when the operator owns the list. An application that keeps its
connections in a table owns the list itself, and `putenv` will not do: it writes
to the script environment, and the provider is built once, before the first
request, from the host's.

`Database::register()` adds one:

```php
Database::register("reports", "postgres://user:pass@host/reports?sslmode=disable");

$db = new Database("reports");
```

The name is lowercased, as it is when it comes from `PLATFORM_DB_REPORTS`.
Registering a name that already means the same thing does nothing; registering
it with a different DSN closes the pool that was opened for the old one, so an
edited connection takes effect on the next request rather than the next
restart.

A virtual host registers into its own provider, so one site cannot reach
another's databases by naming them.

`Database::connections()` returns the names the script can open, sorted:

```php
foreach (Database::connections() as $name) {
	echo $name, "\n";
}
```

The [dbadmin demo](../../demos/dbadmin) is built on this: its `connection` table
holds a name and a DSN per row, and `connection_dao::open()` registers the DSN
before asking for a client.

## Read-only clients

`$db->is_readonly` restricts a client to statements that only read, so a page
that only displays rows cannot write them by accident:

```php
$db = new Database("app");
$db->is_readonly = true;

$users = $db->get_all("select id, name from users");   // runs
$db->query("delete from users");                       // throws
```

It is a property of the client, not of the connection: the pool is the same
pool, and what changes is what this client will send down it. The property is
request scope, like the client holding it. Set it where a request stops
writing, and read it back anywhere:

```php
if (!$db->is_readonly) {
	$db->insert("users", array("name" => "Ada"));
}
```

`insert`, `replace` and `update` are refused outright; they write by definition.
`query`, `get` and `get_all` are refused on the keyword the statement starts
with, past any comment in front of it. These statements run:

| Statement             | Why                                          |
|-----------------------|----------------------------------------------|
| `SELECT`              | the read                                     |
| `SHOW`                | schema and server introspection              |
| `DESCRIBE` and `DESC` | table introspection, where the engine has it |

Everything else is refused, including `EXPLAIN`: `EXPLAIN ANALYZE INSERT ...`
runs the statement it reports on in PostgreSQL, so an explain is not reliably a
read. `PRAGMA` is refused for the same reason: `PRAGMA journal_mode = WAL`
writes.

The refusal is thrown as an exception naming the statement, so a script can
catch it and say which call it lost:

```php
try {
	$db->query("drop table users");
} catch (Exception $e) {
	echo $e;   // database is read-only: drop is not allowed
}
```

`begin`, `commit`, `rollback`, `connect` and `close` stay available, since a
read-only transaction is a read.

Classification reads the start of the statement and nothing else. A statement
has to begin with its keyword, so `(SELECT 1) UNION (SELECT 2)` is refused, and
nothing stops a second statement smuggled in behind a semicolon on a driver that
allows more than one per call.

That is the shape of the whole feature: a boundary for the code holding the
client, not a sandbox around the script. The script that set the property can
clear it, the same way it set it. A connection that must not write belongs to a
database user without the grant to; this is what keeps an application's own code
on the right side of that grant, and what makes a page that only displays rows
say so.

## Run migrations

`Database\Migrate` applies `*.up.sql` migration files to a named connection.
This section sets up migrations for an application from scratch. The
[dbadmin demo](../../demos/dbadmin) is a working copy of the result.

### Lay out the schema

Keep one migration file per table, named after the table, in a `schema/`
directory next to the application:

```text
my-app/
├── migrate.php
├── public/
│   └── index.php
└── schema/
    ├── catalogue.up.sql
    └── users.up.sql
```

Only files ending in `.up.sql` are applied, in filename order. A file holds the
`CREATE TABLE` statement for its table, and any rows the application needs to
start with:

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

Statements are separated by a semicolon at the end of a line, and `--` starts a
comment. Because comments are stripped before the file is split, avoid `--`
inside string values.

### Run them at startup

Migrations belong in an [`@startup`](../usage.md) file, so they finish before
the server accepts requests. Create `migrate.php`:

```php
<?php

// @startup

$migrate = new Database\Migrate("app");
$migrate->load("./schema/*.up.sql");
$migrate->run();
```

The constructor takes the same connection name as `new Database("app")`, which
is `PLATFORM_DB_APP` in the environment. Omitting it selects the connection
named `default`. The name is also the project the run is recorded under, so two
applications sharing one database keep separate records of what they applied.

Before `app`, the constructor tries the connection `app:migrate`. A deployment
that registers one points migrations at a user allowed to alter tables while
the application queries through a user that is not, and the script names
neither. An environment variable name holds no colon, so `app:migrate` cannot
be set as `PLATFORM_DB_APP:MIGRATE`; it is registered with
`Database::register("app:migrate", $dsn)`. Whichever of the two answers, the
project stays `app`.

`load()` accepts a glob relative to the runtime working directory, selecting
the migration files in the application filesystem. `run()` applies them and
records what it applied in a `migrations` table, so later server starts skip
the statements that already ran.

Loading or applying a migration can raise a PHP exception. An uncaught
exception aborts startup, and the startup step and its error are recorded as a
background trace when telemetry is enabled.

### Change a schema later

Migration files are append only. Progress is recorded per statement index, so
adding statements at the end of an existing file is how a table is changed:

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

On the next start, only the `ALTER TABLE` runs. Editing or removing a statement
that already ran does not re-run it, and leaves databases that applied the old
version inconsistent with new ones. Add a table by adding a file.

## Execute queries

Use `query` for statements that do not return rows. Positional arguments are
bound to `?` placeholders:

```php
$db->query("create table if not exists users (id integer primary key, name text)");
$db->query("delete from users where id = ?", 10);
```

A `?` is the placeholder whatever the driver is. A statement that is given
arguments is rebound to the style the driver reads, so the same PHP runs on
sqlite, mysql and postgres, which numbers its placeholders `$1`, `$2`. A
statement with no arguments is passed through as it was written: rebinding
scans rather than parses, and a `?` inside a string literal is not a
placeholder.

**One value that is not a scalar is bound by name, not by position.** A single
argument that is an array, a map or an object is taken as a set of named
parameters for a `:name` query, which is what makes `insert()` and friends
work. A statement that binds exactly one value, and that value is one of those,
therefore becomes a named query and fails somewhere else. An array is the usual
way to trip over it; a `Time` from a date column is the other, see
[Dates and times](#dates-and-times):

```php
// Refused: one argument, and it is an array.
$db->get("select * from users where id = ?", $ids);

// Bind the scalar.
$db->get("select * from users where id = ?", $ids[0]);
```

Code that builds its argument list at runtime should check for it, as dbadmin's
`driver_dao::bind()` does, rather than let sqlx report it.

`get` returns the first row as an associative array, or `false` when there are
no rows. `get_all` returns every row as an array of associative arrays:

```php
$user = $db->get("select id, name from users where name = ?", "Ada");
$users = $db->get_all("select id, name from users order by id");
```

Database errors are raised as PHP exceptions.

## CRUD helpers

`insert`, `replace`, and `update` build statements from associative arrays:

```php
$db->insert("users", array("name" => "Ada"));
$id = $db->insert_id();

$db->replace("users", array("id" => $id, "name" => "Ada Lovelace"));
$db->update("users", array("id" => $id, "name" => "Ada"), "id");

$changed = $db->rows_affected();
```

The final arguments to `update` name the columns used in its `WHERE` clause.
Those columns must also be present in the value array. The write helpers and
`query` return `true` on success.

## Transactions and pinned connections

`begin` starts a transaction. All subsequent operations on that client use the
transaction until `commit` or `rollback`:

```php
$db->begin();
$db->insert("users", array("name" => "Grace"));
$db->commit();
```

Nested transactions are not supported. `commit` and `rollback` are safe to call
when no transaction is active.

The constructor is sufficient for ordinary operations, which borrow connections
from the shared pool. When several non-transactional operations must use the
same physical connection, call the zero-argument `connect()` method to pin one
and call `close()` afterward to return it to the pool:

```php
$db = new Database("app");
$db->connect();
$db->query("create temporary table selected_users (id integer)");
$db->query("insert into selected_users values (?)", 10);
$db->close();
```

`connect()` does not select or configure a named database. The connection name
is always passed to `new Database("name")`.

## Dates and times

A date column is the one place a Go value crosses into PHP without a conversion
being written for it: the driver scans it into a `time.Time`, and that is the
same value [stdlib/time](../design.md#dates-and-times) hands a script. That is
worth designing for rather than working around, and the design is one rule:

**store the offset.** Prefer the column type that carries one, and write RFC
3339 (`2026-08-26T14:48:00+02:00`) wherever the column is text. It is what
`json_encode` already emits for a `Time`, so one spelling covers a database
column, a JSON payload and a Go binding, and the layout has a name:
`$t->format(TIME_RFC3339)`.

### Column types, per dialect

Measured against the drivers this runtime ships, writing `2026-08-26T14:48:00Z`
and reading it back:

| Dialect  | Column type            | Stored as                       | Read back as                       |    |
|----------|------------------------|---------------------------------|------------------------------------|----|
| postgres | `TIMESTAMPTZ`          | `2026-08-26 14:48:00+00`        | `Time`, in the session zone        | ✅ |
| postgres | `TIMESTAMP`            | wall clock, no offset           | `Time`, labelled UTC               | ⚠️  |
| postgres | `DATE`                 | `2026-08-26`                    | `Time` at midnight UTC             |    |
| mysql    | `TIMESTAMP`            | `2026-08-26 16:48:00`, in `loc` | `Time`, in `loc` (the DSN's)       | ✅ |
| mysql    | `DATETIME`             | as above                        | as above                           | ⚠️  |
| sqlite   | `TIMESTAMP`/`DATETIME` | whatever text was written       | `Time`, offset read from that text | ✅ |
| any      | `TEXT`/`VARCHAR`       | see below                       | a plain string                     | ❌ |

A `TIMESTAMPTZ` and a mysql `TIMESTAMP` both store an instant and both round
trip one. The rows marked ⚠️ store a wall clock instead: they round trip
correctly only while every writer and reader agrees on one zone, and they have
no way to say which zone that was. The rows marked ❌ are not date columns at
all; the driver has no reason to parse them, so a script gets the text back.

sqlite has no date type. What makes a column a date there is its **declared**
type: a `TIMESTAMP` or `DATETIME` column is scanned into a `Time`, a `TEXT`
column is not, and the text stored in it is what decides the zone —
`2026-08-26T14:48:00+02:00` keeps the offset, while `2026-08-26 14:48:00` is
read as UTC whatever it meant. That is the whole argument for RFC 3339 in one
line.

### Write the layout, do not bind the value

Binding a `Time` straight into a statement works for the dialects that have a
date type, and produces something unusable for the one that does not:

```php
// sqlite stores Go's own rendering, which its date functions cannot read:
//   2026-08-26 14:48:00 +0000 UTC
$db->query("insert into events (id, at) values (?, ?)", 1, $t);

// Portable, and readable by strftime() and datetime() alike:
//   2026-08-26T16:48:00+02:00
$db->query("insert into events (id, at) values (?, ?)", 1, $t->format(TIME_RFC3339));
```

The same call against mysql stores `2026-08-26 16:48:00` and against postgres
`2026-08-26 14:48:00 +0000 UTC`, so a bound `Time` in a text column has three
formats depending on which driver is under it. Formatting first has one.

Reading back needs no ceremony: the column is a `Time` and its methods are
there, so `$row["at"]->format(TIME_RFC3339)` renders it and
`$row["at"]->unix()` compares it.

### Two things that will surprise you

**`echo` prints a wall clock, and the driver chooses the zone.** A value
written as `14:48:00Z` echoes as `2026-08-26 16:48:00` when it comes back from
mysql or from a `TIMESTAMPTZ`, because both return it in the local zone. It is
the same instant, printed in a different place. Compare with `->unix()` or
`->equal()`, never by echoing two values and reading them; format with
`TIME_RFC3339` when the reading has to be unambiguous.

**A lone `Time` argument is bound by name, not by position.** The rule under
[Execute queries](#execute-queries) — one argument that is an array becomes a
named query — is really *one argument that is not a scalar*, and a `Time` is
not a scalar. `$db->query("insert into events (at) values (?)", $t)` reaches
the server with its `?` unbound and fails as a syntax error. Bind another
column alongside it, or format the value into a string.

## Tracing

Database operations automatically contribute database spans to the trace of the
request that ran them. A query span measures the call and carries the SQL
statement, the transaction depth and any error; a transaction is one span from
`begin()` to `commit()` or `rollback()`. See [Telemetry](../telemetry.md).

A span also carries what the statement is. `/* userGet */ select * from user`
records `query_type` as `select` and `query_comment` as `userGet`, and the
comment stays in the statement that reaches the server, so `SHOW PROCESSLIST`
shows the same tag. A statement a read-only client refused is recorded the same
way, with the refusal as the span's error; it never reaches the query log.

## API summary

The binding provides these methods:

`Database`:

- `Database::register($name, $dsn)` and `Database::connections()`, the statics
- `$db->is_readonly`, a readable and writable property restricting the client to reads
- `insert($table, $values)`
- `replace($table, $values)`
- `update($table, $values, ...$key_columns)`
- `query($sql, ...$arguments)`
- `get($sql, ...$arguments)`
- `get_all($sql, ...$arguments)`
- `connect()` and `close()` for optional connection pinning
- `begin()`, `commit()`, and `rollback()`
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

- [Go `stdlib/database.Database`](https://pkg.go.dev/github.com/titpetric/phpscript@main/stdlib/database#Database)
- [Go `stdlib/database.DatabaseMigrate`](https://pkg.go.dev/github.com/titpetric/phpscript@main/stdlib/database#DatabaseMigrate)
- [tests/fixtures/database_migrate.phpt](../../tests/fixtures/database_migrate.phpt)
- [tests/fixtures/database_readonly.phpt](../../tests/fixtures/database_readonly.phpt)
- [tests/fixtures/platform_database.phpt](../../tests/fixtures/platform_database.phpt)
- [tests/fixtures/test-database.php](../../tests/fixtures/test-database.php)
