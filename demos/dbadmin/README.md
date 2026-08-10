# SQLite Admin demo

A server-rendered, phpMyAdmin-style SQLite catalogue for phpscript. Controllers are annotated `.php` routes and all HTML views live in `templates/*.tpl`, loaded through the `Template` class. It provides a database overview, searchable and paginated table browsing, schema and index inspection, table creation, row insertion/editing/deletion, confirmed table drops, a direct SQL console, and CSV export. On first request it creates and seeds a small `catalogue` table if that table is absent.

The table editor creates up to four initial columns and includes common numeric, text, binary, boolean, and date/time types. Additional schema changes can be made through the SQL console. The demo connection and schema catalogue remain SQLite-backed; the PostgreSQL and MySQL selections validate definitions against those dialect presets.

From this directory, start it with:

```sh
DB_DSN_DBADMIN=sqlite://dbadmin.sqlite go run ../.. server .
```

Then open the address printed by the server (normally `http://localhost:8080`). Static assets are served directly from `public/`; the PHP controllers remain outside the public web root and are registered through their `@route` annotations. To use an absolute database path, use a three-slash DSN such as `sqlite:///tmp/dbadmin.sqlite`.

> **Local demo only:** this application has no authentication or authorization, and its SQL console intentionally executes arbitrary SQL. Do not expose it to an untrusted network or use it against a production database.
