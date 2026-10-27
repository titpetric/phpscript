# `phpscript fmt <path>...`

Format one or more PHP files or directories in place. A directory path formats PHP files directly in that directory; append `/...` to include its subdirectories. With no path, the command uses the current directory (`.`).

It also accepts the [global flags](README.md#global-flags): `-f`, `-w`, `--include`, `-v`, `--cpuprofile`, `--memprofile`, `--cover` and `--coverfile`. The flags below are this command's own.

```bash
phpscript fmt script.php
phpscript fmt ./src        # PHP files directly in ./src
phpscript fmt ./src/...    # PHP files in ./src and its subdirectories
phpscript fmt -l ./src/... # list what needs formatting, rewrite nothing
```

Use `--list` (`-l`) to name the files that would change and rewrite none of them, which is the CI check: the command exits 0 either way and a pipeline reads the output.

The formatter uses tabs for indentation, keeps class, function and control-statement opening braces on the declaration line, and normalizes line endings to LF. It prints class members as constants, then properties, then methods, keeping the blank lines written between them. It breaks an array literal onto one line per entry, with a trailing comma, when the literal holds more than two key/value pairs or does not fit in 100 columns. It keeps comments, the quoting of string literals, type hints and imports unchanged.

On standard error, the formatter reports a file it cannot read in full; it leaves that file alone and formats the rest:

```
$ phpscript fmt ./vendor/...
vendor/titpetric/minitpl/code/MiniTPL/Compiler.php
skipped vendor/titpetric/minitpl/test/TemplateTest.php: line 3: expected "{", got 3("extends")@3
```

Before a file is rewritten, its formatted output has to parse and formatting it again has to produce the same text. The formatter skips a file that fails either check and leaves it alone.
