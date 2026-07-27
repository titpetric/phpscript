# PHP Compatibility

phpscript intentionally implements only a small PHP-compatible surface. The VM
parses and runs a PHP4-like subset with `Exception`, `throw`, `try`, `catch` and
`finally`, while PHP standard-library behavior is provided by opt-in Go shims.

To use the provided PHP standard library APIs:

- import `github.com/titpetric/phpscript/stdlib`
- use `stdlib.Register(rt)` to register pure (non-filesystem) shims and PHP constants
- use `stdlib.RegisterFS(rt, dir)` to add filesystem IO bound to a root directory
- use `runner.Context.Register(rt)` to register request-aware HTTP shims and seed request globals

## Implemented APIs

The authoritative function inventory is generated from the runtime itself:

- [Implemented PHP APIs](./implemented-apis.md)
- [Deferred callbacks](./defer.md)
- Regenerate with `phpscript list-apis.php > docs/implemented-apis.md`

This avoids a manually maintained list drifting from the functions actually
registered by `stdlib.Register`, `stdlib.RegisterFS`, and
`runner.Context.Register` in the standard CLI runtime.

### Database API (stdlib/database.go)

The database extensions are unique to phpscript and are exposed as
`PS\Database`.

- Database connection pooling: Max 20 open connections set via `db.SetMaxOpenConns(20)`
- Supported engines: SQLite, PostgreSQL, MySQL via DSN configuration
- Key methods: `connect()`, `query()`, `insert()`, `get()`, `get_all()`
- Transactions: `begin()`, `commit()`, `rollback()`

### Filesystem

These are registered separately with `stdlib.RegisterFS(rt, dir)`.

### HTTP request / response

These are registered separately with `runner.Context.Register(rt)`.

Request globals include `$_GET`, `$_POST`, `$_PATH`, and `$_SERVER`.

### JSON

Simplified JSON encoding and decoding functions are provided. They are not API
compatible with the PHP standard library because they do not support options
constants.

## Explicitly not implemented by design

- `set_error_handler`
- `trigger_error`
- `global`

Error handling uses Go errors surfaced into the PHP runtime as thrown errors.
PHP code should use `try` / `catch` instead of PHP4-style warning/error handler
APIs. Hosts that need global error reporting should use `Runtime.OnError`.

## Common missing shims

The current shim set was sized for the minitpl compatibility target, not for a
general PHP standard library. Common additions likely to unlock more scripts:

- Array utilities: `array_filter`, `array_search`, `array_key_exists`, `array_reverse`, `array_diff`, `array_intersect`
- String utilities: `str_contains`, `str_starts_with`, `str_ends_with`, `strtolower`/`strtoupper` are present, but `ucfirst`, `ucwords`, `strtr`, `preg_split` are not
- URL/query helpers: `parse_url`, `parse_str`, `http_build_query`, `urlencode`, `urldecode`, `rawurlencode`, `rawurldecode`
- Date/time and randomness: `time`, `date`, `strtotime`, `microtime`, `rand`, `mt_rand`
- Type/casting helpers: `is_bool`, `is_float`, `is_null`, `gettype`, `intval`, `strval`, `boolval`, `floatval`
- Filesystem/path helpers: `realpath`, `is_file`, `is_dir`, `filesize`, `pathinfo`, `glob`, `rename`, `copy`
- Output buffering: `ob_start`, `ob_get_clean`, `ob_get_contents`, `ob_end_clean`
