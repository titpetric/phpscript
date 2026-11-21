# Error handling

An error in phpscript differs from an error in PHP. phpscript is a go runtime and error handling at a call site takes this form:

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

The VM handles an error from Go code implicitly. When a Go binding returns an error, the VM throws an `Exception` object. You can handle the exception either in PHP or in Go.

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

The second parameter is an optional `code`, which can mean an HTTP status or a custom error code. To catch errors thrown from Go or PHP code, use a try/catch statement:

```php
try {
	$storage = new Storage;
	$storage->get($_GET['id']);
} catch (Exception $e) {
	echo "An exception occurred:\n\n";
	echo "Code: " . $e->getCode() . "\n";
	echo "Message: " . $e->getMessage() . "\n";
}
```

The class a clause names selects the errors it takes. `catch (Exception $e)` leaves a `TypeError` for a later clause. See [Exceptions](../reference/exceptions/README.md).

A script can create a fallback with `finally`.

```php
try {
	// exception throwing code
} catch (Throwable $e) {
	// exception handling code
} finally {
	// code that always executes
}
```

The caught exception is also an `error` type and PHP code can pass it along to a Go function like `func(err error)`.

## Handing exceptions in Go

An embedding host can register a runtime callback for statement errors.

```go
rt.OnError(func(err error) {
	telemetry.SpanFromContext(rt.Context()).RecordError(err)
})
```

The callback consumes each non-exit statement error and execution continues. Omit `OnError` when PHP code should control propagation, because the callback can prevent an enclosing PHP `try`/`catch` from receiving the error.

The runtime context comes from `rt.Context()` and holds the running span. `SpanFromContext` returns nil outside a trace. The callback needs no guard, because every span method tolerates that.

## Errors over HTTP

`phpscript server` decides the status of a response in one place. A script sets one with `http_response_code()`, with the third argument of `header()`, or with a status line:

```php
http_response_code(404);
header("Cache-Control: no-store", true, 404);
header("HTTP/1.1 404 Not Found");
```

An uncaught exception also sets a status. The exception's code becomes the status when it falls between 400 and 599:

```php
throw new Exception("The article moved", 503);
```

Any other code is the script's own numbering. The runtime treats no other code as an HTTP status. A `new Exception("connection refused")` fails the request with a 500. `exit()` and `die()` are not failures: a script that ends early answers with the status it set, as it does under PHP.

They are also not catchable, which matters more than it sounds. This is the redirect every application writes:

```php
try {
	$order = $orders->place($cart);
	header("Location: /orders/" . $order["id"]);
	exit();
} catch (Exception $e) {
	$errors[] = $e->getMessage();
}
```

The `exit()` ends the request there. A runtime where a catch clause could swallow it would continue into the error branch and render a page with a `Location` header already staged. `finally` does not run either, for the same reason it does not in PHP.

A response that failed holds the status alone. What went wrong is in the server log and on the request trace, both addressed by the request id in the response, and not in a body the client reads back.

### Error pages

A site supplies a page for a status by writing a file named after it in the document root. No configuration turns it on:

```text
public/
├── 404.php
├── 503.php
└── error.php
```

The server tries four names for a status, in this order:

| File         | Answers for                              |
|--------------|------------------------------------------|
| `404.php`    | That status, rendered by the interpreter |
| `404.html`   | That status, served as written on disk   |
| `error.php`  | Any status the two above do not name     |
| `error.html` | The same, for a site with no PHP in it   |

The server prefers the `.php` name at each step. A site adds a `404.php` next to a hand-written `404.html` and starts serving the new one by writing it. Deleting the file stops a site using one.

`error.php` reads the status it answers for out of `$_SERVER["REDIRECT_STATUS"]`. `error.html` cannot name that status, and a site of static files must answer every failure with this page.

The page receives the request that failed, in the `$_SERVER` keys Apache fills in for an `ErrorDocument`:

```php
<?php

// public/404.php
$path = isset($_SERVER["REDIRECT_URL"]) ? $_SERVER["REDIRECT_URL"] : $_SERVER["REQUEST_URI"];

$title = "Not found";
$body  = "<p>" . htmlspecialchars($path) . " is not here.</p>";

include "templates/layout.php";
```

An `include` resolves against the application root. The document root is a different base. A page shares the layout the rest of the site uses. The example reads `REDIRECT_URL` through `isset`, because the keys are absent on a direct request for `/404.php`.

| Key                     | Holds                                                |
|-------------------------|------------------------------------------------------|
| `REDIRECT_STATUS`       | The status the page answers for                      |
| `REDIRECT_URL`          | The path that failed                                 |
| `REDIRECT_QUERY_STRING` | Its query string                                     |
| `REDIRECT_ERROR_NOTES`  | What went wrong, when the request failed on a script |

`$_GET`, `$_COOKIE` and the rest of `$_SERVER` are the failed request's. A page may set its status, and the call's status is only the default. The server logs a page that fails. The request falls back to the plain status, dispatching no page behind it.

Whether the page also gets `$_POST`, `$_FILES` and `php://input` depends on which of the two things went wrong. The rule is not a policy:

- **The request matched nothing.** No file answered, no script ran and nothing read the body. The page gets it whole. This lets `404.php` dispatch a site's own routes: a form posting to a URL no file backs arrives as its target endpoint would see it. That page answers `http_response_code(200)` and includes the route target.
- **A script failed.** It had already read the body by the time it threw, leaving nothing further. `$_POST` and `$_FILES` are empty, and `php://input` reads nothing.

```php
<?php

// public/404.php, dispatching a site's own routes
$path = isset($_SERVER["REDIRECT_URL"]) ? $_SERVER["REDIRECT_URL"] : $_SERVER["REQUEST_URI"];

if ($path === "/article" && $_SERVER["REQUEST_METHOD"] === "POST") {
    http_response_code(200);
    include "routes/article-save.php";  // reads $_POST as usual
    return;
}
```

### Who gets one

An error page is a website's answer. An API on the same server excludes this page. The path cannot distinguish the two: `phpscript server` mounts one catch-all and an unrouted `/api/users/99` and an unrouted `/blog/old-post` arrive the same way. The server reads the request and the script instead, and renders a page only when all three hold:

1. **The request names HTML in its accept header.** A browser navigating sends `text/html` in `Accept` and names `Sec-Fetch-Dest: document`. A `fetch()`, an XHR, curl, an `<img>`, a stylesheet, a `HEAD` request do not. `*/*` matches everything and is not a request for HTML.
2. **The script wrote no body.** A script that echoed something has answered, and its answer stands.
3. **The script set no `Content-Type`.** A script that declares its reply format has declared that it does not answer in HTML.

An endpoint keeps its output by continuing as before:

```php
<?php

// @route GET /api/users/{id}
header("Content-Type: application/json");
http_response_code(404);
echo json_encode(["error" => "no such user"]);
```

There is no path prefix to configure, no list to keep in step with the routes, and an API that moves from `/api` to `/v1` needs no change.

Two things never get a page. A `Host` no virtual host claims gets a bare 404, because no site exists and so no document root to look in. The server also serves a `.php` file in a `writable_paths` directory as bytes, never running it. That covers error pages: a `404.php` arriving by upload cannot execute.

## References

- [Errors](../reference/errors/README.md)
- [Exceptions](../reference/exceptions/README.md)
- [Runner.OnError](https://pkg.go.dev/github.com/titpetric/phpscript@main/runner#Runtime.OnError)
