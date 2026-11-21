# Virtual hosting

One `phpscript server` can answer for more than one website. The operator lists the sites. Each site owns the tree it serves and the configuration it uses. A request's `Host` header alone selects a site.

This walkthrough builds a server for two sites, `shop.example.com` and `blog.example.com`. It then shows which resources one domain cannot reach on the other. The reference for every key it uses is [Configuration](../configuration.md#virtual-hosts).

## 1. The layout

The operator's file sits next to the application roots it names. Each root is an ordinary phpscript application, the same tree [Building an application](application.md) builds:

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

`shop` leaves the document root alone, which is the expected case: `public` is the default and nothing says so. `blog` already calls that directory `web`. The setting exists for that case.

## 2. The operator's configuration

`server.yml` names the listen address and the sites. A `root` resolves against the working directory. These are the two directories next to the file:

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

Two keys here are off. `telemetry.enabled: false` leaves the platform dashboard unmounted. It would mount on the root router, in front of the host mux, and shadow the dashboard each site mounts. `env: []` drops the connection the embedded defaults hold. A site that configures no database inherits nothing. The sites' scripts also read `env` with `getenv()`. Leaving it empty keeps the operator's environment out of them.

## 3. Each site's configuration

Every application root must hold a `phpscript.yml`. The server reads it on top of `server.yml`. It names only the keys it changes.

`shop/phpscript.yml` takes route scanning and the runner limits as they come. It names a tracer, plus a database:

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

`blog/phpscript.yml` names the directory it serves, and declares an empty `env` because it has no database:

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

Both sites mount a dashboard on `/debug/oida`. That is not a collision: each mounts it inside a private router. A router sees only requests for the domain it serves.

## 4. The sites

The files match a run where each site had a server to itself. `shop/public/index.php`:

```php
<?php echo "shop";
```

`shop/routes/hello.php` sits outside the document root, and the server scans it for annotations:

```php
<?php

// @route GET /hello/{name}

echo "hello " . $_REQUEST["name"];
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

Both sites hold the same `db.php`: `shop/public/db.php` and `blog/web/db.php`. The database boundary is visible later:

```php
<?php

$db = new Database("shop");

echo "connected";
```

## 5. Run it

The operator passes no application root. The entries name the roots, and `phpscript server` rejects an argument here:

```bash
phpscript -f server.yml server
```

```text
shop: schema ready
[platform] started 5 modules: [phpstartup:shop.example.com phpschedule:shop.example.com phpstartup:blog.example.com phpschedule:blog.example.com phpvhost]
Server listening on 127.0.0.1:8080 http://127.0.0.1:8080
```

`@startup` and `@schedule` run per site, and the modules they run as name the domain and `server.modules` still addresses one site's modules. Only `shop` printed, because only `shop` has a startup job.

A startup job that fails is that site's problem. That site's tracer records it as a trace named after the module, `phpstartup:shop.example.com`, and the server goes on serving every site, including the site whose job failed. The rest of that site's jobs still run.

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

A site that answers to more than one name lists them all in `domain`, separated by spaces. The server builds it once. Every name reaches the same handler, which shares routes, recorder and connections:

```yaml
virtualhost:
  - domain: shop.example.com www.shop.example.com
    root: shop
```

The first name is primary: errors and module names report it.

Matching is exact, but the host mux compares a `Host` lowercased, without a port and without a trailing dot. All three reach `shop`:

```sh
curl -H 'Host: SHOP.Example.com.' http://127.0.0.1:8080/
curl -H 'Host: shop.example.com:8080' http://127.0.0.1:8080/
curl -H 'Host: shop.example.com' http://127.0.0.1:8080/
```

## 6. What one domain cannot reach on the other

Each site gets a private router. Every boundary below follows from that.

**Routes.** Shop's router registers `/hello/{name}`. The blog lacks it. `/feed` is not shop's:

```sh
curl -i -H 'Host: blog.example.com' http://127.0.0.1:8080/hello/Ada
# HTTP/1.1 404 Not Found
curl -i -H 'Host: shop.example.com' http://127.0.0.1:8080/feed
# HTTP/1.1 404 Not Found
```

**The document root.** Each site serves the directory beneath the application root: `shop/public` for one, `blog/web` for the other. Any path that exists in one tree is not a path in the other.

**Telemetry.** Each site's `telemetry` block builds a tracer and mounts a front end inside a private router. The same URL is a different dashboard on each domain:

```sh
curl -s -H 'Host: shop.example.com' http://127.0.0.1:8080/debug/oida/traces
curl -s -H 'Host: blog.example.com' http://127.0.0.1:8080/debug/oida/traces
```

Each page takes its title from the `service_name` in the site's file: `shop` on one domain, `blog` on the other. It lists the requests that arrived on that domain, never another's. The counters and the trace store are separate, not two views of one buffer.

**Databases.** The provider builds a site's connections from that site's `env` and sees no other site's connection. The two `db.php` files are identical and only shop's works:

```sh
curl -H 'Host: shop.example.com' http://127.0.0.1:8080/db.php
# connected
curl -H 'Host: blog.example.com' http://127.0.0.1:8080/db.php
# eval "__new(\"Database\", \"shop\")": no configuration found for database: [shop] (1:1)
```

The blog's request fails with 500, not from a permission check: the connection does not exist in the provider that site's runtime resolves through.

**Unclaimed domains.** There is no default site. A `Host` no entry claims gets 404 and reaches no site's code:

```sh
curl -i -H 'Host: other.example.com' http://127.0.0.1:8080/
# HTTP/1.1 404 Not Found
```

## 7. What the sites may not do

The listen address belongs to the operator. A site declares no sites. Adding either key to `shop/phpscript.yml` fails startup, with nothing dropped quietly:

```text
virtualhost "shop.example.com": shop/phpscript.yml: "server" is set by the operator, not by the site
```

The rest of the configuration is the site's. It can turn route scanning off, raise its upload limits, choose the flat-stack runtime, or record its traces to disk, without any of it reaching the other site.

The startup check loads and checks every entry before the server listens. A missing `phpscript.yml`, a root that is not a directory, a document root that does not exist, or a domain configured twice fails the server before it accepts a request. [Configuration](../configuration.md#startup-checks) lists the checks.

## Where to go next

- [Configuration](../configuration.md#virtual-hosts) - every key, default and check
- [Building an application](application.md) - what goes in one application root
- [Routing](routing.md) - the `@route` annotations each site registers
- [Telemetry](../telemetry.md) - the dashboard each site mounts
- [Database bindings](database.md) - the connections a site's `env` names
