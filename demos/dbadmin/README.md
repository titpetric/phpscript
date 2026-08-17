# SQLite Admin demo

A server-rendered, phpMyAdmin-style SQLite catalogue for phpscript. Controllers are annotated `.php` routes and all HTML views live in `templates/*.tpl`, loaded through the `Template` class. It provides a database overview, searchable and paginated table browsing, schema and index inspection, table creation, row insertion/editing/deletion, confirmed table drops, a direct SQL console, and CSV export.

The table editor creates up to four initial columns and includes common numeric, text, binary, boolean, and date/time types. Additional schema changes can be made through the SQL console. The demo connection and schema catalogue remain SQLite-backed; the PostgreSQL and MySQL selections validate definitions against those dialect presets.

From this directory, start it with:

```sh
PLATFORM_DB_DBADMIN=sqlite://dbadmin.sqlite go run ../.. server .
```

Then open the address printed by the server (normally `http://localhost:8080`). Static assets are served directly from `public/`; the PHP controllers remain outside the public web root and are registered through their `@route` annotations. To use an absolute database path, use a three-slash DSN such as `sqlite:///tmp/dbadmin.sqlite`.

## Schema

`schema/` holds one `*.up.sql` migration per table, containing its `CREATE TABLE` statement and the demo rows to seed it with. `migrate.php` is an `@startup` file, so the migrations run to completion before the server accepts requests:

```php
$migrate = new Database\Migrate("dbadmin");
$migrate->load("./schema/*.up.sql");
$migrate->run();
```

Each file is append only. Applied statements are recorded by index in a `migrations` table, so a later change is added as new statements at the end of the file; editing or removing statements that already ran leaves databases inconsistent with each other. Adding a table means adding `schema/<table>.up.sql`.

## Tests

`tests/venom.yml` is a [venom](https://github.com/ovh/venom) suite covering every route the demo registers. It runs against the compose service, which has no published port, so `tests/atkins.yml` resolves the container address for it:

```sh
cd tests && atkins test
```

The suite is idempotent: it removes its own leftovers before it starts and edits rows in a scratch table so the seeded rows keep the ids the migration gave them.

> **Local demo only:** this application has no authentication or authorization, and its SQL console intentionally executes arbitrary SQL. Do not expose it to an untrusted network or use it against a production database.
