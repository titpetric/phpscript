# Predefined variables

| PHP predefined variable                        | Status                | Notes                                                          |
|------------------------------------------------|-----------------------|----------------------------------------------------------------|
| `$_GET`                                        | Compatibility         | First query-string value for each key.                         |
| `$_POST`                                       | Partial compatibility | First parsed form value for each key; uploads are unavailable. |
| `$_PATH`                                       | phpscript extension   | Route wildcard values from the matched Go HTTP pattern.        |
| `$GLOBALS`, `$_SERVER`, `$_FILES`, `$_REQUEST` | Not implemented       | Not seeded by the request runtime.                             |
| `$_SESSION`, `$_ENV`, `$_COOKIE`               | Not implemented       | Not seeded by the request runtime.                             |
| `$argc`, `$argv`                               | Not implemented       | CLI arguments are not exposed as PHP predefined variables.     |
| `$php_errormsg`, `$http_response_header`       | Not implemented       | PHP error and stream globals are unavailable.                  |

These arrays are installed only when a Go host creates and registers a request
context. Their values are ordinary phpscript arrays, while their reserved names
remain visible in function scopes like PHP superglobals.

## `$_GET`

Contains URL query parameters. Repeated values are flattened to the first
value.

```php
$page = $_GET["page"];
```

## `$_POST`

Contains parsed form-body values, also flattened to one string per key.

## `$_PATH`

Contains values captured by Go 1.22+ `ServeMux` route patterns, such as `{id}`
or `{rest...}`.

```php
$id = $_PATH["id"];
```

## Request headers

Headers are not exposed through `$_SERVER`. Use `getallheaders()`,
`get_all_headers()`, or `apache_request_headers()`.
