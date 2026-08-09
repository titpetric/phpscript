# SQLite Admin demo

A server-rendered, phpMyAdmin-style SQLite catalogue for phpscript. It provides a database overview, searchable and paginated table browsing, schema and index inspection, table creation, row insertion/editing/deletion, confirmed table drops, a direct SQL console, and CSV export. On first request it creates and seeds a small `catalogue` table if that table is absent.

The table editor supports up to 100 dynamically added columns. Its type selector includes SQLite, PostgreSQL, and MySQL presets, including date, time, datetime, and timestamp types. The demo connection and schema catalogue remain SQLite-backed; the PostgreSQL and MySQL selections are dialect presets for designing compatible definitions.

From this directory, start it with:

```sh
DB_DSN_DBADMIN=sqlite://dbadmin.sqlite go run ../.. route .
```

Then open the address printed by the route server (normally `http://localhost:8080`). To use an absolute database path, use a three-slash DSN such as `sqlite:///tmp/dbadmin.sqlite`.

> **Local demo only:** this application has no authentication or authorization, and its SQL console intentionally executes arbitrary SQL. Do not expose it to an untrusted network or use it against a production database.
