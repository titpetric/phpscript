# Install and run phpscript

phpscript is one binary: a PHP interpreter, a web server, a formatter, a linter
and a test runner. The same binary runs a script, serves a site and lists the
names it registers. This page is the short path from an empty directory to a
site on port 8080.

## Install

With Go 1.27 or newer:

```sh
go install github.com/titpetric/phpscript@latest
phpscript version
```

The module is `github.com/titpetric/phpscript`. The binary is `phpscript`.

With Docker:

```sh
docker run --rm -p 8080:8080 -v "$PWD:/app" titpetric/phpscript:latest server
```

The image is `titpetric/phpscript:latest`. It holds the binary. The volume is
the site.

## Run a script

```sh
printf '<?php echo "hello\n";' > hello.php
phpscript run hello.php
phpscript hello.php
```

`run` is the default command, so the two lines are the same. `phpscript
--help` prints every command. `phpscript list --bindings` prints the functions,
classes, methods and constants this build registers.

## Put a site

A site is a directory with a `public/` document root. That is the default and
the server does not look for a second name.

```text
my-site/
├── phpscript.yml      # optional: overrides the embedded defaults
├── public/
│   ├── index.php      # the page / answers
│   ├── assets/
│   │   └── app.css
│   └── 404.php        # optional
└── .htpasswd          # optional: read by Auth\Storage\PasswordFile
```

`public/index.php` is executed for `/`. `public/index.html` is served as bytes
when no `index.php` is present. A directory with neither is a 404 and
`autoindex: true` lists it instead. Everything below `public/` is published and
nothing above it is.

```sh
phpscript server ./my-site
curl http://127.0.0.1:8080/
phpscript -t ./my-site
```

`phpscript -t` runs the checks that would stop the server starting. It binds no
socket, opens no connection and leaves nothing behind.

## More than one site

The convention is one directory below `sites/`, each with its own
`phpscript.yml` and document root:

```text
sites/
├── server.yml
├── shop/
│   ├── phpscript.yml
│   └── public/
└── blog/
    ├── phpscript.yml
    └── web/
```

`server.yml` names the operator's listen address and the sites:

```yaml
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

`document_root` is `public` by default, so `blog` sets `document_root: web`.
A site's `phpscript.yml` may not set `server` or `virtualhost`. Those belong to
the operator. [Naming conventions](naming-conventions.md) names the rest of the
tree and [Virtual hosting](virtual-hosting.md) walks through the isolation
between the sites.

## Accounts

`Auth\Storage\PasswordFile` reads an apache-format `username:hash` file from the
application root. With no path it reads `.htpasswd`. Write it with `htpasswd
-B`:

```sh
htpasswd -B -C 10 .htpasswd admin
```

Bcrypt and argon2 lines verify. Plain text and crypt DES lines are refused.
`new Session\Manager` takes the same store and is a working login.

## Verify

```sh
phpscript version
phpscript list --areas
phpscript list --bindings
phpscript -t ./my-site
curl -sS http://127.0.0.1:8080/
```

The last two answer `ok` and the page the site returned. The reference at
<https://phpscript.eu/reference/> is generated from the same `list` output.
