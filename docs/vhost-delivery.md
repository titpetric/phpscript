# Virtual host delivery

How a virtual host gets onto a deploy host, and what phpscript would have to do to fetch one from a URL instead of being pointed at a directory somebody else put there. This is a design, not a feature: nothing described here is implemented, and the sections are the order an implementation would land in.

The question comes from [issue 116](https://github.com/titpetric/phpscript/issues/116). A site today is a whole source tree on a real filesystem mount - apis, modules, themes, migrations, a `phpscript.yml` - and the operator is the one who put it there. The proposal is to name a zip or a tarball instead: a GitHub or Forgejo source archive, a release asset, an S3 presigned URL, mirrored locally and unpacked into a working tree.

Two constraints shape the whole answer. The configuration data model is frozen, so no key is added, renamed, retyped or removed under `config/`. And the `fs.FS` the runtime takes is a narrower seam than it looks: it carries source loading and little else, so an archive cannot be the root on its own.

## What `root` is today

`VirtualHost.Root` is one string (`config/virtualhost.go:39`) read by five consumers, and only one of them wants an `fs.FS`:

| Consumer                                                  | Where                               | Wants                                                      |
|-----------------------------------------------------------|-------------------------------------|------------------------------------------------------------|
| Startup validation                                        | `config/virtualhost.go:157`, `:166` | A real directory, through `os.Stat`                        |
| Source loading, includes, precompile, annotation scanning | `cmd/phpscript/server/vhost.go:180` | `os.DirFS(host.Root)`, assigned to `runner.Options.RootFS` |
| Relative sqlite DSNs                                      | `cmd/phpscript/server/vhost.go:158` | A real directory, joined onto the DSN path                 |
| Every filesystem binding, gd and `exec`                   | `cmd/phpscript/server/run.go:299`   | A real directory string, `h.rootDir`                       |
| `$_SERVER["SCRIPT_FILENAME"]`                             | `cmd/phpscript/server/run.go:253`   | A real path a script can print                             |

A `root` is therefore a directory that happens to also be exposed as an `fs.FS`, not an `fs.FS` that happens to live on disk. That order decides everything below.

## Where the `fs.FS` seam admits a non-local root

`runner.Options.RootFS` is an `fs.FS` tagged `yaml:"-"` (`runner/options.go:14`), so it is outside the frozen model and an implementation may hold anything there. The seam is real and already exercised: the HTTP handler takes an `fs.FS` rather than a path (`cmd/phpscript/server/run.go:107`), and the server's own tests drive a whole application root out of `fstest.MapFS`.

What goes through it: entrypoint loading and `include`/`require` (`runner/runner.go:110` is the single read point), `spl_autoload_register` lookups, the precompile walk, `@route`, `@startup` and `@schedule` scanning, static file serving, SQL migration files, and the read half of the filesystem bindings.

What does not:

| Bypass                                                                       | Where                                | Reason                                                                  |
|------------------------------------------------------------------------------|--------------------------------------|-------------------------------------------------------------------------|
| Every write - `fopen` past `r`, `mkdir`, `unlink`, `rename`, `copy`, `chmod` | `stdlib/files/write.go`, `stream.go` | `fs.FS` is read-only; the package says so at `stdlib/files/files.go:7`  |
| The sqlite file                                                              | `stdlib/database/dsn.go:15`          | `modernc.org/sqlite` is handed a filename and has no VFS hook           |
| gd image reads and writes                                                    | `stdlib/gd/gd.go:139`                | `os.Open` and `os.Create` on a host path, with no `fs.FS` path at all   |
| `exec`, `shell_exec`, `passthru`                                             | `stdlib/pexec/exec.go:33`            | A command is a process, not a path; documented as the deliberate escape |
| Session disk storage                                                         | `stdlib/session/storage_disk.go`     | The script names the directory and it is not rooted at the application  |
| Uploaded parts                                                               | `runner/request.go:739`              | Written to `os.CreateTemp("")` outside any root by design               |

So a zip-backed `fs.FS` with no directory behind it is not viable, and the failure is not graceful. `stdlib.RegisterFS` is called only when `h.rootDir != ""` (`cmd/phpscript/server/run.go:299`); with an empty `rootDir` the bindings keep the root `files.Register` installed, which is `"."`, the server's own working directory. A site served from an archive with no mirror would write its uploads and open its sqlite file in the operator's working directory, shared with every other site in the process. That is a tenancy boundary failure rather than a missing feature.

**A URL root resolves to a real directory. The archive is a transport, never a filesystem.**

## Expressing a URL root without a new field

`root` is already a `string`. A URL is a string, so carrying one adds no key, renames nothing and retypes nothing:

```yaml
virtualhost:
  - domain: shop.example.com
    root: https://github.com/example/shop/archive/refs/tags/v1.4.0.zip
```

Nothing has to disambiguate it. A value carrying an `https://`, `http://` or `s3://` scheme is a URL; everything else is the path it is today. No existing configuration names a site that way, so no file changes meaning, which is what the freeze protects.

The entry is then rewritten to the directory it resolved to, before anything stats it. `VirtualHost.Normalize` is the precedent: it already rewrites `Domain` in place so the rest of the server reads the names a `Host` header is compared against rather than the spelling the operator used (`config/virtualhost.go:51`). A resolve pass does the same for `Root`, and after it every one of the five consumers above sees a local path and needs no change at all.

The hook is `reloadConfig` (`cmd/phpscript/server/run.go:429`), the one function that answers what configuration a generation runs under. Both `manager.Check` and `manager.Setup` call it, so a resolve pass there is seen by `-t` and by the reload alike, and no other command grows a network dependency: `phpscript run`, `fmt`, `lint` and `test` never reach it.

`-t` resolves offline. It reports a URL with no mirror as its own error rather than downloading one, because `-t` is the check an operator runs before a deploy and a check that pulls two hundred megabytes is not one. The documented promise is that `-t` passing predicts a reload being applied, and it still does for everything `-t` can see.

## The mirror

A fetch unpacks into `vendor/<site>/<key>/`, resolved against the process working directory, which is where `root` already resolves from and what `-w` moves before the configuration is read (`internal/flags/flags.go:239`). `vendor` is the first line of `.gitignore`, so the mirror is ignored already, and the issue's instinct to put sites under `vendor/` is the right one.

`<site>` is `VirtualHost.Name()`, the first domain the entry lists, which is what the site is reported as everywhere else. `<key>` is a digest over the URL and the identity the fetch resolved it to.

**A key names an immutable directory, and that is the whole of the atomicity story.** The unpack target is `vendor/<site>/.staging-<random>/`, on the same filesystem, renamed into place with one `os.Rename` when the last entry is written. A directory under `<key>` is either absent or complete. Nothing is ever written into a directory a generation is serving, there is no symlink to flip and therefore no window where a reader sees half a swap, and a fetch that dies leaves a staging directory the next startup removes.

Pruning happens at startup and never at reload. The old generation's runtimes hold open handles and an `os.DirFS` pointed at the previous key for as long as they are draining, and a reload that deleted the directory would break the next `include` they resolve. Startup keeps the key every entry currently resolves to and removes the rest.

## How an update happens

There is no new trigger and no polling loop. The reload that exists already does the whole job ([`phpscript server`](cli/server.md#reloading)): the socket is held, the virtual host list is read again, every site is rebuilt from the file as it is on disk now, the caches go with the old generation, and the `-t` check runs before the swap - a check that fails logs `reload refused, still serving` and the old generation keeps answering. An update is a fetch followed by `phpscript -s reload`, driven by whatever the operator already drives deploys with.

Two properties of that mechanism are worth stating because a site author will meet them:

- **A reload rebuilds every site, not one.** Updating one tenant re-runs every other tenant's `@startup` jobs and drops every other tenant's precompiled tree. Per-site reload does not exist. A host with many tenants and frequent updates pays for all of them on each one, and the answer is to batch deploys rather than to add a second reload path.
- **`@startup` runs again on every reload.** A migration job has always had to be idempotent for that reason and the shipped examples are. A fetch does not change the rule, it only makes it apply more often.

A mutable URL, a branch archive, is refetched on each reload and may resolve to a new key, which is how "redeploy from `main`" works. An immutable URL - a tag, a commit sha, a release asset, a presigned URL naming a version - resolves to a key already on disk and the reload makes no request at all.

## Fetching, and the rate limit

phpscript makes a request in exactly two places: startup, and a reload that found a URL whose key is not on disk. No background goroutine, no interval, no `@schedule` job. That is the rate-limit answer, and it is a better one than a limiter would be: a host that deploys four times a day makes four requests, and an unauthenticated hourly limit stops being a number anybody has to think about.

For a mutable URL the request is conditional. The fetch records the `ETag` and `Last-Modified` it was answered with beside the mirror and sends `If-None-Match` and `If-Modified-Since` on the next one, so a reload that finds nothing new costs a 304 and no unpack. GitHub and Forgejo both answer an archive request with validators.

Credentials go in the URL. A presigned S3 URL carries its own, and a private repository takes a token as basic auth, `https://x-access-token:TOKEN@github.com/...`, which Go's client sends out of the userinfo without any help. That keeps a credential attached to the one URL it authenticates and needs no key. The alternative is a `PLATFORM_`-prefixed entry in the existing `env` list, which the rule that hides connection strings from `getenv()` already keeps away from scripts; it is available when a token has to be shared across entries, and it is not the recommendation, because a token that authenticates one archive should not be readable while fetching another.

Integrity is a `#sha256=` fragment on the URL, verified when present and absent by default. The default is off because an auto-generated source archive is not a stored object: GitHub and Forgejo build one per request out of the git tree and neither guarantees the bytes, so a digest over `/archive/refs/tags/v1.4.0.zip` is a check that starts failing on somebody else's upgrade. A release asset and an S3 object are stored bytes and a digest over one means something. Say which is which, rather than offering a check that works for half the URLs anybody writes.

Neither `archive/zip`, `archive/tar` nor `compress/gzip` is imported anywhere in the tree today. Unpacking has to reject an entry whose cleaned path escapes the destination, reject a symlink outright, and cap the total unpacked size, because the archive is the one input to this design that arrives over a network.

## Writes, and what may not live in the tree

A mirror directory is replaced wholesale by the next update. Anything a site wrote inside it is gone, which makes `data/` the sharpest edge in the proposal: a site whose sqlite database sits at `data/app.db` loses it on its first deploy.

The configuration already expresses the fix, and without a key. `resolveSQLiteDSN` leaves an absolute path and a `file:`-prefixed DSN untouched and joins only a relative one onto the root (`stdlib/database/dsn.go:15`), so a URL-rooted site names its database absolutely in the `env` list it writes anyway:

```yaml
env:
  - "PLATFORM_DB_SHOP=sqlite:///srv/data/shop/shop.db"
```

That works because Go opens the file directly and the mirror is nowhere in the path. **A URL-rooted site keeps nothing durable inside its tree**, and an absolute DSN is how it says so.

Files are harder, and the honest answer is that the current model cannot express them. A PHP write cannot leave the root: `root.resolve` joins every path onto `r.dir` and `clampFSPath` collapses a `..` rather than letting it climb (`stdlib/files/files.go:80`, `runner/runner.go:1139`), so `file_put_contents("/srv/data/x")` writes to `<root>/srv/data/x`. An absolute `writable_paths` entry outside the root is kept verbatim by `WritableRoots` and can then never match a resolved write path, which makes it dead configuration. A site accepting uploads has three options and the first is the recommendation:

| Option                                                                           | Needs           | Verdict                                                                            |
|----------------------------------------------------------------------------------|-----------------|------------------------------------------------------------------------------------|
| The operator bind-mounts a writable directory over `public/upload` in the mirror | No code, no key | Recommended. It works today, it survives the swap, and the site's PHP is unchanged |
| Uploads live in the mirror and are lost on update                                | Nothing         | Wrong, and silently so                                                             |
| A key mapping a host directory into the root                                     | A new field     | Frozen. It is the operator's call and it is not made here                          |

Confining the mirror itself is also the operator's job. `writable_paths` empty allows every write, and the overlay replaces a list wholesale, so a site declaring its own `runner.writable_paths` replaces whatever the operator set: the operator cannot make a URL-rooted site leave its own tree alone. Marking the mirror read-only at the filesystem level does enforce it, costs no code, and turns a write into the `false` return PHP gives for a write the operating system refused.

## No `phpscript.lock`

The issue asks whether this wants a lock file and a vendoring system. It does not, and the reason is structural rather than a matter of scope.

A lock file exists because a manifest names a range. `composer.json` says `^1.4`, several versions satisfy it, resolution picks one, and the lock records which so that the next host picks the same. `root` names one URL and no range. There is nothing to resolve, so there is nothing to lock: the operator's configuration file is already the manifest and already the lock, and a `phpscript.lock` beside it would record the same URL a second time and then drift from it.

The other thing a lock buys is reproducibility, and that is bought here by naming an immutable URL. A tag, a commit sha or a release asset gives two hosts the same tree from the same configuration. A branch archive does not, and that is the point of writing one: it means "whatever is on `main` at the next reload". The choice stays visible in the URL instead of becoming a mode a second file puts the deploy into.

The mirror does keep state - the key, the `ETag`, the `Last-Modified` - and that state lives under the gitignored `vendor/` as a cache, not beside the configuration as a document. It is derived, it is per host, and losing it costs one refetch.

## Composer is not the delivery channel

Composer can install a site. `composer require` against a VCS URL puts a tree at `vendor/<vendor>/<name>/`, which is a perfectly good `root`, and phpscript already supports the other half of composer: `runner.include: vendor/autoload.php` is a documented setting and classes resolve through `spl_autoload_register`.

Use it for a site's PHP dependencies, which is what it is for. Do not use it for site delivery, for three reasons:

- **It installs in place.** `composer update` rewrites `vendor/` while the server is serving out of it. That is the half-unpacked tree this design spends its atomicity on avoiding, and no amount of care at the phpscript end fixes it.
- **It runs the package's scripts.** A `composer.json` `post-install-cmd` executes at operator privilege on the deploy host, off the request path and outside anything `writable_paths` or the `fs.FS` root constrains. An archive that is only unpacked cannot do that.
- **It needs composer, php and a `composer.json` per site on the deploy host.** A fetch needs none of the three. The deploy host exists to run one Go binary.

## Tests travel with the code and do not run on the deploy host

The issue asks whether an update should be gated on the tests a site bundles. It should, and the gate belongs where the archive is built. **The archive is the output of a passing CI run, not the input to one.** A site publishes a tag, its own pipeline runs `phpscript test ./...` over the tree, and the archive that URL names is one that already passed. The deploy host's gate is the `-t` check and the reload refusal, which are about the configuration rather than the code.

Running a tenant's suite from the server is not a smaller version of that. It is four separate problems:

- The suite writes. Fixtures lay down the schema and the rows they read, `test.hooks.setup` runs a script before any of them, and the mirror has to stay immutable for the swap to mean anything.
- Those hooks are arbitrary PHP executing at operator privilege, off the request path, with no `Host` header having selected a site. That is the composer-scripts objection arriving by another door.
- `test.parallel`, `cache` and `skip_php` are read only from the file `-f` names or the one above the working directory, and a `phpscript.yml` discovered below the invocation root that sets one fails the run naming the key. A tenant tree is below the invocation root by construction.
- A `.phpt` is settled against real `php`, and the matrix column that makes a fixture mean anything needs the `php` binary. A deploy host has no reason to carry it, and a suite run with `--skip-php` is not the run that gated anything.

## What an implementation would touch

| Package                      | Change                                                                                                                                          |
|------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------|
| `config`                     | A method reporting whether `Root` is a URL, and `ValidateVirtualHosts` skipping `statDir` for an unresolved one. No key added                   |
| `cmd/phpscript/server`       | A resolve pass in `reloadConfig` rewriting `Root` to the mirror path; offline in `Check`, fetching in `Setup`                                   |
| A new `vhost/mirror` package | Conditional fetch, safe unpack, key derivation, staged rename, startup prune                                                                    |
| `docs/configuration.md`      | The URL form of `root`, the absolute-DSN rule, the mirror layout                                                                                |
| `tests/fixtures/`            | Nothing. This is a delivery mechanism and runs no PHP differently                                                                               |
| Go tests                     | Unpack rejecting a path that escapes the destination and a symlink; conditional-request reuse; the prune keeping a key a generation still holds |

## Decisions this needs

Three calls are the operator's, each with a recommendation rather than an option list:

| Question                                      | Recommendation                                                                                                                                                 |
|-----------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Does the server fetch, or a separate command? | The server, in `reloadConfig`. A separate `phpscript vendor` would need somewhere to write the URL down, and the only place that needs no key is `root` itself |
| Durable files for a URL-rooted site           | A bind-mount, documented as the way. Revisit a key for it when a site needs uploads and cannot be given a mount                                                |
| Confining the mirror                          | Read-only at the filesystem level. phpscript cannot enforce it, because a site's own `writable_paths` replaces the operator's                                  |

## Where to go next

- [Configuration](configuration.md#virtual-hosts) - every key a virtual host entry carries today
- [Virtual hosting](use-cases/virtual-hosting.md) - a two site server worked through in full
- [`phpscript server`](cli/server.md#reloading) - what a reload rebuilds and what it refuses
- [Design decisions](design.md) - what phpscript will not implement
