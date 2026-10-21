# Security

The posture this document holds a phpscript deployment to: what a script may reach, what it may not, and which of the two the code enforces today. Every claim names the file that decides it, so a claim that stops being true is findable. Where a section describes something nothing implements, it says so in the sentence that describes it.

The governing decision, from [issue 139](https://github.com/titpetric/phpscript/issues/139), is that phpscript never manages a raw connection string on a script's behalf. A script names a database connection or a mail server; the credential stays in the host's configuration, no binding reads one back, and no binding reaches another virtual host or another document root. The applications phpscript runs are semi-trusted: the design draws the boundaries for a compromised script. Nothing here is a claim about a compromised host.

## Creating a virtual host

The order these go in is the order a mistake in them costs the most.

1. **Point the site's `root` at its own tree, and no directory above it.** That root anchors every filesystem binding, every `include` and the document root (`cmd/phpscript/server/vhost.go:180`, `cmd/phpscript/server/run.go:299`). A root that is a parent of another site's root is two sites in one tree.
2. **Declare `env` in the site's `phpscript.yml`, even when empty.** The list replaces wholesale, so a site that declares none inherits every connection the operator configured, including the one the embedded `config/config.yml` ships. `env: []` is how a site says it has no database.
3. **Declare `mail`, even when empty.** Same rule, same reason. `mail: {}` is how a site says it sends nothing.
4. **Set `env: []` and `telemetry.enabled: false` in the operator's own file.** The first stops the shipped connection reaching a site that declared nothing. The second leaves the platform's dashboard unmounted, because it sits on the root router in front of the host mux and answers on every domain, including one no entry claims.
5. **Name `writable_paths`.** An empty list, the default, allows every write inside the root. A named list also stops the server executing a `.php` file that lands in one, and stops it scanning annotations there (`cmd/phpscript/server/run.go:136`).
6. **Mount the writable volume from outside, and keep durable state out of the tree.** A sqlite file belongs at an absolute DSN; uploads belong in a directory the operator bind-mounts. See [Writable storage](#writable-storage).
7. **Decide the capability set, and start the server with `--stdlib=secure`.** The server installs `exec`, gd, the HTTP client and disk session storage by default, and each one widens what a tenant reaches. Today the flag drops only `exec`. See [What a virtual host can do](#what-a-virtual-host-can-do).
8. **Decide who reaches the site's own dashboard.** A site that enables `telemetry` mounts an unauthenticated front end on its own domain, and its traces carry the values bound to database queries. [Access control](telemetry.md#access-control) names the three ways to answer that; there is no setting in the configuration file.

[Virtual hosting](use-cases/virtual-hosting.md) works a two site server through in full, and [Configuration](configuration.md#virtual-hosts) is the reference for every key named above.

## Credentials a script cannot spell

A connection and a mail server are both named, never described. The server builds both providers per site from that site's own configuration, and they hold nothing else (`cmd/phpscript/server/vhost.go:158`, `:163`), so a name another site configured does not resolve at all, with no check to refuse it.

| Surface                                | What a script gets                                   | What it cannot get                                                                          |
|----------------------------------------|------------------------------------------------------|---------------------------------------------------------------------------------------------|
| `new Database($name)`                  | A client, or a failure naming `$name`                | The DSN. `get_object_vars()` is empty and `json_encode()` carries no credential             |
| `Database::connections()`              | The names this site can open, sorted                 | Any DSN behind a name                                                                       |
| `Database::register($name, $dsn)`      | A connection added to this site's own provider       | Reach into another site's provider                                                          |
| `new Mail($name)`, `mail()`            | A delivery, or a failure naming `$name`              | A host, a username or a password. There is no listing call and the object has no properties |
| `new Auth\Storage\PasswordFile($file)` | A verdict on one credential, and the record it names | Any stored hash, and the file's contents. The store has no properties                       |
| `getenv()`                             | The variables the site's `env` declared              | Anything named `PLATFORM_*`, and anything the operator's process carries                    |

`Database::register` is the one place a connection string enters from PHP, and there a script supplies one it already holds, with nothing read back; the credential goes into the provider that site already resolves through (`stdlib/database/register.go:59`). `Mail` has no counterpart: naming what exists is itself a disclosure, and the constructor refuses settings, with no coercion. [Credentials stay with the host](configuration.md#credentials-stay-with-the-host) is the full account.

`runner.InfrastructurePrefix` is what keeps connection strings out of `getenv()` (`runner/runtime.go:407`), and it applies to a variable the site declared itself: a site listing `PLATFORM_DB_SHOP` gets the connection and reads nothing back for the name.

### The user database

`Auth\Storage\PasswordFile` holds a site's accounts under the same rule: the site names the file, the runtime reads it, and the verification happens below the boundary. The format is apache's `htpasswd`, the one `mod_authn_file` reads, and a LAMP stack puts the same work in the same place - php sees `$_SERVER["PHP_AUTH_USER"]` after the web server has decided, and carries no secret store of its own.

What a script holds is a store it can ask one question of, and the record an accepted credential names: a username, and a primary key computed from it (`stdlib/auth/user.go`). There is no property carrying a hash, no listing of hashes, and no method that returns one. `users()` lists the accounts and their keys, which is the file as two columns; the third column does not leave the package. Every call reads the file and drops the parse on return, so a hash stays a local the way a mail credential already does (`stdlib/mail/provider.go:86`).

The path resolves inside the application root like any other, so a site cannot read its neighbour's file and cannot name one outside its tree. Keeping the file out of the document root is still the site's job: the server publishes `public/`, and a client fetches a password file there like any other asset.

The constructor refuses a file that is there and unreadable, naming the line that stopped it, ahead of the login it would break. It also refuses a utf-8 byte order mark ahead of the first username: the parse would otherwise leave the mark in the name, so that account would match nothing a login types and nobody would learn why. These are the formats a line may carry:

| Line                           | Written by                     | What a guess costs an attacker                                |
|--------------------------------|--------------------------------|---------------------------------------------------------------|
| `$2y$`, `$2a$`, `$2b$`, `$2x$` | `htpasswd -B`, `password_hash` | The cost the hash records; 214 ms at cost 12 on an Intel N150 |
| `$argon2id$`, `$argon2i$`      | `password_hash`                | The parameters the hash records, memory included              |
| `$apr1$`                       | `htpasswd` with no flag        | 1000 md5 compressions, tens of microseconds                   |
| `{SHA}`                        | `htpasswd -s`                  | One sha1, and no salt: a wordlist lookup per row              |
| plain text, crypt DES          | `htpasswd -p`, `htpasswd -d`   | Refused                                                       |
| `$2$`, bcrypt with no revision | bcrypt before the 2a revision  | Refused                                                       |

The refusal of the last two rows is a decision. Apache reads a plain password on some platforms, and a line no format claims is also what a truncated or corrupted hash looks like, so accepting the class would turn a damaged file into one that authenticates. The bcrypt row reads exactly the four revision markers php's crypt reads: for `$2$`, the 1997 spelling with no minor revision, php answers the failure string `*0` where Go's implementation still verifies the body, so a `$2$` line would sign a user in here and nowhere else, and `password_verify` answers false for it in both runtimes (`tests/fixtures/stdlib/password_bcrypt_revisions.phpt` settles that against the php binary). The two weak rows carry no cost parameter worth anything, and this build reads them for compatibility with files that already exist; `htpasswd -B` is what to write. [Hashing](reference/extensions/hashing.md#what-a-derivation-costs) prices the first two.

A file the parse accepts can still hold no account: an operator stages a file of comments, or sweeps the last user out of one. `exists()` answers false for it, as for a missing file, so a login page says there are no accounts where a form refusing every password would name no cause.

Removing an account ends its sessions, which is a sweep and not a check on the current request: a user holds a session per client, and `$session->delete_by_user($id)` drops every one of them and answers how many it took. A session carries its user's primary key as its whole payload, so a store matches on that and keeps no index of its own, and the manager clears its own session when the sweep took it. The `@startup` job that rebuilds a site's user table notices the removal, so that job signs the account out: `demos/dbadmin/users.php` reads the ids the table held, refills it from the file, and sweeps whatever the file stopped naming.

The password file is not the `secrets:` block of issue 139 item 5, and does not advance it: it holds one kind of credential, the one a login compares against, and nothing decrypts.

## Service-managed and vhost-managed configuration

Two keys belong to the operator, and a site setting either fails startup, with nothing dropped quietly (`config/virtualhost.go:17`): `server`, because the listen address is the operator's, and `virtualhost`, because a site holds no sites. Everything else in the model is the site's, read from `root/phpscript.yml` over whatever `-f` produced.

How a key combines decides what a site inherits:

| Key                                          | Combining             | A site that declares nothing                      |
|----------------------------------------------|-----------------------|---------------------------------------------------|
| `env`                                        | replaced whole        | Inherits every connection the operator configured |
| `mail`                                       | replaced whole        | Inherits every server the operator configured     |
| `runner`, `telemetry`, `routes`, `flatstack` | merged field by field | Inherits the operator's value for each field      |

`env` and `mail` replace because a merge leaks. A single unnamed server is a struct, and a site setting only `host` and `from` would keep the operator's `username` and `password` and go on authenticating as the operator. That is why both are collections keyed by name.

A merged block has the mirror-image property: a site's `runner.writable_paths` replaces the operator's list for that site, so the operator cannot confine a site that declares one. Confining a tenant's writes is the filesystem's job and not the configuration's.

The startup checks run over the whole list before the server listens, and `phpscript -t` runs them without starting anything. [Startup checks](configuration.md#startup-checks) lists them.

## What a virtual host can do

A process started without `--stdlib` installs every binding package `stdlib/imports.go` imports (`stdlib/stdlib.go:17`), so every site gets the whole standard library, including the parts that reach outside the request:

| Capability                                 | Binding          | What it reaches                                                                      |
|--------------------------------------------|------------------|--------------------------------------------------------------------------------------|
| `exec`, `system`, `passthru`, `shell_exec` | `stdlib/pexec`   | A process at the privilege of the user running the server                            |
| `new HTTP\Client`                          | `stdlib/http`    | Any address the host can route to, with headers the script sets                      |
| `new HTTP\Server($addr, ...)`              | `stdlib/http`    | A listening socket on an address the script names                                    |
| `new Session\Storage\Disk($path)`          | `stdlib/session` | A directory inside the root, or the host's temporary directory when it names no path |
| `new Auth\Storage\PasswordFile($file)`     | `stdlib/auth`    | A file inside the root, read and never written; no hash it holds reaches the script  |
| `imagecreatefrompng`, `imagepng`           | `stdlib/gd`      | A path inside the root, writes held to `writable_paths`                              |

`--stdlib` leaves the first row out:

```sh
phpscript --stdlib=secure server
```

The flag names the binding areas the process installs, and `secure` is every binding confined to the runtime's sandbox. An area it omits is not registered, so its names stay undefined and nothing refuses them, through a direct call, a variable function, `call_user_func`, a callable and both engines alike: they all resolve through the one runtime function table. `stdlib.Mount(rt, profile, bindings...)` is the same selection for a host embedding the package.

`--stdlib` is one value for the process. An operator decides what the runtime they started may reach, so there is no per-site key: a capability granted in the operator's file for one tenant is a capability the tenant's neighbours can read, and `virtualhost` is already refused in a site's own `phpscript.yml` so a compromised tree cannot grant itself anything.

## The filesystem boundary

Every path a script names resolves against the root the bindings hold, and no spelling escapes it. A path written from `/` names the root itself, the spelling `getcwd()`, `realpath()`, `__DIR__` and `__FILE__` all answer with, and phpscript collapses a `..` against the root with no way out of it (`stdlib/files/files.go:80`, `runner/runner.go:1139`). There is no spelling for a host path:

```php
file_get_contents("/etc/hostname");   // false
file_exists("/etc/hostname");         // false
scandir("/");                         // the site's own root
```

Writes are additionally held to `writable_paths`, and a refused write throws where an operating system refusal returns `false`. Two properties of that list are not what its name suggests:

- **An empty list allows every write inside the root.** The list is not deny-by-default. A site that names no `writable_paths` may write anywhere in its own tree.
- **The list accepts an absolute entry that then matches nothing.** `WritableRoots` keeps it verbatim, but the check compares it against a path already joined onto the root, so the entry can never match and the write throws exactly as it would with no entry at all. There is no way to grant a write outside the root.

[Writable paths](configuration.md#writable-paths) is the reference for the list and the functions held to it.

### Where phpscript leaves the root behind

Three places reach the host filesystem directly, and all three are decisions.

| Place                                    | Where                               | Status                                                                                                                                            |
|------------------------------------------|-------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------|
| `exec` and the three functions beside it | `stdlib/pexec/exec.go:40`           | By design. A command is a process, and the root only decides where it starts                                                                      |
| An uploaded part                         | `runner/request.go:739`             | By design. Written to the system temporary directory, readable back only through the request's own registry                                       |
| `Session\Storage\Disk` with no path      | `stdlib/session/storage_disk.go:67` | By design. A directory of this application's own below the host's temporary directory, named after a digest of the root, so no script can name it |

Two more rows belong to this table's history and not to the list above. `Session\Storage\Disk($path)` resolves the path a script names through the runtime's rule and holds it to `writable_paths` (`stdlib/session/storage_disk.go:36`), and gd does the same for an image path (`stdlib/gd/gd.go:146`), so every binding resolves an absolute path to the root and `file_exists()` agrees with its neighbours. [tests/fixtures/bindings/session_disk_root.phpt](../tests/fixtures/bindings/session_disk_root.phpt) pins the session half.

### Writable storage

Issue 139 names `/data` as a site's writable filesystem, mapped onto a volume the operator mounted. **That mapping does not exist.** There is no `/data`, no key naming one, and no indirection between the path a script writes and the host path it lands on: `root.resolve` joins onto the real root directory, and a site's own `writable_paths` is the only thing between a script and its tree.

What works today, and what a site builds on until the mapping exists:

- A sqlite database at an absolute DSN, `sqlite:///srv/data/shop/shop.db`. `resolveSQLiteDSN` leaves an absolute path and a `file:` URI alone and joins only a relative one onto the root (`stdlib/database/dsn.go:15`), so the file sits outside the tree and survives a redeploy.
- Uploads in a directory the operator bind-mounts over a path inside the root, named in `writable_paths`. The site's PHP writes a relative path and never learns the path is a mount.

Both hold whether the tree is one the operator put there or one a deployment replaces wholesale. A site that keeps durable files inside its own root loses them the first time the root is rebuilt, and the configuration cannot express the alternative: a write cannot leave the root, and an absolute `writable_paths` entry pointing outside it matches nothing.

The root is also not hidden from the script. `$_SERVER["DOCUMENT_ROOT"]` and `$_SERVER["SCRIPT_FILENAME"]` carry the real host path of the site's tree (`cmd/phpscript/server/run.go:245`, `:254`), and are the one place that spelling reaches PHP. Everything else - `getcwd()`, `realpath()`, `__DIR__`, a path in an error message - answers in the script's own.

## Isolation between sites

A site is a router of its own, built once before the server listens, and the `Host` header is the only thing that selects between sites. Matching is exact and there is no default site: a `Host` no entry claims gets 404 and reaches no site's code (`cmd/phpscript/server/vhost.go:45`). [What one domain cannot reach on the other](use-cases/virtual-hosting.md#6-what-one-domain-cannot-reach-on-the-other) demonstrates the boundaries a request meets.

What is per site, and not per process:

| Thing                                           | Built at                            |
|-------------------------------------------------|-------------------------------------|
| Router, routes, document root, error pages      | `cmd/phpscript/server/vhost.go:126` |
| Database provider                               | `cmd/phpscript/server/vhost.go:158` |
| Mail provider                                   | `cmd/phpscript/server/vhost.go:163` |
| Script environment                              | `cmd/phpscript/server/vhost.go:168` |
| Tracer and debug front end                      | `cmd/phpscript/server/vhost.go:141` |
| Parsed include cache, compiled expression cache | `cmd/phpscript/server/run.go:125`   |
| `@startup` and `@schedule` jobs                 | `cmd/phpscript/server/vhost.go:217` |

The two caches are one pair per site, created with the site's file handler and shared between its routed endpoints and its document root so that one precompile pass covers both (`cmd/phpscript/server/precompile.go:13`). Sites share no cache.

A site's accounts are its own for the same reason its files are: `Auth\Storage\PasswordFile` resolves against the root the bindings hold, so `new Session\Manager` on two sites reads two files and neither can name the other's. Two sites that share a tree share their accounts, which is one more thing a root that is a parent of another root gets wrong.

Session storage on disk is one directory per application root, keyed by a digest of the absolute root (`stdlib/session/storage_disk.go:72`). `Session\Storage\Disk::prune` takes an age and not an ID, so while every root shared one directory, `new Session\Storage\Disk` followed by `prune()` on any site deleted every other site's sessions. Rooting the path a script names does not reach this, because a script that names no path never writes one down.

`SharedMemory` is the exception, and not in the direction its name suggests. No host in this tree binds a store into a runtime context, so `new SharedMemory` hands every caller a fresh empty store that lives as long as the object (`stdlib/core/shared_memory.go:38`). Sites share nothing because `SharedMemory` shares nothing at all, which is also why the `SharedMemory` section of `phpinfo()` never prints under `phpscript server`. Collapsing the caches behind a `runtime.Cache` and defining a per-site store are issue 139's third and fourth items.

Isolation stops at the process. One site's `exec`, one site's `HTTP\Server` and one site's unbounded memory are the same operating system process as every other site's, and a reload rebuilds every site, so updating one tenant re-runs every other tenant's `@startup` jobs. Two tenants that must not share a fault domain are two processes.

## Not implemented yet

Each of these is a design in issue 139 with nothing behind it in the code. A document that read as a description of today would be wrong about all five.

| Intended                                                           | Issue 139 item | State                                                        |
|--------------------------------------------------------------------|----------------|--------------------------------------------------------------|
| A `runtime.Cache` collapsing the per-site caches behind one setter | 4              | The caches are per site already, each set on its own         |
| A `secrets:` block, and `get_secret()` in PHP                      | 5              | No key, no binding                                           |
| `phpscript.key` in a vhost, encrypting that block at rest          | 1, 2           | Nothing reads a key file                                     |
| `config/phpscript.yml` as a second location for a site's file      | 3              | Nothing reads a second location (`config/virtualhost.go:12`) |
| A writable volume presented to a site as `/data`                   | 1              | No mapping. See [Writable storage](#writable-storage)        |

Two shapes the secrets work has to keep when it lands, because the rest of this page rests on them. A secret decrypted for a request lives in a local, and nothing outliving the call holds it. A mail credential already works that way (`stdlib/mail/provider.go:103`). And `get_secret()` names a secret the way `new Database` names a connection: a site's secrets are its own, there is no listing call, and a name another site declared does not resolve.

## The class name on a refused write

A write refused by `writable_paths` throws, and `get_class($e)` on it reports `errorString` - a Go internal type name reaching a script. A script can catch it and the message is correct, so a script catching `Exception` runs unchanged, but one that discriminates on the class name is matching against a Go identifier. Every error a binding raises as a Go value, without constructing a PHP class, answers the same way. It follows from [one type for all throwables](design.md#exceptions-without-a-hierarchy) and is not specific to this path.

## Where to go next

- [Configuration](configuration.md) - every key, and which of them belong to the operator
- [Virtual hosting](use-cases/virtual-hosting.md) - a two site server, and what one domain cannot reach on the other
- [Telemetry](telemetry.md#access-control) - what a trace records, and who can read the front end
- [Design decisions](design.md) - the features phpscript will not add
- [Known divergences from PHP](README.md#known-divergences-from-php) - the sandbox entries a script sees
