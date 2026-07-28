# Basic syntax

| PHP language-reference feature | Status | Notes |
| --- | --- | --- |
| PHP tags | Compatibility | `<?php`, short `<?`, and `?>` are recognized. |
| Escaping from HTML | Compatibility | Text outside PHP tags is emitted; as in PHP, one newline immediately after `?>` is consumed. |
| Instruction separation | Compatibility | Semicolons terminate simple statements; a closing tag also ends PHP mode. |
| Comments | Compatibility | `//`, `#`, and `/* ... */` comments are supported. |
| Shebang | phpscript extension | A leading `#!` line is ignored, allowing executable scripts. |

phpscript accepts files containing PHP blocks and inline text. The closing tag
is optional at the end of a file. One newline immediately following a closing
tag is consumed rather than emitted.

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

Use semicolons after assignments, calls, `echo`, `return`, `break`, and
`continue`. Brace-delimited declarations and control structures do not need a
trailing semicolon.

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
