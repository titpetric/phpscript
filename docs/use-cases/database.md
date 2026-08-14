# Database bindings

The phpscript CLI imports the following drivers:

- sqlite - `modernc.org/sqlite`
- mysql - `github.com/go-sql-driver/mysql`
- postgres - `github.com/jackc/pgx/v5/stdlib`

The standard runtime provides two Go-backed PHP APIs:

- `PS\DatabaseClient` is the preferred CRUD and transaction client. It uses
  named `PLATFORM_DB_*` connections and automatically contributes database spans
  to server-status traces.
- `PS\Database` is the lower-level, older prepared-statement API. It reads a
  named `DB_DSN_*` environment variable.

## Database client

Configure a connection in `config.yml` and pass the file to phpscript:

```yaml
env:
  - "PLATFORM_DB_APP=sqlite://app.db"
```

```bash
phpscript -f config.yml script.php
```

Constructing the client opens the named pooled connection:

```php
$db = new PS\DatabaseClient("app");

$db->query("create table if not exists users (id integer primary key, name text)");
$db->insert("users", array("name" => "Ada"));

$user = $db->get("select id, name from users where name = ?", "Ada");
$users = $db->get_all("select id, name from users order by id");
```

The client supports `insert`, `replace`, `update`, `query`, `get`, `get_all`,
`connect`, `close`, `begin`, `rollback`, `commit`, `insert_id`, and
`rows_affected`. A transaction keeps its exclusive connection until commit or
rollback:

```php
$db->begin();
$db->insert("users", array("name" => "Grace"));
$db->commit();
```

See [Configuration](../configuration.md#database-connections) for connection
registration details.

## Legacy `Database.php` helper

Lower-level bindings and the fixture helper are also available:

- [go stdlib/ps.Database](https://pkg.go.dev/github.com/titpetric/phpscript@main/stdlib/ps#Database)
- [go stdlib.DatabaseStatement](https://pkg.go.dev/github.com/titpetric/phpscript@main/stdlib#DatabaseStatement)
- [php Database.php](../../tests/fixtures/code/Database.php)

The PHP Database class combines these Go bindings into a functional
client. The following functions are provided for common database
interactions:

```php
class Database {
	public function connect($connection_name)
	public function close() {
	public function insert($table, $values)
	public function replace($table, $values, $extra_sql = '')
	public function update($table, $values, $keys)
	public function query($query, $values = false)
	public function get()
	public function get_all()
	public function fetch($stmt)
	public function insert_id()
	public function begin()
	public function start()
	public function rollback()
	public function commit()
}
```

To use, copy and include `Database.php` in your project.

## Query examples

The database interactions are quite simple:

```php
<?php

include("code/Database.php");

$db = new Database;
$db->connect();

// queries

$db->close();
```

The API is optimized to retrieve one or multiple rows:

```
$rows = $db->get_all("select * from users");
$row = $db->get("select * from users limit 1");
```

The API allows simple crud operations:

```php
$db->insert("table", $user);
$db->replace("table", $user, "ON DUPLICATE KEY ...");
$db->update("table", $user, "id");
```

There is no deletion implemented, favoring soft-deletes. This is an
arbitrary design choice that routes delete statements into `query`.

```php
$db->query("delete from users where id=?", id);
$db->query("update users set is_deleted=1 where id=?", id);
```

The latter form is usually preferred where data needs to be kept for
various reasons. The queries should be wrapped in a transaction if they
modify data in multiple tables in sequence.

```php
$db->begin();
$db->insert("users", $user1);
$db->insert("users", $user2);
// ...
if !$db->commit() {
	$db->rollback();
}
```

## Legacy connection names / DSN

The older `PS\Database` binding and `Database.php` helper take a
`$connection_name` and resolve it from the environment. For a connection named
"user", `DB_DSN_USER` supplies the literal connection string. These variables
are separate from the `PLATFORM_DB_*` registry used by `PS\DatabaseClient`.

```php
$db->connect("maillist") // connects to DB_DSN_MAILLIST connection.
```

Alternatively, if the value contains a `://`, the value is parsed into a
database driver name (`<driver>://`) and a DSN for that database. With
postgres, the full DSN is passed into the `pgx` driver, while other
database drivers just use the remainder as the connection string.

```bash
DB_DSN_SQLITE_TEST="sqlite://file:phpscript-test?mode=memory&cache=shared"
DB_DSN_POSTGRES_TEST="postgres://postgres:test@localhost:15432/postgres?sslmode=disable"
DB_DSN_MYSQL_TEST="mysql://root:test@tcp(localhost:13306)/mysql"
```

The above environment enables:

```php
$db->connect("sqlite_test");
$db->connect("postgres_test");
$db->connect("mysql_test");
```

This enables easier refactoring. For example to implement least
privilege changes in an additive way, you would:

- take your existing connection and make N env keys for each connection (N copies)
- each env value is set to the currently used connection (no risk)
- code is upcycled to use db->connect("users") and other keys (no risk)
- development is tested with `DB_DSN_USERS` with least privilege (testing)
- production is switched to least privilege by updating `DB_DSN_USERS`

Sharded connections and least privilege usually exclude usage of JOIN in
sql statements. This turns out to be a good thing for performance,
because if you join every SQL query you have against an user table,
writes to that table usually negatively impact performance under load,
blocking read operations for index updates on write.

The client holds transaction state. This in Go requires using smarter
clients, that are similarly constructed for the lifetime of the request.
The client could use a pooled connection handle until a transaction is
started. The writer connection itself could be separate to the reader.

Most small programs rely on a single database connection. While it's not
ideal practice, falling back to a single connection string is a safe
change you can make to grow into this principle of least privilege.

## References

- [tests/fixtures/database_test.phpt](../../tests/fixtures/database_test.phpt)
- [tests/fixtures/platform_database.phpt](../../tests/fixtures/platform_database.phpt)
- [tests/fixtures/test-database.php](../../tests/fixtures/test-database.php)
