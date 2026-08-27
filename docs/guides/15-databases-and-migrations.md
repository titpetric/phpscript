# Databases and migrations

This chapter connects the application to a database, gives it a schema applied before the
first request, and shows how a query is written so that it runs unchanged on sqlite, mysql
and postgres. At the end of it you have a `schema/` directory, a `migrate.php` that runs at
startup, and the rules every store in
[Structuring an application](25-structuring-an-application.md) follows.

## Register a connection

A connection is named in `config.yml`, in the `env` list, as
`PLATFORM_DB_<NAME>=<driver>://<dsn>`:

```yaml
env:
  - "PLATFORM_DB_COMMON=sqlite://common.db"
```

A sqlite file is written by the application, so `common.db` also belongs in
`runner.writable_paths`. The part after `PLATFORM_DB_` is lowercased into a connection name,
so PHP asks for it as `"common"`:

```php
$db = new Database("common");
```

The constructor throws when the name is not registered or the pool will not open. The three
drivers the standard runtime imports are `sqlite`, `mysql` and `postgres`.

The `env` list is not the process environment. It is handed to the connection registry and
nothing else, so `getenv("PLATFORM_DB_COMMON")` is `false` in a script and a credential
never reaches one.

An application that keeps its own connections in a table registers them itself. `putenv`
will not do, because the provider is built once before the first request, out of the host's
environment and not the script's:

```php
Database::register("reports", "sqlite:///tmp/reports.db");

$reports = new Database("reports");
```

The name is lowercased, as it is when it arrives from `PLATFORM_DB_REPORTS`. Registering the
same DSN again does nothing; a different one closes the pool opened for the old DSN, so an
edited connection takes effect on the next request. `Database::connections()` returns the
names this script can open, sorted, and a virtual host registers into its own provider, so
one site cannot reach another's databases by naming them.

## Write a migration

Migrations live in `schema/`, next to `migrate.php` and `config.yml`. There is one file per
table, named after the table, and only `*.up.sql` is applied, in filename order. The shipped
application has six: `module`, `rule`, `user`, `user_group`, `user_group_member` and
`user_session`.

A file is one table, its indexes, and a comment saying what the table is for:

```sql
-- user: one account.
--
-- id is a ULID minted by the application. There are no foreign keys anywhere in
-- this schema, so deleting a user deletes its user_group_member and
-- user_session rows as an application step.

CREATE TABLE IF NOT EXISTS user (
	id CHAR(26) PRIMARY KEY NOT NULL,
	username VARCHAR(64) NOT NULL,
	email VARCHAR(255) NOT NULL DEFAULT '',
	password VARCHAR(255) NOT NULL DEFAULT '',
	is_admin TINYINT NOT NULL DEFAULT 0,
	is_active TINYINT NOT NULL DEFAULT 1,
	properties TEXT,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME
);

CREATE UNIQUE INDEX IF NOT EXISTS uidx_user_username ON user(username);
CREATE INDEX IF NOT EXISTS idx_user_email ON user(email);
```

Statements are separated by a semicolon at the end of a line and `--` starts a comment.
Comments are stripped before the file is split, so avoid `--` inside a string value.

Migration files are append only. Progress is recorded per statement index, so a later change
to `user` is a new statement at the end of the file and never an edit to one that already
ran. Editing one does not re-run it; it only makes fresh databases disagree with old ones.
Add a table by adding a file.

The file that runs them carries `// @startup`, so the server finishes it before it listens
and no request meets a half-applied schema. This is `migrate.php`, down to the seed:

```php
<?php

// @startup

$migrate = new Database\Migrate("common");
$migrate->load("./schema/*.up.sql");
$migrate->run();

echo "migrate: schema applied\n";

include "bootstrap.php";
```

The migration runs before `bootstrap.php` is included, not after. The composition root
queries the `rule` and `module` tables while it builds the navigation, and on a fresh
database those tables do not exist yet.

`load()` takes a glob relative to the runtime working directory, and a glob matching nothing
applies nothing and does not fail. `run()` applies what it matched and records it in a
`migrations` table, so the next start skips it.

The connection name is also the project the run is recorded under, so two schemas sharing
one database keep separate records. Before `common`, the constructor tries the connection
`common:migrate`. A deployment that registers one with
`Database::register("common:migrate", $dsn)` points migrations at a user allowed to alter
tables while the application queries as a user that is not, and the script names neither. An
environment variable name holds no colon, so that credential cannot come from a
`PLATFORM_DB_` entry. Whichever of the two answers, the project stays `common` and the
`migrations` rows read `common schema/user.up.sql`.

