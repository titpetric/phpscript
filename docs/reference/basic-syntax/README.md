# Basic syntax

| PHP language-reference feature | Status              | Notes                                                                                                    |
|--------------------------------|---------------------|----------------------------------------------------------------------------------------------------------|
| PHP tags                       | Compatibility       | The lexer recognizes `<?php`, short `<?`, and `?>`.                                                      |
| Escaping from HTML             | Compatibility       | phpscript emits text outside PHP tags; as in PHP, the lexer consumes one newline immediately after `?>`. |
| Instruction separation         | Compatibility       | A semicolon ends a simple statement; a closing tag also ends PHP mode.                                   |
| Comments                       | Compatibility       | The parser accepts `//`, `#`, and `/* ... */` comments.                                                  |
| Shebang                        | phpscript extension | The lexer skips a leading `#!` line and a script runs as an executable.                                  |
| `declare()`                    | Parsed, ignored     | The parser reads a directive and drops it; the runtime has one set of semantics to select from.          |

phpscript accepts files containing PHP blocks and inline text. The closing tag is optional at the end of a file. The lexer consumes one newline immediately following a closing tag.

## declare()

phpscript parses `declare(strict_types=1);` and the other directives, then ignores them. No semantics sit behind them to select: phpscript does not coerce argument types at all and neither strict nor coercive typing changes what a call does. Files written for stock PHP parse and run unchanged.

```php
<?php
declare(strict_types=1);
```

A block form, `declare(ticks=1) { ... }`, runs its block as ordinary code.

## PHP tags

```php
<?php
echo "Hello\n";
```

Short open tags do not depend on PHP configuration:

```php
<? echo "Hello"; ?>
```

`<?= ... ?>` is not implemented; use `<?php echo ...; ?>`.

## Escaping from HTML

```php
<h1><?php echo "Hello"; ?></h1>
```

## Instruction separation

Use semicolons after assignments, calls, `echo`, `return`, `break`, and `continue`. Brace-delimited declarations and control structures do not need a trailing semicolon.

## Comments

```php
// line comment
# line comment
/* block comment */
```

## Shebang scripts

```php
#!/usr/bin/env phpscript
<?php
echo "Hello world\n";
```
