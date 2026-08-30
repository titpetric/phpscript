# HTTP client bindings

The standard runtime provides the Go-backed `HTTP\Client` and `HTTP\Request`
classes. They are what a script calls instead of PHP's `curl_*` family, which
phpscript does not implement.

A request is a `net/http` request handed straight over, so a script reads and
writes it the way Go names it. A response is a facade, because its body is read
in full before the script sees it.

## Create a client

`new HTTP\Client` gives a client with a 30 second timeout that follows
redirects:

```php
$client = new HTTP\Client();
```

Settings come in as an associative array. Every key is optional:

```php
$client = new HTTP\Client(array(
    "timeout"          => 10,
    "base_url"         => "https://api.example.com",
    "follow_redirects" => false,
    "user_agent"       => "phpscript/1",
    "headers"          => array("Authorization" => "Bearer " . getenv("API_TOKEN")),
    "insecure"         => false,
));
```

| Option             | Meaning                                                                                      |
|--------------------|----------------------------------------------------------------------------------------------|
| `timeout`          | Seconds, covering the whole request. A string with a unit (`"500ms"`, `"1m30s"`) also parses |
| `base_url`         | Prefix a relative request URL resolves against                                               |
| `follow_redirects` | `false` returns the 3xx and its `Location` instead of the destination                        |
| `user_agent`       | Sent as the `User-Agent` header                                                              |
| `headers`          | Sent with every request the client makes                                                     |
| `insecure`         | Disables certificate verification, for a test server and not for a service                   |

A client always has a timeout. There is no way to ask for none, because a
request with no deadline is the one failure a script cannot recover from.

An unrecognised key throws, so a typo is reported where it is written rather
than at the far end of a request that did not carry what it was meant to:

```php
new HTTP\Client(array("timeuot" => 5));   // throws: unknown option "timeuot"
```

## Send one request

`get()` and `post()` cover the common cases:

```php
$response = $client->get("/users");
$response = $client->post("/users", '{"name":"ada"}');
```

For anything else, build a request and send it. Building one sends nothing:

```php
$request = new HTTP\Request("POST", "/users", '{"name":"ada"}');
$request->header->set("Content-Type", "application/json");

$response = $client->send($request);
```

HTTP methods are written uppercase. A lowercase one is corrected before the
request goes out, because a server treats the method as case-sensitive.

`send()` throws on a transport failure or a timeout. An HTTP error status is
not a failure, so check the status:

```php
if ($response->ok()) {
    $data = $response->json();
} else {
    echo "upstream returned " . $response->status();
}
```

## Read a request

`HTTP\Request` is a `net/http` request, so its fields and methods are Go's,
matched case-insensitively:

```php
$trace_id = "b7c1";
$request = new HTTP\Request("GET", "https://api.example.com/users?page=1");

echo $request->method;          // GET
echo $request->host;            // api.example.com
echo $request->url->path;       // /users
echo $request->url->string();   // the full URL

$request->header->set("Accept", "application/json");
$request->header->add("X-Trace", $trace_id);
echo $request->header->get("accept");
```

Everything `net/http` exports on a request is reachable, so there is one
vocabulary for a request rather than a PHP-side name for each part of it.

## Read the request being served

`HTTP\Request::current()` returns the inbound request, as the same
`HTTP\Request` an outbound one is, and `null` off a request: a command line
run, a `@startup` or a `@schedule` job.

```php
$request = HTTP\Request::current();

echo $request->method;                     // POST
echo $request->url->path;                  // /api/echo/a/b
echo $request->header->get("content-type");
echo $request->useragent();
echo $request->referer();

list($user, $password, $sent) = $request->basicauth();
```

It is not a replacement for the superglobals, and the two are not
interchangeable. Read the request itself for what `net/http` answers and PHP
has no name for — `useragent()`, `referer()`, `basicauth()`, `cookie($name)`,
`formvalue($name)`, and `pattern`, the route the request matched. Read the
superglobals for anything with more than one value in it:

