# Predefined variables

| PHP predefined variable                  | Status                | Notes                                                              |
|------------------------------------------|-----------------------|--------------------------------------------------------------------|
| `$_GET`                                  | Compatibility         | Query-string fields, bracket names decoded into nested arrays.     |
| `$_POST`                                 | Compatibility         | Form fields, urlencoded or multipart, decoded the same way.        |
| `$_FILES`                                | Partial compatibility | File parts of a multipart body, with php.ini's size limits.        |
| `$_COOKIE`                               | Compatibility         | Cookies sent with the request, decoded the same way.               |
| `$_SERVER`                               | Partial compatibility | The request line, peer, scheme, timing, `HTTP_*`, and the script.  |
| `$_ENV`                                  | Partial compatibility | Seeded empty. Only a Go host fills it; `getenv()` is unrelated.    |
| `$argc`, `$argv`                         | Partial compatibility | Seeded for scheduled jobs. CLI arguments are not passed to either. |
| `$_REQUEST`                              | Partial compatibility | Query, form and cookie fields, with route path values merged over. |
| `$GLOBALS`, `$_SESSION`                  | Not implemented       | Reserved names, never seeded, so they read as null.                |
| `$php_errormsg`, `$http_response_header` | Not implemented       | PHP error and stream globals are unavailable.                      |

These arrays are installed only when a Go host creates and registers a request
context. Their values are ordinary phpscript arrays, while their reserved names
remain visible in function scopes like PHP superglobals.

A context built from an HTTP request fills what that request carries. A context
built without one, which is what the `phpscript` CLI and a startup job use,
installs the same arrays empty, so a script reads them rather than failing on
an undefined name.

## `$_GET`

Contains URL query parameters. A key sent more than once keeps the last value,
as it does in PHP.

```php
$page = $_GET["page"];
```