## Follow the schema conventions

The rules the shipped schema holds to, in every file:

- **Singular table names.** `user`, `module`, `rule`. A child table is `<parent>_<thing>`:
  `user_session`, `user_group_member`.
- **One `id` column, `CHAR(26) PRIMARY KEY NOT NULL`, listed first.** A join table gets a
  composite natural key and no surrogate `id`, as `user_group_member` does with
  `PRIMARY KEY (user_id, user_group_id)`.
- **No foreign keys anywhere.** No `FOREIGN KEY`, no `REFERENCES`.
- **Index names are `idx_<table>_<columns>`, or `uidx_` when unique**, as in
  `uidx_user_session_token` and `idx_user_group_member_user_group_id`. Every `<table>_id`
  column leads some index, which is all that stands in for the relation.
- **A point in time is a native column type**, `DATETIME` on sqlite and `TIMESTAMP` on the
  other two, never `TEXT`. `created_at` is `NOT NULL DEFAULT CURRENT_TIMESTAMP`, and nullable
  is reserved for "has not happened yet", which is what `revoked_at` means.

No foreign keys costs you the cascade. Deleting a user has to clear, by hand and in this
order, the `user_group_member` rows for that user, then its `user_session` rows, which
`revoke_all()` marks and `prune()` deletes later, and only then the `user` row. Memberships
go first, because a membership row that outlives its user would be inherited by a later user
reusing the id. `rule` rows are not among them: a grant attaches to a group and never to a
user, so a deleted user's grants stop applying when the membership row goes.

## Query

The full method list. There is no more of it:

| Call                                                  | Returns                                                      |
|-------------------------------------------------------|--------------------------------------------------------------|
| `query($sql, ...$args)`                               | `true`. For statements returning no rows.                    |
| `get($sql, ...$args)`                                 | one associative row, or `false` when there are none          |
| `get_all($sql, ...$args)`                             | an array of associative rows, empty when there are none      |
| `insert($table, $values)`, `replace($table, $values)` | `true`                                                       |
| `update($table, $values, ...$key_columns)`            | `true`. The key columns must be in `$values`.                |
| `insert_id()`, `rows_affected()`                      | the generated auto increment id, rows the last write changed |
| `begin()`, `commit()`, `rollback()`                   | transaction control                                          |
| `connect()`, `close()`                                | pin one physical connection and return it                    |

`get` and `get_all` differ only in how many rows come back:

```php
$user = $db->get("SELECT id, username, is_admin FROM user WHERE username = ?", "admin");
echo $user["username"], " ", $user["is_admin"], "\n";      // admin 1

$missing = $db->get("SELECT id FROM user WHERE username = ?", "nobody");
var_dump($missing);                                        // bool(false)

$rows = $db->get_all("SELECT name, title FROM module ORDER BY name");
echo count($rows), "\n";                                   // 3
```

Check `get` against `false` with `===`. `$row["missing"]` is `null` with no notice, so
`isset()` first wherever absence and zero differ. The write helpers build the statement from
an associative array:

```php
$db->insert("user_group", array("id" => $id, "name" => "editors", "description" => "Can edit"));
$db->update("user_group", array("id" => $id, "description" => "Edits pages"), "id");
echo $db->rows_affected(), "\n";                           // 1
```

Database errors arrive as PHP exceptions. `insert_id()` is for a schema using an auto
increment column; this one mints its ids in SQL and never reads it.

## Bind every value

A `?` is the placeholder on all three drivers. A statement carrying arguments is rebound to
whatever the driver reads, including postgres and its `$1`, `$2`. Nothing is ever
interpolated into the SQL text, which leaves the `IN (...)` list to build:

```php
public function placeholders($count)
{
	$count = (int)$count;
	if ($count < 1) {
		return "NULL";
	}

	return substr(str_repeat("?, ", $count), 0, -2);
}
```

`placeholders(3)` is `?, ?, ?` and `placeholders(1)` is `?`. `placeholders(0)` is `NULL`,
because `IN ()` is a syntax error and `IN (NULL)` is a legal statement that matches nothing.
It is `str_repeat` because `array_fill` is not bound in this runtime. The caller passes the
values positionally:

