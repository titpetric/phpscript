# Error handling

There's significant differences between an error in phpscript and an
error in PHP. Since phpscript is a go runtime, the statements we're
usually dealing with here are in the following form:

```go
storage, err := NewStorage()
if err != nil {
	return err
}

res1, err := storage.Call(ctx, id)
if err != nil {
	return err
}
// ...
```

PHP simplifies this to the following syntax:

```php
$storage = new Storage;
$res1 = $storage->call($_GET['id']);
```

The VM provides implicit error handling of Go code. If an error is
returned from the Go binding, an `Exception` object is thrown in the VM.
You can handle the exception either in PHP or in Go.

## Handling PHP exceptions

There are a few important statements to handle exceptions in PHP:

- `try`
- `catch`
- `finally`
- `throw`

To create an exception in PHP, you would do:

```php
throw new Exception("Not found", 404);
```

The second parameter is an optional `code`, which can mean a HTTP Status
code, or custom error handling codes for your own errors. To catch
errors thrown from either Go or PHP code, you use a try/catch statement:

```php
try {
	$storage = new Storage;
	$storage->get($_GET['id']);
} catch ($e) {
	echo "An exception occured:\n\n";
	echo "Code: " . $e->getCode();
        echo "Message: " . $e->getMessage();
}
```

Certain fallback situations can be created with `finally`.

```php
try {
	// exception throwing code
} catch ($e) {
	// exception handling code
} finally {
	// code that always executes
}
```

The caught exception is also an `error` type, so it can be passed along
to Go functions like `func(err error)` from PHP code.

## Handing exceptions in Go

An embedding host can register a runtime callback for statement errors.

```go
rt.OnError(func(err error) {
	telemetry.SpanFromContext(rt.Context()).RecordError(err)
})
```

The callback consumes each non-exit statement error and execution continues.
It can therefore prevent an enclosing PHP `try`/`catch` from receiving the
error; omit `OnError` when PHP code should control propagation.

The runtime context is accessible over `rt.Context()`, which is what carries
the running span. `SpanFromContext` returns nil outside a trace and every span
method tolerates that, so the callback needs no guard.

## Errors over HTTP

`phpscript server` decides the status of a response in one place. A script sets
one with `http_response_code()`, with the third argument of `header()`, or with
a status line:

```php
http_response_code(404);
header("Cache-Control: no-store", true, 404);
header("HTTP/1.1 404 Not Found");
```

An uncaught exception sets one too. The code carried by the exception becomes
the status when it is one, that is when it falls between 400 and 599:

```php
throw new Exception("The article moved", 503);
```

Any other code is the script's own numbering rather than an HTTP status, so a
`new Exception("connection refused")` fails the request with a 500. `exit()` and
`die()` are not failures: a script that ends early answers with the status it
set, as it does in PHP.

A response that failed carries the status and nothing else. What went wrong is
in the server log and on the request trace, both addressed by the request id the
response carries, and not in a body the client reads back.

### Error pages

A site puts up its own page for a status by writing a file named after it in the
document root. There is nothing to configure and nothing to enable:

```text
public/
├── 404.php
├── 503.php
└── error.php
```

`404.php` answers for a 404, `404.html` is used when there is no `404.php`, and
`error.php` answers for any status the first two do not name. Deleting the file
is how a site stops using one.

The page is given the request that failed, in the `$_SERVER` keys Apache fills
in for an `ErrorDocument`:

```php
<?php

// public/404.php
$path = isset($_SERVER["REDIRECT_URL"]) ? $_SERVER["REDIRECT_URL"] : $_SERVER["REQUEST_URI"];

$title = "Not found";
$body  = "<p>" . htmlspecialchars($path) . " is not here.</p>";

include "templates/layout.php";
```

An `include` resolves against the application root rather than the document
root, so a page shares the layout the rest of the site uses. The keys are absent
when the file is requested directly, at `/404.php`, which is why the example
reads `REDIRECT_URL` through `isset`.

| Key                     | Holds                                                |
|-------------------------|------------------------------------------------------|
| `REDIRECT_STATUS`       | The status being answered for                        |
| `REDIRECT_URL`          | The path that failed                                 |
| `REDIRECT_QUERY_STRING` | Its query string                                     |
| `REDIRECT_ERROR_NOTES`  | What went wrong, when the request failed on a script |

`$_GET`, `$_COOKIE` and the rest of `$_SERVER` are the failed request's;
`$_POST` and `$_FILES` are empty. A page may answer with a status of its own,
and the one it was called for is only the default. A page that fails itself is
logged, and the request falls back to the plain status rather than dispatching a
second page behind it.

### Who gets one

An error page is a website's answer, and an API on the same server wants no part
of it. Nothing about the path can tell the two apart: `phpscript server` mounts
one catch-all, so an unrouted `/api/users/99` and an unrouted `/blog/old-post`
arrive the same way. The request and the script are asked instead, and a page is
rendered only when all three hold:

1. **The client asked for HTML.** A browser navigating sends `text/html` in
   `Accept` and `Sec-Fetch-Dest: document`. A `fetch()`, an XHR, curl, an
   `<img>` and a stylesheet do not, and neither does a `HEAD` request. `*/*`
   matches everything and is not a request for HTML.
2. **The script wrote no body.** A script that echoed something has answered,
   and its answer stands.
3. **The script set no `Content-Type`.** A script that declared what it answers
   with has declared that it is not answering in HTML.

So an endpoint keeps its own output by doing what it does anyway:

```php
<?php

// @route GET /api/users/{id}
header("Content-Type: application/json");
http_response_code(404);
echo json_encode(["error" => "no such user"]);
```

There is no path prefix to configure, no list to keep in step with the routes,
and an API that moves from `/api` to `/v1` needs no change.

Two things never get a page. A `Host` no virtual host claims gets a bare 404,
because there is no site and so no document root to look in. And a `.php` file
in a `writable_paths` directory is served as bytes rather than run, error pages
included, so a `404.php` that arrived by upload cannot execute.

## References

- [Errors](../reference/errors/README.md)
- [Exceptions](../reference/exceptions/README.md)
- [Runner.OnError](https://pkg.go.dev/github.com/titpetric/phpscript@main/runner#Runtime.OnError)
