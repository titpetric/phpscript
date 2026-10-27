# Serving static files

`phpscript server` is a web server first and an interpreter second. The server publishes everything below the document root: it executes a `.php` file there, and serves anything else as the bytes on disk, with the content type its extension names, its modification time, and the range and conditional request handling that go with it.

A site does not have to be an application for the server to publish it. A built front end, a directory of hand written pages, a documentation tree or a folder of images are all sites this server answers for, with no `index.php` and no route annotations anywhere in them.

```text
site/
└── public/
    ├── index.html
    ├── 404.html
    ├── style.css
    └── docs/
        └── index.html
```

```bash
phpscript server ./site
```

## Index pages

The server answers a request that names a directory with that directory's index page. It tries two names, in order:

| File         | Answers with                      |
|--------------|-----------------------------------|
| `index.php`  | The page the interpreter renders  |
| `index.html` | The file, served as bytes on disk |

`/` is `public/index.php` or `public/index.html`, `/docs/` is `public/docs/index.php` or `public/docs/index.html`, and so on down the tree. `index.php` wins when a directory has both: the dynamic page is the one the site wrote for the server to publish, and the `index.html` beside it remains a placeholder nobody has removed.

The server redirects a directory named without its trailing slash to the same path with one, before serving the index page. The links in that page resolve against its serving URL and `/docs` has to become `/docs/` or every relative link in it points a directory too high.

A directory with neither index page is a 404, answered by the site's own [error page](error-handling.md#error-pages) when it has one. It does not list the files below it, unless the site enables a listing.

## Directory listings

`autoindex` answers a directory that has no index page with a listing of what is in it, the way nginx's `autoindex on;` does:

```yaml
# phpscript.yml
autoindex: true
```

`autoindex` is off by default. Publishing every file below the document root is a decision a site makes, and not one it should arrive at by leaving an `index.html` out of a directory.

The server generates the listing, and no file needs writing or regenerating when the directory changes. It holds:

- every entry, linked relative to the directory's request path, with directories first
- the size of each file and when it last changed
- images as thumbnails above the table as well as rows in it and a folder of screenshots is browsable and still says how big each one is
- a link to the directory above, except at the document root, whose parent is not the site's to link to

The listing leaves out names beginning with a dot: a listing is what a site chose to publish, and a dotfile below the document root is at best noise and at worst an editor's backup of a script. The page carries the little styling it needs inline and fetches nothing from anywhere else and a listing tells a third party nothing about the browsing and renders the same on a machine with no route out.

An index page still wins where there is one. `autoindex` only decides what a directory without one answers.

## Error pages

A static site puts up its own error pages the same way an application does, by writing a file named after the status in the document root. `404.html` answers for a 404 and `error.html` answers for every status the site does not name one for and a site with no PHP in it needs no PHP to answer a dead link. See [Error pages](error-handling.md#error-pages).

## What is not served

The document root is the boundary. A file above it, `phpscript.yml` and the route and template directories among them, has no URL and is not reachable by one; a path with `..` in it does not escape.

A `.php` file in a directory the configuration lists in `runner.writable_paths` is content and not code: the server serves it as bytes and never executes it. That includes an `index.php` and a `404.php` and an upload directory below the document root cannot turn an upload form into a way to run code.

## Virtual hosting

Each site answers for its own document root, its own index pages, its own error pages and its own `autoindex`, all read from the `phpscript.yml` in its own application root. One domain can publish a file drop while another serves an application. See [Virtual hosting](virtual-hosting.md).
