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

## Execute queries

Use `query` for statements that do not return rows. Positional arguments are
bound to `?` placeholders:

```php
$db->query("create table if not exists users (id integer primary key, name text)");
$db->query("delete from users where id = ?", 10);
```

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

## Tracing

Database operations automatically contribute database spans to server-status
traces. Query spans include the SQL statement, execution time, errors, and
transaction depth.

## API summary

The binding provides these methods:

- `insert($table, $values)`
- `replace($table, $values)`
- `update($table, $values, ...$key_columns)`
- `query($sql, ...$arguments)`
- `get($sql, ...$arguments)`
- `get_all($sql, ...$arguments)`
- `connect()` and `close()` for optional connection pinning
- `begin()`, `commit()`, and `rollback()`
- `insert_id()` and `rows_affected()`

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

- [Go `stdlib/ps.Database`](https://pkg.go.dev/github.com/titpetric/phpscript@main/stdlib/ps#Database)
- [tests/fixtures/platform_database.phpt](../../tests/fixtures/platform_database.phpt)
- [tests/fixtures/test-database.php](../../tests/fixtures/test-database.php)
