# SQLite Admin demo

A server-rendered, phpMyAdmin-style SQLite catalogue for phpscript. Each controller is a `.php` file carrying a `@route` annotation, and every HTML view lives in `templates/*.tpl`, which [titpetric/minitpl](https://github.com/titpetric/minitpl) renders. It carries a database overview, searchable and paginated table browsing, schema and index inspection, table creation, row insertion/editing/deletion, confirmed table drops, a direct SQL console, and CSV export.

The table editor creates up to four initial columns and includes common numeric, text, binary, boolean, and date/time types. The SQL console takes any schema change beyond that. The demo connection and schema catalogue remain SQLite-backed; the PostgreSQL and MySQL selections check definitions against those dialect presets.

## Dependencies

The template engine comes from composer, like in any other PHP project, and
this repository vendors none of it. Install it before the first run:

```sh
composer install
```

That writes `vendor/`, which `setup.php` pulls in with
`require "vendor/autoload.php"`. phpscript interprets composer's generated
autoloader as ordinary PHP, and the runtime special-cases nothing about it.

minitpl compiles each template to PHP on first use and caches the result in
`templates/cache/`, which has to be writable. `.gitignore` holds both directories.

## Running

From this directory, start it with:

```sh
PLATFORM_DB_DBADMIN=sqlite://dbadmin.sqlite go run ../.. server .
```

Then open the address printed by the server (normally `http://localhost:8080`). The server answers a static asset straight out of `public/`; the PHP controllers sit outside the public web root and reach the router through their `@route` annotations. To use an absolute database path, use a three-slash DSN such as `sqlite:///tmp/dbadmin.sqlite`.

Every page is behind a login. The demo ships with two accounts:

| Username | Password         |
|----------|------------------|
| `admin`  | `hunter2hunter2` |
| `reader` | `readerreader`   |

## Accounts

`.htpasswd` in this directory is the user database, in apache's format: one
`username:hash` per line, with lowercase names. It sits in the application root
and not under `public/`, so no request can reach it, and the application never
opens it either - `new Auth\Storage\PasswordFile` is what reads it, and a
script gets a verdict, a username and an id. No password or hash appears in any
PHP here, and the schema has no password column.

Adding an account is a line in the file and a reload:

```sh
htpasswd -B -C 10 .htpasswd alice
phpscript -s reload       # or: kill -HUP $(pidof phpscript)
```

bcrypt (`-B`) is what to write. The runtime also reads apache's default
`$apr1$` md5 and its `{SHA}`, because files in the wild carry them, and neither
has a cost parameter worth anything; see [security.md](../../docs/security.md).

A reload re-runs the `@startup` jobs, and `users.php` is one: it rebuilds the
`user` table from the file, so the accounts browse like any other table. That
table is a view of the file and no store of its own - nothing writes to it, and
`schema/` leaves it out. Its primary key is a ULID the username determines, so
removing an account takes its row and nothing else, and adding the same name
back restores the same key. A reload signs nobody out: sessions are files,
keyed by the application root.

The sign-in route holds each address, and each account name, to five refusals
in ten seconds, and answers further posts with 429 until the window passes: a
password check pays a bcrypt derivation, and without the ceiling an
unauthenticated caller decides how many of those the server runs. The sign-in
and sign-out forms carry a double-submit `csrf` token, and a post that does
not return the cookie's value gets 403; [security.md](../../docs/security.md)
states what that adds over the session cookie's `SameSite=Lax`.

## Schema

`schema/` holds one `*.up.sql` migration per table, containing its `CREATE TABLE` statement and the demo rows to seed it with. `migrate.php` is an `@startup` file, so the migrations run to completion before the server accepts requests:

```php
$migrate = new Database\Migrate("dbadmin");
$migrate->load("./schema/*.up.sql");
$migrate->run();
```

Each file is append only. A `migrations` table records every applied statement by index, so a later change goes in as new statements at the end of the file; editing or removing statements that already ran leaves databases inconsistent with each other. Adding a table means adding `schema/<table>.up.sql`.

## Tests

`tests/venom.yml` is a [venom](https://github.com/ovh/venom) suite covering every route the demo registers. It runs against the compose service, which has no published port, so `tests/atkins.yml` resolves the container address for it:

```sh
cd tests && atkins test
```

Every step is `curl` under an `exec` step, sharing one cookie jar, because the
suite signs in: venom's http executor follows redirects and cannot publish a
captured cookie to a later step, so there is nowhere to keep a session id. The
jar is a file, which is all the state the suite needs.

It runs against a locally started server too, with the reload command named for
that process:

```sh
cd tests && venom run venom.yml \
  --var=host=http://127.0.0.1:8080 \
  --var='reload=pkill -HUP -x phpscript'
```

The suite is idempotent: it removes its own leftovers before it starts - the
scratch tables, and the account the reload case appends to `.htpasswd` - and
edits rows in a scratch table so the seeded rows keep the ids the migration
gave them.

> **Local demo only:** the login is a real session over a real password file,
> but this application's SQL console intentionally executes arbitrary SQL for
> anyone who signs in, and it has no authorization beyond "has an account". Do
> not expose it to an untrusted network or use it against a production
> database.