| For                   | Read                | Not                                    |
|-----------------------|---------------------|----------------------------------------|
| Query fields          | `$_GET`             | `$request->url->query()`               |
| Every request header  | `getallheaders()`   | `$request->header`                     |
| Form fields           | `$_POST`            | `$request->form`, `$request->postform` |
| The raw body          | `php://input`       | `$request->body`                       |
| Route path parameters | `$_REQUEST`         | `$request->pathvalue($name)`           |
| Whether this is TLS   | `$_SERVER["HTTPS"]` | `$request->tls`                        |

The left column is an ordered PHP array and the right one is a Go map, whose
key order is re-randomised on every pass and whose values are lists rather than
strings. `$request->body` is an `io.ReadCloser`, a stream a script has no way
to hold or close. `$request->pathvalue()` takes the name the *router* captured,
which is not always the name the annotation wrote: chi publishes a
`{rest...}` parameter under `*`, where `$_REQUEST` carries it under `rest`.
`$request->tls` is a pointer, and a nil pointer is neither `null` nor false, so
`if ($request->tls)` is true on a plain HTTP request; `isset($_SERVER["HTTPS"])`
is the question that answers itself.

[api-echo.php](../../demos/example/api-echo.php) is the worked example: an
httpbin-style endpoint that encodes the request it was given, reading each part
from whichever of the two answers it well.

## Read a response

| Method          | Returns                                                              |
|-----------------|----------------------------------------------------------------------|
| `status()`      | The status code as an int, `0` when no response arrived              |
| `ok()`          | Whether a response arrived with a 2xx status                         |
| `err()`         | Why the request failed, or an empty string                           |
| `body()`        | The body as a string                                                 |
| `header($name)` | One response header, matched case-insensitively                      |
| `headers()`     | Every response header as an array                                    |
| `json()`        | The body decoded into arrays and scalars; throws when it is not JSON |

The body is read in full when the response is constructed, and bounded at
32 MiB. A script has no way to close a stream, so an unread body would leak its
connection when the request ended.

## Send several requests at once

`parallel()` takes an array of requests keyed by a name the script chooses, and
returns the responses under those names. A page making three calls waits for
the slowest rather than the sum:

```php
$results = $client->parallel(array(
    "users" => new HTTP\Request("GET", "/users"),
    "stats" => new HTTP\Request("GET", "/stats"),
    "flags" => new HTTP\Request("GET", "/flags"),
));

foreach ($results as $name => $response) {
    if ($response->ok()) {
        echo $name . ": " . $response->status() . "\n";
    } else {
        echo $name . ": " . $response->err() . "\n";
    }
}
```

One request failing does not fail the others and does not throw. That response
reports `ok()` as false and `err()` as the reason, so a page renders what it
has instead of losing every result to one unreachable host:

```php
$results = $client->parallel(array(
    "users"   => new HTTP\Request("GET", "/users"),
    "offline" => new HTTP\Request("GET", "http://127.0.0.1:1/nope"),
));

$results["users"]->ok();      // true
$results["users"]->status();  // 200

$results["offline"]->ok();      // false
$results["offline"]->status();  // 0, no response arrived
$results["offline"]->err();     // the transport error, ending in "connection refused"
```

`parallel()` throws when the argument is not an array of `HTTP\Request`, which
is reported before any connection is opened. Each request is bounded by the
client's timeout, and the client's headers and `base_url` apply to all of them.

## A page that calls an API

Putting it together, a route that renders a user list from an upstream service:

```php
<?php
// @route GET /users

$client = new HTTP\Client(array(
    "base_url" => getenv("API_URL"),
    "timeout"  => 5,
    "headers"  => array("Authorization" => "Bearer " . getenv("API_TOKEN")),
));

try {
    $response = $client->get("/users");
} catch (Exception $e) {
    http_response_code(502);
    echo "upstream unavailable";
    return;
}

if (!$response->ok()) {
    http_response_code(502);
    echo "upstream returned " . $response->status();
    return;
}

foreach ($response->json() as $user) {
    echo htmlspecialchars($user["name"]) . "<br>";
}
```

A transport failure throws and an error status does not, so both are handled,
and neither leaves the page rendering a half-built list.

## Tracing

`send()` records an external span per request, carrying the method, host,
status and byte count. `parallel()` records one span for the batch alongside
the per-request spans, so a slow page shows which upstream call it waited on.
See [Telemetry](../telemetry.md).
