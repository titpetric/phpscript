# Security

The posture a phpscript deployment is held to: what a script may reach, what it may not, and which of the two is enforced by code today. Every claim names the file that decides it, so a claim that stops being true is findable. Where a section describes something nothing implements, it says so in the sentence that describes it.

The governing decision, from [issue 139](https://github.com/titpetric/phpscript/issues/139), is that phpscript never manages a raw connection string on a script's behalf. A script names a database connection or a mail server; the credential stays in the host's configuration, no binding reads one back, and no binding reaches another virtual host or another document root. The applications phpscript runs are semi-trusted: the boundaries are drawn for a compromised script. Nothing here is a claim about a compromised host.

## Creating a virtual host

The order these go in is the order a mistake in them costs the most.

1. **Point the site's `root` at its own tree, and no directory above it.** Every filesystem binding, every `include` and the document root are anchored there (`cmd/phpscript/server/vhost.go:180`, `cmd/phpscript/server/run.go:299`). A root that is a parent of another site's root is two sites in one tree.
2. **Declare `env` in the site's `phpscript.yml`, even when it is empty.** The list replaces wholesale, so a site that declares none inherits every connection the operator configured, including the one the embedded `config/config.yml` ships. `env: []` is how a site says it has no database.
3. **Declare `mail`, even when it is empty.** Same rule, same reason. `mail: {}` is how a site says it sends nothing.
4. **Set `env: []` and `telemetry.enabled: false` in the operator's own file.** The first stops the shipped connection reaching a site that declared nothing. The second leaves the platform's dashboard unmounted, because it sits on the root router in front of the host mux and answers on every domain, including one no entry claims.
5. **Name `writable_paths`.** An empty list, the default, allows every write inside the root. A named list also stops the server executing a `.php` file that lands in one, and stops it scanning annotations there (`cmd/phpscript/server/run.go:136`).
6. **Mount the writable volume from outside, and keep durable state out of the tree.** A sqlite file belongs at an absolute DSN; uploads belong in a directory the operator bind-mounts. See [Writable storage](#writable-storage).
7. **Decide the capability set, and start the server with `--stdlib=secure`.** `exec`, gd, the HTTP client and disk session storage are installed by default, and each one widens what a tenant reaches. Only `exec` can be left out today. See [What a virtual host can do](#what-a-virtual-host-can-do).
8. **Decide who reaches the site's own dashboard.** A site that enables `telemetry` mounts an unauthenticated front end on its own domain, and its traces carry the values bound to database queries. [Access control](telemetry.md#access-control) names the three ways to answer that; there is no setting in the configuration file.

[Virtual hosting](use-cases/virtual-hosting.md) works a two site server through in full, and [Configuration](configuration.md#virtual-hosts) is the reference for every key named above.

## Credentials a script cannot spell

A connection and a mail server are both named, never described. Both providers are built per site from that site's own configuration and hold nothing else (`cmd/phpscript/server/vhost.go:158`, `:163`), so a name another site configured does not resolve at all, with no check to refuse it.

| Surface                           | What a script gets                             | What it cannot get                                                                          |
|-----------------------------------|------------------------------------------------|---------------------------------------------------------------------------------------------|
| `new Database($name)`             | A client, or a failure naming `$name`          | The DSN. `get_object_vars()` is empty and `json_encode()` carries no credential             |
| `Database::connections()`         | The names this site can open, sorted           | Any DSN behind a name                                                                       |
| `Database::register($name, $dsn)` | A connection added to this site's own provider | Reach into another site's provider                                                          |
| `new Mail($name)`, `mail()`       | A delivery, or a failure naming `$name`        | A host, a username or a password. There is no listing call and the object has no properties |
| `getenv()`                        | The variables the site's `env` declared        | Anything named `PLATFORM_*`, and anything the operator's process carries                    |

`Database::register` is the one place a connection string enters from PHP, and it is a script supplying one it already holds, with nothing read back; the credential goes into the provider that site already resolves through (`stdlib/database/register.go:59`). `Mail` has no counterpart: naming what exists is itself a disclosure, and the constructor refuses settings, with no coercion. [Credentials stay with the host](configuration.md#credentials-stay-with-the-host) is the full account.

`runner.InfrastructurePrefix` is what keeps connection strings out of `getenv()` (`runner/runtime.go:407`), and it applies to a variable the site declared itself: a site listing `PLATFORM_DB_SHOP` gets the connection and reads nothing back for the name.

## Service-managed and vhost-managed configuration

Two keys belong to the operator, and a site setting either fails startup, with nothing dropped quietly (`config/virtualhost.go:17`): `server`, because the listen address is the operator's, and `virtualhost`, because a site holds no sites. Everything else in the model is the site's, read from `root/phpscript.yml` over whatever `-f` produced.

How a key combines decides what a site inherits:

| Key                                          | Combining             | A site that declares nothing                      |
|----------------------------------------------|-----------------------|---------------------------------------------------|
| `env`                                        | replaced whole        | Inherits every connection the operator configured |
| `mail`                                       | replaced whole        | Inherits every server the operator configured     |
| `runner`, `telemetry`, `routes`, `flatstack` | merged field by field | Inherits the operator's value for each field      |

`env` and `mail` replace because a merge was worse. The mail block was once a single unnamed server, and a site setting only `host` and `from` kept the operator's `username` and `password` and went on authenticating as the operator. That is why both are collections keyed by name.

A merged block has the mirror-image property: a site's `runner.writable_paths` replaces the operator's list for that site, so the operator cannot confine a site that declares one. Confining a tenant's writes is the filesystem's job and not the configuration's.

The startup checks run over the whole list before the server listens, and `phpscript -t` runs them without starting anything. [Startup checks](configuration.md#startup-checks) lists them.

## What a virtual host can do

A process started without `--stdlib` installs every binding package `stdlib/imports.go` imports (`stdlib/stdlib.go:17`), so every site gets the whole standard library, including the parts that reach outside the request:

| Capability                                 | Binding          | What it reaches                                                                  |
|--------------------------------------------|------------------|----------------------------------------------------------------------------------|
| `exec`, `system`, `passthru`, `shell_exec` | `stdlib/pexec`   | A process at the privilege of the user running the server                        |
| `new HTTP\Client`                          | `stdlib/http`    | Any address the host can route to, with headers the script sets                  |
| `new HTTP\Server($addr, ...)`              | `stdlib/http`    | A listening socket on an address the script names                                |
| `new Session\Storage\Disk($path)`          | `stdlib/session` | A directory the script names, created outside the root                           |
| `imagecreatefrompng`, `imagepng`           | `stdlib/gd`      | A host path, until [#132](https://github.com/titpetric/phpscript/pull/132) lands |

`--stdlib` leaves the first row out:

```sh
phpscript --stdlib=secure server
```

The flag names the binding areas the process installs, and `secure` is every binding confined to the runtime's sandbox. An area it omits is not registered, so its names are undefined and nothing refuses them, through a direct call, a variable function, `call_user_func`, a callable and both engines alike: they all resolve through the one runtime function table. `stdlib.Mount(rt, profile, bindings...)` is the same selection for a host embedding the package.

It is one value for the process. An operator decides what the runtime they started may reach, so there is no per-site key: a capability granted in the operator's file for one tenant is a capability the tenant's neighbours can read, and `virtualhost` is already refused in a site's own `phpscript.yml` so a compromised tree cannot grant itself anything.

## The filesystem boundary

Every path a script names resolves against the root the bindings were bound to, and no spelling escapes it. A path written from `/` names the root itself, the spelling `getcwd()`, `realpath()`, `__DIR__` and `__FILE__` all answer with, and a `..` is collapsed against the root with no way out of it (`stdlib/files/files.go:80`, `runner/runner.go:1139`). There is no spelling for a host path:

```php
file_get_contents("/etc/hostname");   // false
file_exists("/etc/hostname");         // false
scandir("/");                         // the site's own root
```

Writes are additionally held to `writable_paths`, and a refused write throws where an operating system refusal returns `false`. Two properties of that list are not what its name suggests:

- **An empty list allows every write inside the root.** It is not deny-by-default. A site that names no `writable_paths` may write anywhere in its own tree.
- **An absolute entry is accepted and then matches nothing.** `WritableRoots` keeps it verbatim, but the path it is compared against has already been joined onto the root, so the entry can never match and the write throws exactly as it would with no entry at all. There is no way to grant a write outside the root.

[Writable paths](configuration.md#writable-paths) is the reference for the list and the functions held to it.

### Where the root is left behind

Four places reach the host filesystem directly. Two are decisions and two are defects.

| Place                                    | Where                               | Status                                                                                                                    |
|------------------------------------------|-------------------------------------|---------------------------------------------------------------------------------------------------------------------------|
| `exec` and the three functions beside it | `stdlib/pexec/exec.go:40`           | By design. A command is a process, and the root only decides where it starts                                              |
| An uploaded part                         | `runner/request.go:739`             | By design. Written to the system temporary directory, readable back only through the request's own registry               |
| `new Session\Storage\Disk($path)`        | `stdlib/session/storage_disk.go:20` | A defect. The constructor calls `os.MkdirAll` on the path the script named, outside the root and outside `writable_paths` |
| gd image reads and writes                | `stdlib/gd/gd.go:139`               | A defect, fixed in [#132](https://github.com/titpetric/phpscript/pull/132)                                                |

The two defects have one shape: a binding resolving an absolute path to itself while every other binding resolves it to the root. Both are visible from PHP as a disagreement between bindings, where `file_exists()` answers `false` for a path the host can see a file at.

A deployment that cannot wait for either fix confines the process: a read-only bind mount over the tree, and a user whose only write permission is the volume.

### Writable storage

Issue 139 names `/data` as a site's writable filesystem, mapped onto a volume the operator mounted. **That mapping does not exist.** There is no `/data`, no key naming one, and no indirection between the path a script writes and the host path it lands on: `root.resolve` joins onto the real root directory, and a site's own `writable_paths` is the only thing between a script and its tree.

What works today, and what a site is written against until the mapping exists:

- A sqlite database at an absolute DSN, `sqlite:///srv/data/shop/shop.db`. `resolveSQLiteDSN` leaves an absolute path and a `file:` URI alone and joins only a relative one onto the root (`stdlib/database/dsn.go:15`), so the file sits outside the tree and survives a redeploy.
- Uploads in a directory the operator bind-mounts over a path inside the root, named in `writable_paths`. The site's PHP writes a relative path and never learns it is a mount.

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

The two caches are one pair per site, created with the site's file handler and shared between its routed endpoints and its document root so that one precompile pass covers both (`cmd/phpscript/server/precompile.go:13`). No cache is shared across sites.

`SharedMemory` is the exception, and not in the direction its name suggests. No host in this tree binds a store into a runtime context, so `new SharedMemory` hands every caller a fresh empty store that lives as long as the object (`stdlib/core/shared_memory.go:38`). Nothing is shared between sites because nothing is shared at all, which is also why the `SharedMemory` section of `phpinfo()` never prints under `phpscript server`. Collapsing the caches behind a `runtime.Cache` and defining a per-site store are issue 139's third and fourth items.

Isolation stops at the process. One site's `exec`, one site's `HTTP\Server` and one site's unbounded memory are the same operating system process as every other site's, and a reload rebuilds every site, so updating one tenant re-runs every other tenant's `@startup` jobs. Two tenants that must not share a fault domain are two processes.

## Not implemented yet

Each of these is a design in issue 139 with nothing behind it in the code. A document that read as a description of today would be wrong about all five.

| Intended                                                           | Issue 139 item | State                                                          |
|--------------------------------------------------------------------|----------------|----------------------------------------------------------------|
| A `runtime.Cache` collapsing the per-site caches behind one setter | 4              | The caches are per site already, each set on its own           |
| A `secrets:` block, and `get_secret()` in PHP                      | 5              | No key, no binding                                             |
| `phpscript.key` in a vhost, encrypting that block at rest          | 1, 2           | No key file is read anywhere                                   |
| `config/phpscript.yml` as a second location for a site's file      | 3              | Only `root/phpscript.yml` is read (`config/virtualhost.go:12`) |
| A writable volume presented to a site as `/data`                   | 1              | No mapping. See [Writable storage](#writable-storage)          |

Two shapes the secrets work has to keep when it lands, because the rest of this page rests on them. A secret decrypted for a request is a local that is gone when the call returns, the way a mail credential already is (`stdlib/mail/provider.go:86`). And `get_secret()` names a secret the way `new Database` names a connection: a site's secrets are its own, there is no listing call, and a name another site declared does not resolve.

## The class name on a refused write

A write refused by `writable_paths` throws, and `get_class($e)` on it reports `errorString` - a Go internal type name reaching a script. It is catchable and the message is correct, so a script catching `Exception` is unaffected, but one that discriminates on the class name is matching against a Go identifier. Every error a binding raises as a Go value, without constructing a PHP class, answers the same way. It follows from [one type for all throwables](design.md#exceptions-without-a-hierarchy) and is not specific to this path.

## Where to go next

- [Configuration](configuration.md) - every key, and which of them belong to the operator
- [Virtual hosting](use-cases/virtual-hosting.md) - a two site server, and what one domain cannot reach on the other
- [Telemetry](telemetry.md#access-control) - what a trace records, and who can read the front end
- [Design decisions](design.md) - what phpscript will not implement
- [Known divergences from PHP](README.md#known-divergences-from-php) - the sandbox entries a script sees
