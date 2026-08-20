# Virtual hosting

One `phpscript server` can answer for several websites. The operator lists the
sites, each site owns the tree it is served from and the configuration it runs
under, and the `Host` header of a request is the only thing that selects between
them.

This walkthrough builds a server for two sites, `shop.example.com` and
`blog.example.com`, and then shows what one domain cannot reach on the other.
The reference for every key it uses is
[Configuration](../configuration.md#virtual-hosts).

## 1. The layout

The operator's file sits next to the application roots it names. Each root is an
ordinary phpscript application, the same tree
[Building an application](application.md) builds:

```text
sites/
├── server.yml                 # the operator's configuration
├── shop/
│   ├── phpscript.yml          # the site's configuration
│   ├── migrate.php            # @startup: applies the schema
│   ├── routes/
│   │   └── hello.php          # @route GET /hello/{name}
│   └── public/
│       ├── index.php
│       └── db.php
└── blog/
    ├── phpscript.yml
    ├── routes/
    │   └── feed.php           # @route GET /feed
    └── web/
        ├── index.php
        └── db.php
```

`shop` leaves the document root alone, which is the expected case: `public` is
the default and nothing has to say so. `blog` already calls that directory
`web`, which is the reason the setting exists.

## 2. The operator's configuration

`server.yml` names the listen address and the sites. A `root` is resolved
against the working directory, so these are the two directories beside the file:

```yaml
server:
  addr: "127.0.0.1:8080"

telemetry:
  enabled: false

env: []

virtualhost:
  - domain: shop.example.com
    root: shop
  - domain: blog.example.com
    root: blog
```

Two keys here are turned off on purpose. `telemetry.enabled: false` leaves the
platform's own dashboard unmounted: it would sit on the root router, in front of
the host mux, and shadow the dashboard each site mounts for itself. `env: []`
drops the connection the embedded defaults carry, so a site that configures no
database of its own inherits nothing. It is also what the sites' scripts read
with `getenv()`, so leaving it empty is what keeps the operator's own
environment out of them.

## 3. Each site's configuration

Every application root must hold a `phpscript.yml`. It is read on top of
`server.yml`, so it only names what it changes.

`shop/phpscript.yml` takes route scanning and the runner limits as they come,
and names a tracer and a database of its own:

```yaml
routes:
  enabled: true

telemetry:
  enabled: true
  path: "/debug/oida"
  service_name: "shop"

env:
  - "PLATFORM_DB_SHOP=sqlite://shop.db"
```

`blog/phpscript.yml` names the directory it serves, and declares an empty `env`
because it has no database:

```yaml
document_root: web

routes:
  enabled: true

telemetry:
  enabled: true
  path: "/debug/oida"
  service_name: "blog"

env: []
```

Both sites mount a dashboard on `/debug/oida`. That is not a collision: each
mounts it inside its own router, and a router only ever sees the requests for
its own domain.

## 4. The sites

The files are what they would be if each site ran in a server of its own.
`shop/public/index.php`:

```php
<?php echo "shop";
```

`shop/routes/hello.php`, outside the document root, is scanned for annotations:

```php
<?php

// @route GET /hello/{name}

echo "hello " . $_PATH["name"];
```

`shop/migrate.php` runs to completion before the server listens:

```php
<?php

// @startup

$db = new Database("shop");

$db->query("CREATE TABLE IF NOT EXISTS products (id INTEGER PRIMARY KEY, name TEXT)");

echo "shop: schema ready\n";
```

`blog/routes/feed.php` is the blog's only endpoint:

```php
<?php

// @route GET /feed

header("Content-Type: application/json");

echo json_encode(array("posts" => array()));
```

Both sites carry the same `db.php`, `shop/public/db.php` and `blog/web/db.php`,
which is what makes the database boundary visible later:

```php
<?php

$db = new Database("shop");

echo "connected";
```

## 5. Run it

No application root is passed. The entries name their own, and an argument here
is rejected rather than guessed at:

```bash
phpscript -f server.yml server
```

```text
shop: schema ready
[platform] started 5 modules: [phpstartup:shop.example.com phpschedule:shop.example.com phpstartup:blog.example.com phpschedule:blog.example.com phpvhost]
Server listening on 127.0.0.1:8080 http://127.0.0.1:8080
```

`@startup` and `@schedule` run per site, and the modules they run as carry the
domain, which is what lets `server.modules` still address one site's modules.
Only `shop` printed, because only `shop` has a startup job.

A startup job that fails is that site's problem. It is recorded on that site's
recorder as a trace named after the module, `phpstartup:shop.example.com`, and
the server carries on serving every site including the one whose job failed. The
rest of that site's jobs still run.

Requests select a site by `Host`:

```sh
curl -H 'Host: shop.example.com' http://127.0.0.1:8080/
# shop
curl -H 'Host: blog.example.com' http://127.0.0.1:8080/
# blog
curl -H 'Host: shop.example.com' http://127.0.0.1:8080/hello/Ada
# hello Ada
curl -H 'Host: blog.example.com' http://127.0.0.1:8080/feed
# {"posts":[]}
```

A site that answers to more than one name lists the extras as `aliases`. It is
built once and every name reaches the same handler, sharing its routes, its
recorder and its connections:

```yaml
virtualhost:
  - domain: shop.example.com
    aliases: ["www.shop.example.com"]
    root: shop
```

Matching is exact, but a `Host` is compared lowercased, without its port and
without a trailing dot, so all three of these reach `shop`:

```sh
curl -H 'Host: SHOP.Example.com.' http://127.0.0.1:8080/
curl -H 'Host: shop.example.com:8080' http://127.0.0.1:8080/
curl -H 'Host: shop.example.com' http://127.0.0.1:8080/
```

## 6. What one domain cannot reach on the other

Each site gets a router of its own, and that is where the boundaries come from.

**Routes.** `/hello/{name}` is registered inside shop's router, so the blog does
not have it, and `/feed` is not shop's:

```sh
curl -i -H 'Host: blog.example.com' http://127.0.0.1:8080/hello/Ada
# HTTP/1.1 404 Not Found
curl -i -H 'Host: shop.example.com' http://127.0.0.1:8080/feed
# HTTP/1.1 404 Not Found
```

**The document root.** Each site serves the directory beneath its own
application root, `shop/public` and `blog/web`. A path that exists in one tree
is not a path in the other.

**Telemetry.** Each site's `telemetry` block builds its own tracer and mounts
its own front end inside its own router. The same URL is a different dashboard
on each domain:

```sh
curl -s -H 'Host: shop.example.com' http://127.0.0.1:8080/debug/oida/traces
curl -s -H 'Host: blog.example.com' http://127.0.0.1:8080/debug/oida/traces
```

Each page is titled with the `service_name` its own file configured, `shop` on
one domain and `blog` on the other, and lists the requests that arrived on that
domain and no others. The counters and the trace store behind them are separate,
not two views of one buffer.

**Databases.** A site's connections are built from its own `env`, and the
provider holding them sees nothing else. The two `db.php` files are identical
and only shop's works:

```sh
curl -H 'Host: shop.example.com' http://127.0.0.1:8080/db.php
# connected
curl -H 'Host: blog.example.com' http://127.0.0.1:8080/db.php
# eval "__new(\"Database\", \"shop\")": no configuration found for database: [shop] (1:1)
```

The blog's request fails with 500. It is not a permission check: the connection
does not exist in the provider that site's runtime resolves through.

**Unclaimed domains.** There is no default site. A `Host` no entry claims gets
404 and reaches no site's code:

```sh
curl -i -H 'Host: other.example.com' http://127.0.0.1:8080/
# HTTP/1.1 404 Not Found
```

## 7. What the sites may not do

The listen address belongs to the operator, and a site holds no sites of its
own. Adding either key to `shop/phpscript.yml` fails startup rather than being
dropped quietly:

```text
virtualhost "shop.example.com": shop/phpscript.yml: "server" is set by the operator, not by the site
```

The rest of the configuration is the site's. It can turn route scanning off,
raise its own upload limits, choose the flat-stack runtime, or record its traces
to disk, without any of it reaching the other site.

Every entry is loaded and checked before the server listens, so a missing
`phpscript.yml`, a root that is not a directory, a document root that does not
exist, or a domain configured twice fails the server rather than one request.
[Configuration](../configuration.md#startup-checks) lists the checks.

## Where to go next

- [Configuration](../configuration.md#virtual-hosts) - every key, default and check
- [Building an application](application.md) - what goes in one application root
- [Routing](routing.md) - the `@route` annotations each site registers
- [Telemetry](../telemetry.md) - the dashboard each site mounts
- [Database bindings](database.md) - the connections a site's `env` names
