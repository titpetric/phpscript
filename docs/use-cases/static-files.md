# Serving static files

`phpscript server` is a web server first and an interpreter second. The server publishes everything below the document root: it executes a `.php` file there. Anything else goes out as the bytes on disk, with the content type its extension names, its modification time, and the range and conditional request handling it needs.

A site need not be an application for the server to publish. A built front end, a directory of hand written pages, a documentation tree or a folder of images all make sites this server answers for. A site needs no `index.php` or route annotations.

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

`/` is `public/index.php` or `public/index.html`, `/docs/` is `public/docs/index.php` or `public/docs/index.html` and so on down the tree. `index.php` wins when a directory has both: the site wrote the dynamic page for the server to publish, and the `index.html` alongside it remains a placeholder nobody removed.

The server redirects a directory named without a trailing slash to the same path with one, before serving the index page. The links in that page resolve against the serving URL, and `/docs` must become `/docs/` or every relative link in it points a directory too high.

A directory holding neither index page is a 404, answered by the site's [error page](error-handling.md#error-pages) when the site defines one, and lists nothing unless the site enables a listing.

## Directory listings

`autoindex` answers a directory lacking an index page by listing its contents, the way nginx's `autoindex on;` does:

```yaml
# phpscript.yml
autoindex: true
```

`autoindex` is off by default. Publishing every file below the document root is a decision a site makes, not one to reach by leaving an `index.html` out of a directory.

The server generates the listing, with no file to write or regenerate when the directory changes. It holds:

- every entry, linked relative to the directory's request path, with directories first
- the size of each file and when it last changed
- images as thumbnails above the table and as rows inside the table, keeping a folder of screenshots browsable and each file's size visible
- a link to the directory above, except at the document root, whose parent is not the site's to link to

The listing leaves out names beginning with a dot: a listing shows the files the site chose to publish. A dotfile below the document root is noise at best, an editor's backup of a script at worst. The page holds its little styling inline, fetches nothing from elsewhere, tells a third party nothing about the browsing, and renders the same offline.

An index page still wins where present. `autoindex` only decides the response for a directory without an index page.

## Error pages

A static site puts up error pages the same way an application does: by writing a file named after the status in the document root. `404.html` answers for a 404, and `error.html` answers for every remaining status. A site without PHP needs no PHP to answer a dead link. See [Error pages](error-handling.md#error-pages).

## What is not served

The document root is the boundary. A file above it has no URL, and no route reaches one. Files such as `phpscript.yml`, plus the route and template directories, are above the document root. A path holding `..` does not escape.

A `.php` file in a directory the configuration lists in `runner.writable_paths` is content, not code: the server serves it as bytes, without executing it. Examples include an `index.php` or a `404.php`. An upload directory below the document root cannot turn an upload form into a way to run code.

## Virtual hosting

Each site answers for a document root, index pages, error pages and `autoindex`, all read from the `phpscript.yml` in its application root. One domain can publish a file drop while another serves an application. See [Virtual hosting](virtual-hosting.md).
