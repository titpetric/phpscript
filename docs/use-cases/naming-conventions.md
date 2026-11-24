# Naming conventions

Two naming systems meet in a phpscript tree. The runtime reads a small set of
names at fixed positions: the document root, the site configuration, the
password file. A site may put anything else wherever it likes. The Go and PHP
names the runtime registers are a separate contract, recorded in
[Naming conventions](../naming-conventions.md).

## The three names a site relies on

These are the names an operator or a site author meets first. Each is a
default in the implementation, not a rule invented by a template.

| Name         | Position                                       | What reads it                                                                 |
|--------------|------------------------------------------------|-------------------------------------------------------------------------------|
| `public/`    | below the application root                     | `document_root: public` in `config/config.yml`, and the fallback in `VirtualHost.load` |
| `.htpasswd`  | in the application root                        | `Auth\Storage\PasswordFile`, whose `DefaultPasswordFile` is `.htpasswd`       |
| `sites/`     | wherever the operator keeps the sites          | the operator's `virtualhost` list. The runtime has no `sites/` default        |

`public/` and `.htpasswd` are defaults in code. `sites/` is a convention this
repository uses in its examples. The server reads the `root` of each
`virtualhost` entry, so the directory above the roots may be named anything.

## The document root: `public/`

`document_root` is the directory below the application root that the server
publishes. The embedded default is `public`:

```yaml
# config/config.yml
document_root: public
```

The server answers a request that names a directory with that directory's
`index.php`, then its `index.html` when no `index.php` is present. `/` is
`public/index.php` or `public/index.html`. A site without either is a 404,
unless `autoindex: true` asks for a listing.

A site that already calls its web root `web` sets the key instead of renaming
the tree:

```yaml
# blog/phpscript.yml
document_root: web
```

Nothing above `public/` has a URL. `phpscript.yml`, `routes/`, `templates/`,
`vendor/` and `tests/` sit beside the document root and are not published. A
site that stores uploads below `public/upload` names it in
`runner.writable_paths`. The server then serves a `.php` file there as bytes
and never executes it.

## The password file: `.htpasswd`

`Auth\Storage\PasswordFile` is the user database a login reads. It holds
apache's format: one `username:hash` line per account. The constructor resolves
a relative path inside the application root. With no argument it reads
`.htpasswd` there:

```php
$users = new Auth\Storage\PasswordFile();
$manager = new Session\Manager();          // the same store, by default
```

`Session\Manager` defaults to `Auth\Storage\PasswordFile` and
`Session\Storage\Disk`. The file is the application's, so each site with its
own root reads its own accounts. Write it with `htpasswd -B`:

```sh
htpasswd -B -C 10 .htpasswd admin
```

Bcrypt and argon2 lines verify. `$apr1$` and `{SHA}` are read for compatibility
with files that already exist. Plain text and crypt DES lines are refused. The
refusal is a decision: a line no format claims also resembles a truncated hash,
and accepting it would make a damaged file authenticate.

## A tree of sites: `sites/`

One `phpscript server` answers for more than one site when the operator lists
them. The convention is one directory per site below `sites/`, with the
operator's file beside them:

```text
sites/
├── server.yml
├── shop/
│   ├── phpscript.yml
│   ├── public/
│   └── .htpasswd
└── blog/
    ├── phpscript.yml
    └── web/
```

```yaml
# sites/server.yml
server:
  addr: "127.0.0.1:8080"

virtualhost:
  - domain: shop.example.com
    root: shop
  - domain: blog.example.com
    root: blog
```

```sh
phpscript -f sites/server.yml server
```

Each root holds its own `phpscript.yml`. The file is named `phpscript.yml` in
the application root and has no second location. A site's file may set the
document root, the runner, the routes, the telemetry, the environment and the
mail servers. It may not set `server` or `virtualhost`: the listen address and
the list of sites belong to the operator. The server refuses a file that tries.

The domain names are exact after lowercasing, the port and a trailing dot are
removed. One entry may list several names, `example.com www.example.com`, and
every one reaches the same site. The first name is the one errors and module
names report.

## What a site may contain

The names below are conventions, not requirements. A site needs only a
document root. The rest are the names the examples and the demo trees use.

| Name              | What it holds                                                                 |
|-------------------|-------------------------------------------------------------------------------|
| `phpscript.yml`   | the site's configuration, read over the operator's defaults                    |
| `public/`         | the document root, `document_root` when it is not the default                 |
| `.htpasswd`       | the accounts `Auth\Storage\PasswordFile` reads by default                     |
| `routes/`         | PHP files with `@route` annotations, scanned outside the document root         |
| `templates/`      | templates the application includes                                           |
| `vendor/`         | composer packages, loaded with `--include vendor/autoload.php` or `runner.include` |
| `tests/`          | `.phpt` fixtures and the scripts `phpscript test ./...` runs                  |
| `migrations/`     | SQL or PHP a `@startup` job applies                                           |
| `dbadmin/`        | the phpMyAdmin-style demo, when a tree carries one                            |

`@route`, `@startup` and `@schedule` are comments, not files. The scanner reads
them in PHP files outside `public/`. A route file may be anywhere the scan
reaches. A directory named in `runner.writable_paths` is skipped, because a
visitor can write there.

## Names the runtime registers

The names a script calls are a compatibility surface, and the rules that settle
them are in the [naming conventions](../naming-conventions.md) for the Go and
PHP surface. A name PHP already defines keeps PHP's spelling and behaviour.
Everything else is a class named for the subject a script works with, such as
`Database`, `Session\Manager` or `SharedMemory`. A single call registers a
function, not a class, and takes PHP's lowercase spelling whether PHP defines
it or not.
