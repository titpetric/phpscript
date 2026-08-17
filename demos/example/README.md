# Example application

A bookmark list: the application built step by step in
[Building an application](../../docs/use-cases/application.md). It is the
smallest complete phpscript application — a database with migrations, three
annotated endpoints, a compiled template and a static asset.

```sh
phpscript -f config.yml server .
```

Then open the address printed by the server (normally `http://localhost:8080`).
`config.yml` registers the SQLite connection as `bookmarks`, so the database is
created next to the sources on first start.

| Route                              | File                                          |
|------------------------------------|-----------------------------------------------|
| `GET /`                            | [index.php](./index.php)                      |
| `POST /bookmarks`                  | [bookmark-create.php](./bookmark-create.php)  |
| `POST /bookmarks/{id}/delete`      | [bookmark-delete.php](./bookmark-delete.php)  |
| `@startup`                         | [migrate.php](./migrate.php)                  |

`include/Compiler.php` and `include/Template.php` are
[minitpl](https://github.com/titpetric/minitpl), vendored from
[tests/fixtures/code](../../tests/fixtures/code) and formatted with
`phpscript fmt`. They compile `templates/*.tpl` into PHP under
`templates/cache/` on first use. The engine assigns inside `if` conditions, so
`phpscript lint` reports warnings on those two files.

## Tests

[tests/venom.yml](tests/venom.yml) covers every route. It runs against the
compose service, which has no published port, so
[tests/atkins.yml](tests/atkins.yml) resolves the container address for it:

```sh
cd tests && atkins test
```

The suite leaves the database as it found it: each case that adds a bookmark
extracts the id it was given and deletes it again, so it can be run repeatedly.