```php
$sql = "SELECT * FROM user WHERE id IN (" . $this->conn->placeholders(count($ids)) . ")";
$args = array_merge(array($sql), array_values($ids));

// call_user_func_array, because argument unpacking at a call site does not parse
// in this runtime.
return call_user_func_array(array($this->conn->db(), "get_all"), $args);
```

The legacy API this replaced built an `IN` list by pasting quoted values into the statement,
which is where the injection lived. Generated placeholders keep the values bound, and the
only text a caller contributes is a column name taken from a fixed list the store declares.

One trap: a single argument that is an array is taken as a set of named parameters for a
`:name` query, which is what makes `insert()` work. `$db->get("... WHERE id = ?", $ids)` is
refused for that reason. Bind the scalar.

## Transactions

`begin()` opens one, and every call on that client runs inside it until `commit()` or
`rollback()`. Nothing rolls a transaction back on its own, and a connection left inside one
stays held until the request's `Database` object is collected. So a transaction is written
with the catch that releases it, the way `routes/admin-users-delete.php` does:

```php
$db->begin();
try {
	$groups->remove_user($user["id"]);
	$session->revoke_all($user["id"]);
	$users->remove($user["id"]);
	$db->commit();
} catch (Exception $failed) {
	$db->rollback();

	throw $failed;
}
```

The inner `catch` rolls back and rethrows. It does not handle the error: the route's outer
`catch` still turns it into a response. Nested transactions are not supported, and `commit`
and `rollback` are safe to call when none is active.

## Keep it portable

The package targets sqlite, mysql and postgres, and two expressions have no portable
spelling. `Common\Store\Connection` hides both, which is why a store takes a `Connection` and
not a `Database`: the driver name travels with the handle, and `Database` has no driver
property.

Minting an identifier or a token needs the server's random source, because this runtime has
no CSPRNG. `random_expr($length)` returns the expression for that many lowercase hex
characters, and `mint()` and `token()` select it. The sqlite form is
`lower(substr(hex(randomblob(26)), 1, 26))`, the mysql form is
`lower(substr(replace(uuid(), '-', ''), 1, 26))` and the postgres form is
`substr(md5(random()::text || clock_timestamp()::text), 1, 26)`. It costs one round trip,
paid on an insert and never on a read.

The current time is `now_expr($offset_seconds)`: `datetime('now', '0 seconds')` on sqlite,
`DATE_ADD(UTC_TIMESTAMP(), INTERVAL 0 SECOND)` on mysql,
`(NOW() AT TIME ZONE 'utc' + INTERVAL '0 seconds')` on postgres. The offset is what an
expiry is written with, and zero is what it is compared against:

```php
$sql = "SELECT id, user_id, csrf_token FROM user_session"
	. " WHERE token = ? AND revoked_at IS NULL AND expires_at > " . $this->conn->now_expr(0);
```

**Nothing in this book formats or parses a timestamp in PHP.** There is no `date()` and no
`gmdate()` in this runtime, so a stored time is written by the server and compared by the
server. A `DATETIME` column read back into PHP arrives as a Go time object that echoes as
the empty string, so do not select one into a template or a context array.

One smaller rule: write `LIMIT 1`. `LIMIT 0,1` is accepted by mysql and by neither of the
other two.

## Turn a client read-only

`is_readonly` restricts one client to statements that only read, so a page that displays
rows cannot write them by accident:

```php
$db->is_readonly = true;
echo count($db->get_all("SELECT id FROM user_group")), "\n";

try {
	$db->query("DELETE FROM user_group");
} catch (Exception $e) {
	echo $e, "\n";      // database is read-only: delete is not allowed
}
```

`insert`, `replace` and `update` are refused outright. `query`, `get` and `get_all` are
refused on the keyword the statement starts with, past any leading comment: `SELECT`, `SHOW`,
`DESCRIBE` and `DESC` run, everything else is refused. `EXPLAIN` is refused because
`EXPLAIN ANALYZE INSERT` runs the statement it reports on in postgres, and `PRAGMA` because
`PRAGMA journal_mode = WAL` writes. `begin`, `commit`, `rollback`, `connect` and `close`
stay available, since a read-only transaction is a read.

It is a property of the client and not of the connection: the pool is the same pool, and the
script that set the property can clear it. A connection that must not write belongs to a
database user without the grant.

Next: [Templates and rendering](20-templates-and-rendering.md).
