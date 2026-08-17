# The template engine is a dependency now, not a copy

**2026-08-17.** phpscript carried its own `Template.php` and `Compiler.php`:
[titpetric/minitpl](https://github.com/titpetric/minitpl) with the parts the
interpreter could not run edited out, in two places — `tests/fixtures/code/` and
`demos/example/include/`. Both copies are gone. The engine arrives through
composer, unmodified, and the gaps that forced the edits were closed in the
runtime instead.

```php
<?php

include "vendor/autoload.php";

$tpl = new MiniTPL\Template("templates/");
```

That file runs under phpscript and under stock PHP. `atkins composer:install`
installs `tests/fixtures` and `demos/example`; `vendor/` is not committed.

## Composer resolution

composer's generated `vendor/autoload.php` bootstraps a ~600 line `ClassLoader`
through static properties, closure binding and generated class names — none of
which the interpreter implements. The data behind it is plain JSON, so the new
[composer](../../composer) package reads `composer.json` and
`vendor/composer/installed.json` and installs a Go class loader from the PSR-4,
PSR-0 and `files` entries it finds there. `classmap` is not supported: composer
only emits it as PHP.

Including `vendor/autoload.php` is what turns autoloading on. Binding it through
the new `Runtime.RegisterInclude` — rather than installing the loader when the
runtime is built — keeps PHP's semantics: a script that never includes the
autoloader sees no vendor classes, and one that includes a missing autoloader
fails the way PHP would. In practice it is one include in `bootstrap.php`.

`vendor/` is now skipped when scanning for `@route` and `@startup` annotations
and by `phpscript list`. A dependency does not get to publish endpoints into the
application, and the inventory stays about the application's own files.

See [Composer support](../composer.md).

## What the engine needed

The edits in the old copies were each a missing piece of the runtime:

- **Namespaces reaching global functions.** Inside `namespace MiniTPL`, an
  unqualified call resolves in the namespace first and then falls back to the
  global function. That fallback existed but missed two paths: `func_get_args`
  is a frame-aware builtin held in the evaluation environment rather than the
  function table, and the by-reference argument table used to be keyed only by
  the qualified name, so `preg_match_all($re, $subject, $matches)` stopped
  filling `$matches`.

- **PHP callables.** `call_user_func_array` accepted a Go func and nothing else,
  which is why the copy replaced `array($this, "_find_path")` with a property
  read. `Runtime.Callable` now resolves every spelling PHP accepts — a closure,
  `"function"`, `"Class::method"`, `array($object, "method")` and
  `array("Class", "method")` — and `call_user_func`, `is_callable`, `usort` and
  `array_map` all go through it.

- **Output buffering.** `Template::get()` renders to a string by capturing what
  a compiled template echoes. `ob_start`, `ob_get_contents`, `ob_get_clean`,
  `ob_end_clean`, `ob_end_flush`, `ob_get_flush` and `ob_get_level` are
  implemented as a buffer stack behind `Runtime.Output()`.

- **`function_exists`.** It returned false unconditionally. Compiled templates
  guard their generated block functions with it, so every include redeclared
  them.

Upstream met the runtime halfway: minitpl moved from `Monotek\MiniTPL` to
`MiniTPL`, declared the properties it used to create in its constructor, and
guarded its one `$GLOBALS` read with `isset` so a runtime without the
superglobal treats the object as a template variable instead. It still compiles
all thirteen of its own template fixtures byte-identically under PHP 8.4.

## Fixtures

`include_minitpl.phpt` is gone: the embedded fixture filesystem cannot hold an
uncommitted `vendor/`, so the engine integration test is
`tests/fixtures/test-minitpl.php`, which asserts its own output and runs under
both `phpscript` and `php` in the pipeline. The autoloader itself is covered by
Go tests over an `fstest.MapFS` composer project, and `callables.phpt` and
`output_buffering.phpt` cover the new standard-library surface.

The parser benchmarks still read the engine sources, now from
`tests/fixtures/vendor/`, and skip when composer has not run.
