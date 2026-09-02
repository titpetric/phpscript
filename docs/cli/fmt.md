# `phpscript fmt <path>...`

Format one or more PHP files or directories in place. A directory path formats
PHP files directly in that directory; append `/...` to include its
subdirectories. With no path, the command uses the current directory (`.`).

It also accepts the [global flags](README.md#global-flags): `-f`, `-w`,
`--include`, `-v`, `--cpuprofile`, `--memprofile`, `--cover` and `--coverfile`.
The flags below are this command's own.

```bash
phpscript fmt script.php
phpscript fmt ./src        # PHP files directly in ./src
phpscript fmt ./src/...    # PHP files in ./src and its subdirectories
phpscript fmt -l ./src/... # list what needs formatting, rewrite nothing
```

The formatter uses tabs for indentation, keeps class, function and
control-statement opening braces on the declaration line, and normalizes line
endings to LF. Class members are printed as constants, then properties, then
methods, keeping the blank lines written between them. An array literal with
more than two key/value pairs, or one that does not fit in 100 columns, is
printed one entry per line with a trailing comma. Comments, the quoting of
string literals, type hints and imports are kept as they were written.

A file the formatter cannot read in full is reported on standard error and
left alone, and the remaining files are still formatted:

```
$ phpscript fmt ./vendor/...
vendor/titpetric/minitpl/code/MiniTPL/Compiler.php
skipped vendor/titpetric/minitpl/test/TemplateTest.php: line 3: expected "{", got 3("extends")@3
```

Before a file is rewritten, its formatted output has to parse and formatting
it again has to produce the same text. A file that fails either check is
skipped rather than written.