Bracket syntax in a field name is decoded into nested arrays, so the query
`a[b]=1&ids[]=7&ids[]=9` arrives as `$_GET["a"]["b"]` and a two-element
`$_GET["ids"]`, not as the literal keys `a[b]` and `ids[]`. `parse_str()` is
the same decoder applied to a string, and
[Arrays](../types/README.md#keys) covers which field names become integer
keys.

The order is the request's own, and two limits bound what a hostile query can
build: `max_input_vars` (1000) and `max_input_nesting_level` (64). A field
nested past the limit is dropped whole rather than truncated. Both are
[configurable](../../configuration.md).

## `$_POST`

Contains parsed form-body values, last one wins. Both body encodings a browser
form produces are decoded: `application/x-www-form-urlencoded` and
`multipart/form-data`. The value parts of a multipart body land here; its file
parts land in `$_FILES`.

Field names are decoded the same way `$_GET` decodes them, so a repeating form
row named `line[0][hours]` arrives as `$_POST["line"][0]["hours"]`. `$_FILES`
is the exception: it is still keyed by the literal field name, so a file input
named `docs[]` is one entry `docs[]` rather than a nested one.

## `$_FILES`

Contains the file parts of a `multipart/form-data` body, keyed by form field
name, each with PHP's keys:

```php
$file = $_FILES["avatar"];
if ($file["error"] === 0) {
    move_uploaded_file($file["tmp_name"], "uploads/" . $file["name"]);
}
```

| Key         | Value                                                            |
|-------------|------------------------------------------------------------------|
| `name`      | Client-supplied file name, with any directory part stripped.     |
| `full_path` | Client-supplied file name as sent, directory part included.      |
| `type`      | Client-supplied content type. It is not verified.                |
| `tmp_name`  | Absolute path of the server-side temporary copy.                 |
| `error`     | `UPLOAD_ERR_OK` (0), or 1, 4, 6, 7 when the part was not stored. |
| `size`      | Bytes written to `tmp_name`.                                     |

A field named `files[]` collects every file sent under it, and its entry holds
one array per key rather than one value: `$_FILES["files"]["name"][0]`. Any
other field takes the last file sent under it, the way a repeated form value
assigns over the one before it.

The temporary copy lives for the duration of one request; a host handler
removes it after the response is written. `move_uploaded_file()` puts an upload
somewhere permanent, and refuses any path the request did not produce, as does
`is_uploaded_file()`. Both are installed with the rest of the filesystem shims,
so the destination is resolved against the same root.

A stored upload is given the `upload_file_mode` of the runner, `0644` by
default, because the temporary copy it came from is readable by nobody but this
process. See [Configuration](../../configuration.md#upload-file-mode).

### Size limits

`upload_max_filesize` and `post_max_size` are configuration keys of the runner,
written and defaulted as php.ini writes them; see
[Configuration](../../configuration.md#sizes). A file part over
`upload_max_filesize` is not stored and its entry carries
`UPLOAD_ERR_INI_SIZE`, so the rest of the form still reaches the script. A body
over `post_max_size` is not parsed at all, so both `$_POST` and `$_FILES` are
empty.

An entry for a part that was not stored, for any of the error codes, describes
only what the client sent: `name` and `full_path` are filled, `type` and
`tmp_name` are empty and `size` is `0`.

Neither is catchable: they happen before the script runs. The Go host is told
why through `Runtime.RecordError`; see
[Errors a script cannot catch](../errors/README.md#errors-a-script-cannot-catch).

PHP's per-form `MAX_FILE_SIZE` field, and the `UPLOAD_ERR_FORM_SIZE` and
`UPLOAD_ERR_PARTIAL` codes that go with it, are not implemented.

## `$_COOKIE`

Contains one value per cookie sent with the request, keyed by cookie name.
Setting a cookie is `setcookie()`, or `header("Set-Cookie: ...")` for a header this runtime spells differently.

## `$_SERVER`

Contains the part of PHP's server array that an HTTP request answers for on its
own:

| Key                                             | Value                                                             |
|-------------------------------------------------|-------------------------------------------------------------------|
| `REQUEST_METHOD`, `REQUEST_URI`, `QUERY_STRING` | The request line, as the Go server parsed it.                     |
| `HTTP_HOST`, `SERVER_PROTOCOL`                  | Host header and protocol version.                                 |
| `REMOTE_ADDR`, `REMOTE_PORT`                    | The peer address and port, split as PHP splits them.              |
| `REQUEST_SCHEME`, `HTTPS`                       | `http` or `https`. `HTTPS` is `on` over TLS and absent otherwise. |
| `CONTENT_TYPE`, `CONTENT_LENGTH`                | Present when the request announced them, as in PHP.               |
| `REQUEST_TIME`, `REQUEST_TIME_FLOAT`            | When the request was received. An integer and a float.            |
| `HTTP_*`                                        | One key per request header, upper-cased with `-` as `_`.          |

`phpscript server` adds the keys that depend on where the site lives, which the
request alone does not say:

| Key                          | Value                                                             |
|------------------------------|-------------------------------------------------------------------|
| `DOCUMENT_ROOT`              | The served directory of the application root.                     |
| `SCRIPT_NAME`, `PHP_SELF`    | The entrypoint as a URL path.                                     |
| `SCRIPT_FILENAME`            | The entrypoint on disk.                                           |
| `SERVER_NAME`, `SERVER_PORT` | The name the request arrived under, and its port when it had one. |
| `SERVER_SOFTWARE`            | `phpscript`.                                                      |

`SERVER_NAME` is the requested host rather than the listening socket PHP reads
it from, because this server routes by `Host`: the name a request arrived under
is the site it reached. `X-Forwarded-Proto` is not consulted for
`REQUEST_SCHEME`, since a client can send it; a host behind a proxy it trusts
sets the two scheme keys itself.

`GATEWAY_INTERFACE` and `PATH_INFO` are absent.

## `$_ENV`

Installed, and empty unless a Go host puts something in the request context it
registers. It is not the process environment: `getenv()` and `putenv()` read
and write a separate map seeded from the process, and neither one is visible in
`$_ENV`.

## `$argc` and `$argv`

Installed, and filled for a scheduled job: `$argv[0]` is the file, and what
follows is whatever its `// @schedule` annotation put after `--`. Everywhere
else they are `0` and an empty array; the arguments passed to the `phpscript`
CLI are not among them.

## `$_REQUEST`

Contains the query, form and cookie fields merged the way PHP's default
`request_order` merges them — `$_GET`, then `$_POST`, then `$_COOKIE`, a later
source overwriting an earlier one — and then the values captured by the
matched route pattern, such as `{id}` or `{rest...}`, written over all three.

```php
// @route GET /users/{id}
$id = $_REQUEST["id"];
```

The path values are the deliberate divergence from PHP, whose `$_REQUEST`
carries no route parameters: a path parameter is request input here, and it
arrives under PHP's name rather than under one PHP does not have. A request
field spelled like a path parameter is overwritten by it, so
`/users/42?id=abc` answers `"42"`.

`$_REQUEST` is its own array, as it is in PHP: writing to it changes none of
the arrays it was merged from, and their writes do not appear in it.

`$_PATH`, the phpscript-only name that used to carry the path values, is gone;
`$_REQUEST` is where they arrive.

## Request headers

Request headers reach a script two ways: as `HTTP_*` keys in `$_SERVER`, and
through `getallheaders()`, `get_all_headers()`, or `apache_request_headers()`,
which return them under their canonical names.
