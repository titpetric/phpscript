# PHP Compatibility

While not much of PHP is directly compatible due to the missing standard
library APIs, there is some coverage to pass loading a PHP template
engine and using it. In order for that to work, several low level PHP
functions needed to be implemented.

To use the PHP standard library APIs:

- import `github.com/titpetric/phpscript/stdlib`
- use `stdlib.Register(rt)` to register pure (non-filesystem) shims and PHP constants
- use `stdlib.RegisterFS(rt, dir)` to add filesystem IO bound to a root directory

## Strings

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

## Arrays

- `count`
- `in_array`
- `array_unique`
- `array_merge`
- `array_keys`
- `array_values`
- `usort`

## Language Constructs

- `isset`
- `empty`
- `is_array`
- `is_string`
- `is_object`
- `is_numeric`
- `function_exists`

## Tokenizer + Constants

- `token_get_all`
- `token_name`
- `T_VARIABLE`
- `T_OBJECT_OPERATOR`
- `T_STRING`

## Filesystem

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

## Regex

- `preg_match_all`
- `preg_match`
- `preg_replace`
