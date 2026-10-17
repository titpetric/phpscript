# phpscript-service

The test server's API written the way a phpscript application is written: one file per endpoint, the `// @route METHOD /path` annotation above it as the whole routing table, and the request in the superglobals. No front controller, and nothing in the program registers a handler.

```php
<?php

// @route GET /users/{id}

header("Content-Type: application/json");

$path = explode("?", $_SERVER["REQUEST_URI"]);

echo json_encode(array("id" => $_REQUEST["id"], "path" => $path[0])) . "\n";
```

`{id}` arrives in `$_REQUEST` under the name the annotation declares. `$_GET` is the query, `$_POST` is the form body, `$_SERVER` carries the CGI keys, and `echo` is the response.

## Running it

```sh
docker compose up -d --wait
curl http://127.0.0.1:8097/users/42
docker compose down
```

Or without docker, from the repository root:

```sh
go build -o bin/phpscript .
bin/phpscript server -f testdata/phpscript-service/config.yml testdata/phpscript-service
```

`config.yml` is what makes the directory an application: `routes: enabled: true` turns on the annotation scan, and `server: addr` is the port. `public/` is the web root `phpscript server` requires and is empty on purpose, because a route annotation inside it is ignored.

The compose file mounts the binary instead of baking one in. A benchmark measures the build under test, and an image carrying its own phpscript would answer for a binary nobody asked about.

## This is Path A

`phpscript server` builds a runtime per request. `testdata/phpscript-http-bridge` is Path B: the script is the server, it is parsed once, each handler is resolved at `$mux->handle()`, and the workers are forked once at `listen()`.

A number from one never belongs in a table with a number from the other, which is why `scripts/bench-http.sh` writes one table per runtime and heads each with the model its rows describe. [../../docs/agents/performance.md](../../docs/agents/performance.md) is the rule and [../../docs/design.md](../../docs/design.md) is why the two paths exist.

What the two cost, four segments of 2s per route at concurrency 1, four workers a side:

| runtime                     | /hello p50 us | /users p50 us | /echo p50 us | /hello req/s |
|-----------------------------|---------------|---------------|--------------|--------------|
| `phpscript-http-bridge` (B) | 47.0          | 50.0          | 54.0         | 19624        |
| `phpscript-service` (A)     | 188.0         | 199.0         | 209.0        | 4430         |

The gap is the runtime a request builds and tears down. A served application that wants the bridge's latency writes the bridge's shape; an application that wants one file per endpoint pays this instead. It is still well under a third of what the same API costs on Slim.

## Where it differs from the bridge

The endpoints answer the same bytes on every route, which is what makes the two columns comparable. Two things are written differently because the host is different:

- `header()` and `echo` in place of `$w->header()->set()` and `$w->write()`. The response is the one the server built for this request, so there is no writer to hold.
- `json_encode(...) . "\n"` in place of `(new JSON\Encoder($w))->encode(...)`. The encoder writes to a stream and takes the newline with it; `json_encode` returns a document and the newline is appended. The bytes are the same either way, phpscript's `json_encode` being the same encoder.

## Known gap

`error_log()` from an annotated endpoint does not reach the server's log. The sweep's `/slow` abort check greps that log for the line the endpoint writes when it notices a client leaving, so the check reports `FAIL` on this side and the other four pass it. The endpoint runs and answers; what is lost is the diagnostic.
