# `phpscript fmt <path>...`

Format one or more PHP files or directories in place. A directory path formats PHP files directly in that directory. Append `/...` to include its subdirectories. Without a path, the command uses the current directory (`.`).

It also accepts the [global flags](README.md#global-flags): `-f`, `-w`, `--include`, `-v`, `--cpuprofile`, `--memprofile`, `--cover` and `--coverfile`. The flags below belong to this command.

```bash
phpscript fmt script.php
phpscript fmt ./src        # PHP files directly in ./src
phpscript fmt ./src/...    # PHP files in ./src and its subdirectories
phpscript fmt -l ./src/... # list what needs formatting, rewrite nothing
```

Use `--list` (`-l`) to name the files that would change and rewrite none. That is the CI check: the command exits 0 either way. A pipeline reads the output.

The formatter uses tabs for indentation, keeps class, function and control-statement opening braces on the declaration line, and normalizes line endings to LF. A line that opens and closes PHP is template markup: the code and text on it stay as written, and only its leading indentation moves. A line that is PHP alone stays at the code's depth, and the markup inside an inline block and inside a container tag such as `<ul>` takes one level. A closing tag returns to its opener's level. A tag the formatter does not know keeps the function depth. It prints class members as constants, then properties, then methods, keeping the blank lines written between members. A call directly beneath an assignment follows it without a blank line. It breaks an array literal onto one line per entry, with a trailing comma, when the literal holds more than two key/value pairs or does not fit in 100 columns. It keeps comments, the quoting of string literals, type hints and imports unchanged.

A go runtime file takes go's spelling: no trailing semicolons, member names in lowerCamelCase. One newline ends the file. A file imports the runtime with `import` or `use Runtime\Go;`, and `phpscript fmt` reads that line to choose the style.

On standard error, the formatter reports a file it cannot read in full, leaves that file alone and formats the rest:

```
$ phpscript fmt ./vendor/...
vendor/titpetric/minitpl/code/MiniTPL/Compiler.php
skipped vendor/titpetric/minitpl/test/TemplateTest.php: line 3: expected "{", got 3("extends")@3
```

Before a file is rewritten, its formatted output must parse. Formatting the output again must produce the same text. The formatter skips a file that fails either check and leaves it alone.
