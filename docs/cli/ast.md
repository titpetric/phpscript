# `phpscript ast <file.php>`

Tokenize a PHP file and print its PHP-style token stream.

It accepts the [global flags](README.md#global-flags) and nothing else: `-f`,
`-w`, `--include`, `-v`, `--cpuprofile`, `--memprofile`, `--cover` and
`--coverfile`.

```bash
phpscript ast tests/fixtures/syntax/code/TestCase.php
```

The output uses the same token names exposed by `token_get_all()` and
`token_name()`, such as `T_OPEN_TAG`, `T_STRING`, `T_VARIABLE`, and
`T_OBJECT_OPERATOR`, plus `CHAR` for single-character tokens. Each line includes
the source line number, token name, and raw token text.

This is a debugging/development helper. It is useful when checking how PHP
source is tokenized before changing parser or runtime behavior.
