# Security

This document states the posture a phpscript deployment must hold: the script's reach, its limits, and which limits the code enforces today. Every claim names the deciding file. A stale claim is findable. A section describing something nothing implements says so in that sentence.

The governing decision, from [issue 139](https://github.com/titpetric/phpscript/issues/139), is that phpscript never manages a raw connection string on a script's behalf. A script names a database connection or a mail server. The credential stays in the host's configuration. No binding reads one back. No binding reaches another virtual host or another document root. The applications phpscript runs are semi-trusted: the design draws the boundaries for a compromised script. None of this is a claim about a compromised host.

## Creating a virtual host

The order these go in is the order a mistake in them costs the most.

1. **Point the site's `root` at the site's tree. No directory above it may be a parent.** That root anchors every filesystem binding, every `include` and the document root (`cmd/phpscript/server/vhost.go:177`, `cmd/phpscript/server/server_run.go:530`). A root that is a parent of another site's root is two sites in one tree.
2. **Declare `env` in the site's `phpscript.yml`, even when empty.** The list replaces wholesale. A site declaring none inherits every connection the operator configured, including the shipped connection in `config/config.yml`. `env: []` declares no database.
3. **Declare `mail`, even when empty.** Same rule, same reason. `mail: {}` tells the runtime to send nothing.
4. **Set `env: []` and `telemetry.enabled: false` in the operator's own file.** The first stops the shipped connection reaching a site that declared nothing. The second leaves the platform's dashboard unmounted, because the root router mounts it in front of the host mux, where it answers on every domain, even a domain no entry claims.
5. **Name `writable_paths`.** An empty list, the default, allows every write inside the root. A named list also stops the server executing a `.php` file that lands in one. Annotation scanning there stops too (`cmd/phpscript/server/server_run.go:590`).
6. **Mount the writable volume from outside, and keep durable state out of the tree.** A sqlite file belongs at an absolute DSN. Uploads belong in a directory the operator bind-mounts. See [Writable storage](#writable-storage).
7. **Decide the capability set, and start the server with `--stdlib=secure`.** The server installs `exec`, gd, the HTTP client and disk session storage by default. Each binding widens a tenant's reach. Today the flag drops only `exec`. See [Virtual host capabilities](#what-a-virtual-host-can-do).
8. **Decide who reaches the site dashboard.** A site that enables `telemetry` mounts an unauthenticated front end on the site domain. Its traces record the values bound to database queries. [Access control](telemetry.md#access-control) names three ways to answer that, none of which the configuration file sets.

[Virtual hosting](use-cases/virtual-hosting.md) works a two site server through in full, and [Configuration](configuration.md#virtual-hosts) is the reference for every key named above.

## Credentials a script cannot spell

A script names connections and mail servers, never describes their settings. The server builds both providers per site from that site's configuration. They hold no more than that (`cmd/phpscript/server/vhost.go:155`, `:160`). A name another site configured does not resolve, and nothing refuses it.

| Surface                                | What a script gets                                   | What it cannot get                                                                                                                         |
|----------------------------------------|------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------|
| `new Database($name)`                  | A client, or a failure naming `$name`                | The DSN, and the client under the read-only flag. `get_object_vars()` reports the name and the flag, `json_encode()` returns no credential |
| `Database::connections()`              | The names this site can open, sorted                 | Any DSN behind a name                                                                                                                      |
| `Database::register($name, $dsn)`      | A connection added to this site's own provider       | Reach into another site's provider                                                                                                         |
| `new Mail($name)`, `mail()`            | A delivery, or a failure naming `$name`              | A host, a username or a password. There is no listing call and the object has no properties                                                |
| `new Auth\Storage\PasswordFile($file)` | A verdict on one credential, and the record it names | Any stored hash, and the file's contents. The store has no properties                                                                      |
| `getenv()`                             | The variables the site's `env` declared              | Anything named `PLATFORM_*`, and anything the operator's process holds                                                                     |

`Database::register` is the only place a connection string enters from PHP. A script supplies a string it already holds, and no binding reads it back. The credential goes into the provider that site already resolves through (`stdlib/ps/database/register.go:59`). `Mail` has no counterpart: naming what exists is itself a disclosure. The constructor refuses settings without coercion. [Credentials stay with the host](configuration.md#credentials-stay-with-the-host) is the full account.

`runner.InfrastructurePrefix` keeps connection strings out of `getenv()` (`runner/runtime.go:407`). It applies to a variable the site declared itself: a site listing `PLATFORM_DB_SHOP` gets the connection and reads nothing back for the name.

### The user database

`Auth\Storage\PasswordFile` holds a site's accounts by the same rule: the site names the file, the runtime reads it. Verification happens below the boundary. The format is apache's `htpasswd`, which `mod_authn_file` reads. A LAMP stack puts the same work in the same place - php sees `$_SERVER["PHP_AUTH_USER"]` after the web server has decided, and holds no secret store itself.

A script holds a store it can ask one question of, plus the record an accepted credential names: a username plus a primary key computed from the record (`stdlib/ps/auth/user.go`). No property holds a hash. No listing of hashes exists. No method returns one. `users()` lists the accounts and their keys, which is the file as two columns. The third column does not leave the package. Every call reads the file and drops the parse on return. A hash stays a local, as a mail credential already does (`stdlib/ps/mail/provider.go:86`).

The path resolves inside the application root, as every path does. A site cannot read a neighbour's file, nor name one outside its tree. Keeping the file out of the document root remains the site's job: the server publishes `public/`. A client fetches a password file there like any other asset.

The constructor refuses a file that exists but is unreadable, naming the line that stopped it, ahead of the login it would break. It also refuses the byte order mark of a utf-8 file ahead of the first username. Otherwise the parse leaves the mark in the name, that account matches nothing a login types, and nobody learns why. The line takes one of these formats:

| Line                           | Written by                     | What a guess costs an attacker                                |
|--------------------------------|--------------------------------|---------------------------------------------------------------|
| `$2y$`, `$2a$`, `$2b$`, `$2x$` | `htpasswd -B`, `password_hash` | The cost the hash records; 214 ms at cost 12 on an Intel N150 |
| `$argon2id$`, `$argon2i$`      | `password_hash`                | The parameters the hash records, memory included              |
| `$apr1$`                       | `htpasswd` with no flag        | 1000 md5 compressions, tens of microseconds                   |
| `{SHA}`                        | `htpasswd -s`                  | One sha1, and no salt: a wordlist lookup per row              |
| plain text, crypt DES          | `htpasswd -p`, `htpasswd -d`   | Refused                                                       |
| `$2$`, bcrypt with no revision | bcrypt before the 2a revision  | Refused                                                       |

The refusal of the last two rows is a decision. Apache reads a plain password on some platforms. A line no format claims also resembles a truncated or corrupted hash, and accepting the class would make a damaged file authenticate. The bcrypt row reads exactly the four revision markers php's crypt reads: for `$2$`, the 1997 spelling without a minor revision, php answers the failure string `*0`, where Go's implementation still verifies the body, while a `$2$` line would sign a user in here and nowhere else. `password_verify` answers false for it in both runtimes (`tests/fixtures/stdlib/password_bcrypt_revisions.phpt` settles that against the php binary). The two weak rows record no cost parameter worth anything. This build reads them for compatibility with files that already exist. Write files with `htpasswd -B`. [Hashing](reference/extensions/hashing.md#what-a-derivation-costs) prices the first two.

A file the parse accepts can still hold no account: an operator stages a file of comments, or sweeps the last user out of one. `exists()` answers false for it, as for a missing file. A login page then says there are no accounts, where a form refusing every password would name no cause.

Removing an account ends its sessions, which is a sweep and not a check on the current request. A user holds a session per client, and `$session->delete_by_user($id)` drops them all. It answers how many it took. A session holds its user's primary key as its whole payload. A store matches on that and keeps no user index. The manager clears the session of a user the sweep removed. The `@startup` job that rebuilds a site's user table notices the removal and that job signs the account out: `demos/dbadmin/users.php` reads the ids the table held, refills it from the file, and sweeps whatever the file stopped naming.

A sign-in is the only request that pays a password derivation. The demo bounds the spend of an unauthenticated caller. `demos/dbadmin/login.php` holds every address and account name to five refusals in ten seconds. It answers the sixth post with 429 before anything reads the credential: at bcrypt cost 10 that ceiling caps one client at about three percent of a core ([hashing.md](reference/extensions/hashing.md#what-a-derivation-costs) prices the derivation and names this lever). The sign-in and sign-out forms also send a double-submit token (`demos/dbadmin/setup.php`). A post must return the value the `csrf` cookie holds. The session cookie's `SameSite=Lax` already stops a cross-site post from riding a session the browser holds. The token covers what `SameSite` cannot. A forged sign-in holds no session for `SameSite` to withhold - the attack posts the attacker's own credentials to sign the visitor into an attacker-controlled account - while a client that ignores `SameSite` still cannot produce a field that matches a cookie it can neither read nor set.

The password file is not the `secrets:` block of issue 139 item 5 and does not advance it. It holds one kind of credential, compared by a login, with no decryption.

## Service-managed and vhost-managed configuration

Two keys belong to the operator. A site setting either fails startup, with nothing dropped quietly (`config/config_virtualhost.go:17`): `server`, because the listen address is the operator's, and `virtualhost`, since a site holds no sites. Everything else in the model belongs to the site, read from `root/phpscript.yml` over whatever `-f` produced.

Key combining rules decide site inheritance:

| Key                                          | Combining             | A site that declares nothing                      |
|----------------------------------------------|-----------------------|---------------------------------------------------|
| `env`                                        | replaced whole        | Inherits every connection the operator configured |
| `mail`                                       | replaced whole        | Inherits every server the operator configured     |
| `runner`, `telemetry`, `routes`, `flatstack` | merged field by field | Inherits the operator's value for each field      |

`env` and `mail` replace whole, since a merge leaks. One unnamed server is a struct. A site setting only `host` and `from` would keep the operator's `username` and `password`, then authenticate as the operator. Both are collections keyed by name for that reason.

A merged block has the mirror-image property: a site's `runner.writable_paths` replaces the operator's list for that site and the operator cannot confine a site that declares one. Confining a tenant's writes is the filesystem's job and not the configuration's.

The startup checks run over the whole list before the server listens, and `phpscript -t` runs them without starting anything. [Startup checks](configuration.md#startup-checks) lists them.

## What a virtual host can do

A process started without `--stdlib` installs every binding package `stdlib/imports.go` imports (`stdlib/stdlib.go:17`). Every site gets the whole standard library, including the parts that reach outside the request:

| Capability                                 | Binding             | What it reaches                                                                      |
|--------------------------------------------|---------------------|--------------------------------------------------------------------------------------|
| `exec`, `system`, `passthru`, `shell_exec` | `stdlib/ps/pexec`   | A process at the privilege of the user running the server                            |
| `new HTTP\Client`                          | `stdlib/ps/http`    | Any address the host can route to, with headers the script sets                      |
| `new HTTP\Server($addr, ...)`              | `stdlib/ps/http`    | A listening socket on an address the script names                                    |
| `new Session\Storage\Disk($path)`          | `stdlib/ps/session` | A directory inside the root, or the host's temporary directory when it names no path |
| `new Auth\Storage\PasswordFile($file)`     | `stdlib/ps/auth`    | A file inside the root, read and never written; no hash it holds reaches the script  |
| `imagecreatefrompng`, `imagepng`           | `stdlib/ps/gd`      | A path inside the root, writes held to `writable_paths`                              |

`--stdlib` leaves the first row out:

```sh
phpscript --stdlib=secure server
```

The flag names the binding areas the process installs. `secure` covers every binding confined to the runtime's sandbox. An area it omits is not registered. Its names stay undefined and nothing refuses a call. A direct call, a variable function, `call_user_func`, a callable, either engine: all resolve through a single runtime function table. `stdlib.Mount(rt, profile, bindings...)` is the same selection for a host embedding the package.

`--stdlib` is a single process value. An operator decides the reach of the runtime they started, with no per-site key. A capability granted in the operator's file for one tenant is readable by the tenant's neighbours. `virtualhost` is already refused in a site's `phpscript.yml`. A compromised tree cannot grant itself anything.

## The filesystem boundary

Every path a script names resolves against the root the bindings hold. No spelling escapes the root. A path written from `/` names the root itself, the spelling `getcwd()`, `realpath()`, `__DIR__` and `__FILE__` all answer with. phpscript collapses a `..` against the root and leaves no way out (`stdlib/ps/files/files.go:75`, `runner/runtime.go:680`). There is no spelling for a host path:

```php
file_get_contents("/etc/hostname");   // false
file_exists("/etc/hostname");         // false
scandir("/");                         // the site's own root
```

Writes are additionally held to `writable_paths`. A refused write throws, where an operating system refusal returns `false`. Two properties of that list contradict the name:

- **An empty list allows every write inside the root.** The list is not deny-by-default. A site that names no `writable_paths` may write anywhere in the site tree.
- **The list accepts an absolute entry that then matches nothing.** `WritableRoots` keeps the entry verbatim, but the check compares it to a path already joined onto the root. The entry can never match. The write throws exactly as a missing entry would. No way grants a write outside the root.

[Writable paths](configuration.md#writable-paths) is the reference for the list and the functions held to it.

### Where phpscript leaves the root behind

Three places reach the host filesystem directly, and all three are decisions.

| Place                                    | Where                                  | Status                                                                                                                                            |
|------------------------------------------|----------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------|
| `exec` and the three functions beside it | `stdlib/ps/pexec/exec.go:40`           | By design. A command is a process, and the root only decides where it starts                                                                      |
| An uploaded part                         | `runner/context_upload.go:81`          | By design. Written to the system temporary directory, readable back only through the request's own registry                                       |
| `Session\Storage\Disk` with no path      | `stdlib/ps/session/storage_disk.go:67` | By design. A directory of this application's own below the host's temporary directory, named after a digest of the root and no script can name it |

Two more rows belong to this table's history and not to the list above. `Session\Storage\Disk($path)` resolves the path a script names through the runtime's rule and holds it to `writable_paths` (`stdlib/ps/session/storage_disk.go:36`). gd does the same for an image path (`stdlib/ps/gd/gd.go:146`). Every binding resolves an absolute path to the root, and `file_exists()` agrees with its neighbours. [tests/fixtures/bindings/session_disk_root.phpt](../tests/fixtures/bindings/session_disk_root.phpt) pins the session half.

### Writable storage

Issue 139 names `/data` as a site's writable filesystem, mapped onto a volume the operator mounted. **That mapping does not exist.** No `/data` exists. No key names that directory. The path a script writes lands on its host path directly: `root.resolve` joins onto the real root directory. A site's `writable_paths` is the only thing between a script and its tree.

These work today until the mapping exists:

- A sqlite database at an absolute DSN, `sqlite:///srv/data/shop/shop.db`. `resolveSQLiteDSN` leaves absolute paths and `file:` URIs alone and joins only a relative one onto the root (`stdlib/ps/database/dsn.go:15`). The file sits outside the tree and survives a redeploy.
- Uploads land in a directory the operator bind-mounts onto a path inside the root, named in `writable_paths`. The site's PHP writes a relative path, leaving the mount point invisible.

Both hold whether the tree came from the operator or from a deployment that replaces it wholesale. A site that keeps durable files inside the site root loses them when the root is rebuilt. The configuration cannot express the alternative: a write cannot leave the root, and an absolute `writable_paths` entry pointing outside the root matches nothing.

The root is also not hidden from the script. `$_SERVER["DOCUMENT_ROOT"]` and `$_SERVER["SCRIPT_FILENAME"]` hold the real host path of the site's tree (`cmd/phpscript/server/server_run.go:258`, `:267`), and are the only place that spelling reaches PHP. Everything else - `getcwd()`, `realpath()`, `__DIR__`, a path in an error message - answers in the script's own.

## Isolation between sites

A site is a separate router, built once before the server listens, and the `Host` header is the only thing that selects between sites. Matching is exact and there is no default site: a `Host` no entry claims gets 404 and reaches no site's code (`cmd/phpscript/server/vhost.go:45`). [A site cannot reach another site](use-cases/virtual-hosting.md#6-what-one-domain-cannot-reach-on-the-other) demonstrates the boundaries a request meets.

What is per site, and not per process:

| Thing                                           | Built at                                 |
|-------------------------------------------------|------------------------------------------|
| Router, routes, document root, error pages      | `cmd/phpscript/server/vhost.go:122`      |
| Database provider                               | `cmd/phpscript/server/vhost.go:155`      |
| Mail provider                                   | `cmd/phpscript/server/vhost.go:160`      |
| Script environment                              | `cmd/phpscript/server/vhost.go:165`      |
| Tracer and debug front end                      | `cmd/phpscript/server/vhost.go:143`      |
| Parsed include cache, compiled expression cache | `cmd/phpscript/server/server_run.go:121` |
| `@startup` and `@schedule` jobs                 | `cmd/phpscript/server/vhost.go:215`      |

The two caches form a pair per site, created with the site's file handler and shared between its routed endpoints and its document root. One precompile pass covers both (`cmd/phpscript/server/precompile.go:13`). Sites share no cache.

A site's accounts belong to it, for the same reason its files do: `Auth\Storage\PasswordFile` resolves against the root the bindings hold. `new Session\Manager` across two sites reads two files. Neither can name the other's. Two sites that share a tree share their accounts, one more thing a parent root of another root gets wrong.

Session storage on disk uses one directory per application root, keyed by a digest of the absolute root (`stdlib/ps/session/storage_disk.go:72`). `Session\Storage\Disk::prune` takes an age and ignores IDs. When every root shared one directory, `new Session\Storage\Disk` followed by `prune()` on any site deleted each other site's sessions. Rooting the path a script names does not reach this. A script naming no path never writes one down.

`SharedMemory` is the exception, and not in the direction its name suggests. No host in this tree binds a store into a runtime context and `new SharedMemory` hands every caller a fresh empty store that lives as long as the object (`stdlib/ps/core/shared_memory.go:38`). Sites share no state because `SharedMemory` shares none, which is also why the `SharedMemory` section of `phpinfo()` never prints under `phpscript server`. Collapsing the caches behind a `runtime.Cache` and defining a per-site store are issue 139's third and fourth items.

Isolation stops at the process. One site's `exec`, `HTTP\Server` and unbounded memory share the operating system process of the other sites. A reload rebuilds every site. Updating one tenant re-runs the `@startup` jobs of each neighbour. Two tenants that must not share a fault domain need two processes.

## Not implemented yet

Each of these is a design in issue 139 with nothing behind it in the code. A document that read as a description of today would be wrong about all five.

| Intended                                                           | Issue 139 item | State                                                                    |
|--------------------------------------------------------------------|----------------|--------------------------------------------------------------------------|
| A `runtime.Cache` collapsing the per-site caches behind one setter | 4              | The caches are per site already, each set separately                     |
| A `secrets:` block, and `get_secret()` in PHP                      | 5              | No key, no binding                                                       |
| `phpscript.key` in a vhost, encrypting that block at rest          | 1, 2           | The runtime reads no key file                                            |
| `config/phpscript.yml` as a second location for a site's file      | 3              | The runtime reads no second location (`config/config_virtualhost.go:12`) |
| A writable volume presented to a site as `/data`                   | 1              | No mapping. See [Writable storage](#writable-storage)                    |

Two shapes the secrets work must keep when it lands, since the rest of this page depends on those shapes. A secret decrypted for a request stays in a local. No holder outliving the call retains it. Mail credentials already work that way (`stdlib/ps/mail/provider.go:103`). `get_secret()` names a secret the way `new Database` names a connection: a site's secrets belong to the site, no listing call exists. A name another site declared does not resolve.

## The class name on a refused write

A write refused by `writable_paths` throws, and `get_class($e)` on it reports `errorString`, a Go internal type name reaching a script. A script can catch the error. The message is correct. A script catching `Exception` runs unchanged, but one that discriminates on the class name compares it to a Go identifier. Every error a binding raises as a Go value, without constructing a PHP class, answers the same way. It follows from [one type for all throwables](design.md#exceptions-without-a-hierarchy) and is not specific to this path.

## Where to go next

- [Configuration](configuration.md) - every key and its owner
- [Virtual hosting](use-cases/virtual-hosting.md) - a two-site server, plus one domain's reach on another
- [Telemetry](telemetry.md#access-control) - trace records and front-end access
- [Design decisions](design.md) - the features phpscript will not add
- [Known divergences from PHP](README.md#known-divergences-from-php) - the sandbox entries a script sees
