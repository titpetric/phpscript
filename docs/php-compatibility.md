# PHP Compatibility

phpscript intentionally implements only a small PHP-compatible surface. The VM
parses and runs a PHP4-like subset with `Exception`, `throw`, `try`, `catch` and
`finally`, while PHP standard-library behavior is provided by opt-in Go shims.

To use the provided PHP standard library APIs:

- import `github.com/titpetric/phpscript/stdlib`
- use `stdlib.Register(rt)` to register pure (non-filesystem) shims and PHP constants
- use `stdlib.RegisterFS(rt, dir)` to add filesystem IO bound to a root directory
- use `runner.Context.Register(rt)` to register request-aware HTTP shims and seed request globals

## Implemented shims

### Strings

- `strlen`
- `strtoupper`
- `strtolower`
- `trim`
- `rtrim`
- `ltrim`
- `substr`
- `strpos`
- `strstr`
- `str_replace`
- `str_repeat`
- `implode`
- `explode`
- `htmlspecialchars`
- `sprintf`
- `crc32`

### Arrays

- `count`
- `in_array`
- `array_unique`
- `array_merge`
- `array_keys`
- `array_values`
- `usort`

### Language helpers

- `isset`
- `empty`
- `is_array`
- `is_string`
- `is_object`
- `is_numeric`
- `function_exists` (currently returns `false`)
- `get_included_files`

### Tokenizer + Constants

- `token_get_all`
- `token_name`
- `T_VARIABLE`
- `T_OBJECT_OPERATOR`
- `T_STRING`

### Filesystem

These are registered separately with `stdlib.RegisterFS(rt, dir)`.

- `file_get_contents`
- `file_exists`
- `filemtime`
- `dirname`
- `basename`
- `mkdir`
- `unlink`
- `fopen`
- `fwrite`
- `fclose`

### Regex

- `preg_match_all`
- `preg_match`
- `preg_replace`

### HTTP request / response

These are registered separately with `runner.Context.Register(rt)`.

- `getallheaders`
- `get_all_headers`
- `apache_request_headers`
- `header`
- `$_GET`
- `$_POST`
- `$_PATH`
- `$_SERVER`

## Explicitly not implemented by design

- `set_error_handler`
- `trigger_error`

Error handling uses Go errors surfaced into the PHP runtime as thrown errors.
PHP code should use `try` / `catch` instead of PHP4-style warning/error handler
APIs. Hosts that need global error reporting should use `Runtime.OnError`.

## Common missing shims

The current shim set was sized for the minitpl compatibility target, not for a
general PHP standard library. Common additions likely to unlock more scripts:

- JSON: `json_encode`, `json_decode`, `json_last_error`, `json_last_error_msg`
- Array utilities: `array_map`, `array_filter`, `array_search`, `array_key_exists`, `array_slice`, `array_reverse`, `array_diff`, `array_intersect`
- String utilities: `str_contains`, `str_starts_with`, `str_ends_with`, `strtolower`/`strtoupper` are present, but `ucfirst`, `ucwords`, `strtr`, `preg_split` are not
- URL/query helpers: `parse_url`, `parse_str`, `http_build_query`, `urlencode`, `urldecode`, `rawurlencode`, `rawurldecode`
- Date/time and randomness: `time`, `date`, `strtotime`, `microtime`, `rand`, `mt_rand`
- Type/casting helpers: `is_int`, `is_bool`, `is_float`, `is_null`, `gettype`, `intval`, `strval`, `boolval`, `floatval`
- Filesystem/path helpers: `realpath`, `is_file`, `is_dir`, `filesize`, `pathinfo`, `glob`, `rename`, `copy`
- Output buffering: `ob_start`, `ob_get_clean`, `ob_get_contents`, `ob_end_clean`
