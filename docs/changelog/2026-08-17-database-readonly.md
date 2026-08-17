# A read-only Database client, and what a query says about itself

**2026-08-17.** `$db->is_readonly = true` turns a client into one that runs reads
and refuses everything else. The connection is the same pooled connection; what
changes is what this client will send down it.

```php
$db = new Database("app");
$db->is_readonly = true;

$users = $db->get_all("select id, name from users");   // runs
$db->query("delete from users");                       // database is read-only: delete is not allowed
$db->insert("users", array("name" => "Ada"));          // database is read-only: insert is not allowed
```

It is a property of the client, not configuration of the connection, and not an
argument to the constructor. A client is request scope, so the restriction is
too: the request sets it where it stops writing, and reads it back anywhere —
`if (!$db->is_readonly)`.

`insert()`, `replace()` and `update()` are refused before a statement is built
for them: they write by definition, so there is nothing to classify. `query()`,
`get()` and `get_all()` are classified on the keyword the statement starts with.

## The allowlist

`SELECT`, `SHOW`, `DESCRIBE` and `DESC` run. Everything else is refused, because
a keyword nobody thought about is far more likely to be a write than a read. Two
exclusions are worth naming:

- `EXPLAIN`, because `EXPLAIN ANALYZE INSERT ...` runs the statement it reports on in PostgreSQL. An explain is not reliably a read.
- `PRAGMA`, because `PRAGMA journal_mode = WAL` writes.

Classification skips comments in front of the statement rather than reading them
as the statement, so a query tagged for `SHOW PROCESSLIST` still reads as what it
is:

```php
$db->get("/* userGet */ select * from user where id = ?", $id);
```

It reads the start of the statement and nothing else. A statement has to begin
with its keyword — `(SELECT 1) UNION (SELECT 2)` is refused rather than guessed
at — and a second statement smuggled in behind a semicolon is the driver's
business, not the classifier's.

## What it is, and what it is not

This is a boundary for the code holding the client, not a sandbox around the
script: the script that set the property can clear it. A connection that must
not write belongs to a database user without the grant to; this is what keeps an
application's own code on the right side of that grant, and what makes a page
that only displays rows say so.

`begin()`, `commit()`, `rollback()`, `connect()` and `close()` stay available. A
read-only transaction is a read.

## Every query now says what it is

The classification is useful beyond the property, so it runs for every client. A
database span carries two attributes beside the statement:

| attribute       | from `/* userGet */ SELECT * FROM user` |
|-----------------|-----------------------------------------|
| `query_type`    | `select`                                |
| `query_comment` | `userGet`                               |

The statement text alone does not group a trace: two calls of the same query
differ by their bound values and by nothing else worth grouping on. The comment
is left in the statement that reaches the server, so the tag in the trace is the
tag in `SHOW PROCESSLIST` and in the slow query log — the reason to write one. A
refused statement never reaches the query log, so its span carries the same
attributes plus the refusal as its error.

## Snake case reaches struct fields

`$db->is_readonly` resolves the Go field `IsReadonly`. Property lookup folded
case but not underscores, so it found `Value` for `$rec->value` and nothing for a
two-word name; it now folds both, the way method lookup has always resolved
`get_all()` to `GetAll`. Reads and writes share one resolver, and an unexported
field is no longer a property — reading one used to panic through reflect.

Assigning a property is also compiled by the flat bytecode backend now. It had no
opcode for it, so every script writing one — every script that sets
`is_readonly` — fell back to the interpreter for the whole program.

## Verification

[`database_readonly.phpt`](../../tests/fixtures/database_readonly.phpt) covers
the property end to end: reads run, each refusal is caught and printed, the table
is unchanged afterwards, and clearing the property lets the same client write
again. [`database_property_access.phpt`](../../tests/fixtures/database_property_access.phpt)
and [`database_property_unwritable.phpt`](../../tests/fixtures/database_property_unwritable.phpt)
pin what a property is on a Go binding. All three run on both backends.
`TestParseQuery` pins the classification over 18 statements, and
`TestDatabaseReadonlyRefusesWritingStatements` checks each one against a
restricted and an unrestricted client, so the property is the only thing
deciding.
