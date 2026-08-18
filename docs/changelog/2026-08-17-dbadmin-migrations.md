# dbadmin: schema migrations, and the bugs a test suite found

**2026-08-17.** The dbadmin demo no longer creates its table from PHP on the
first request. Its schema lives in `demos/dbadmin/schema/catalogue.up.sql` and
is applied by `migrate.php`, an `@startup` file, before the server listens.
Writing a venom suite for the demo's routes then surfaced three defects, two of
them in the interpreter.

## The demo

One `*.up.sql` file per table, holding its `CREATE TABLE` and the rows to seed
it with, applied by:

```php
<?php

// @startup

$migrate = new Database\Migrate("dbadmin");
$migrate->load("./schema/*.up.sql");
$migrate->run();
```

Files are append only: progress is recorded per statement index in a
`migrations` table, so a schema change is new statements at the end of an
existing file. [Run migrations](../use-cases/database.md#run-migrations) is the
walkthrough; the demo is the worked example.

`bootstrap.php` lost the seeding block and `render()`; the eight view endpoints
call `$tpl->load()`, `assign()` and `render()` directly, and every `$_GET` and
`$_POST` read now happens in a file with a `@route` annotation.

## What the suite found

`demos/dbadmin/tests/venom.yml` covers all 17 routes, and runs from the root
pipeline as `test:demos:dbadmin` after the compose service is up. It fails
against the demo as it stood before this change:

- **`/sql` never executed anything.** `$db->query()` returns a bool, not a
  statement handle, so the console's `$statement->fetch()` loop was a 500 on
  POST and was never reached on GET. Rows now come from `$db->get_all()`, and
  both verbs share one helper, each reading its own superglobal.
- **`die("message")` printed nothing.** The argument was passed to `toInt` and
  read as an exit status, so every guard in the demo (`Table not found.`,
  `Invalid table name.`, `Confirmation did not match the table name.`) returned
  a blank page. `exit`/`die` now follow PHP: a string argument is written to the
  output and exits with status 0, an integer sets the status.
- **`sort()` did not exist.** Added `sort` and `rsort`, sharing `sortValues`
  with `usort`. They order numerically when both values are numbers and by
  string otherwise, discarding keys as PHP does.

`die("message")` is a behaviour change for existing scripts: a call that used to
produce no output now writes its message. Scripts passing an integer are
unaffected.

## Documentation

The `@startup` example in [usage.md](../usage.md) loaded `./schema/*.sql`, a
glob the runner never applies: only `*.up.sql` files run, so that example
migrated nothing. The demo README started the server with `DB_DSN_DBADMIN`,
which registers no connection; the prefix is `PLATFORM_DB_`.
