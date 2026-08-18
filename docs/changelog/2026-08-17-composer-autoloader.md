# composer's autoloader runs as interpreted PHP

**2026-08-17.** `demos/dbadmin` now pulls [titpetric/minitpl](https://github.com/titpetric/minitpl) in with composer, the way [demos/example](../use-cases/application.md) already declared it would. `require "vendor/autoload.php"` is the whole integration: phpscript interprets composer's generated `ClassLoader.php`, registers its `loadClass` method as an autoloader, and resolves PSR-4 and classmap entries from `composer.json`. Nothing about composer is special-cased in the runtime, and `runner.RegisterInclude`, the hook that existed so a host could reimplement `vendor/autoload.php` in Go, is not used by either demo.

The bootstrap failed at its first statement before this change:

```
parse "vendor/autoload.php": line 22: unexpected token 7(")")@22
```

Line 22 is `return ComposerAutoloaderInit...::getLoader();`. Static method invocation was not implemented: `Class::` parsed only as a class-constant reference.

## The language

composer's autoloader is unremarkable PHP, which is what makes it a useful test: every gap it hit is a gap any non-trivial library would hit.

| Construct                                                  | Before                     |
|------------------------------------------------------------|----------------------------|
| `Class::method()`, `self::method()`, `static::method()`    | parse error                |
| `static $prop` declarations, `Class::$prop` read and write | parse error                |
| `Class::class`                                             | resolved as a constant     |
| `function () use ($x) {}`                                  | `use` list parsed, dropped |
| `static function () {}`                                    | parse error                |
| `$fn($x)`, calling a value rather than a name              | parse error                |
| `use A\B\C;` / `use A\B\C as D;`                           | parse error                |
| `unset($a, $b[$k], $o->p, C::$s)`                          | undefined function         |
| `declare(strict_types=1);`                                 | parse error                |
| `function f(array $x): void`, parameter and return types   | parse error                |
| `false !== $pos = strrpos($s, "\\")`                       | parse error                |
| `$a[$x][$y] = $v` where `$a[$x]` does not exist            | assignment error           |
| `array(...) + $other`, array union                         | added as integers          |
| `spl_autoload_register(array($this, "loadClass"))`         | callback not callable      |

A static property is storage on the class, so it lives in one bag per class on the `Runtime` rather than on any instance; `self::` and `static::` resolve to the class of the running method, which without inheritance is the same thing. A `use (...)` capture is snapshotted where the closure value is created, matching PHP's by-value semantics; `&$name` is accepted and binds the same way, since the runtime has no reference cells to write back through. Parameter and return types are read and discarded; phpscript never coerces arguments, so there is nothing for `declare(strict_types=1)` to select either, and both are accepted so that stock-PHP sources parse unchanged.

`Closure::bind()` accepts a null `$newThis` and returns the closure as it is: phpscript enforces no property visibility, so a scope change has nothing to alter. Rebinding `$this` is an error rather than a silent no-op.

## The platform

The autoloader also expects a platform to exist before it will run: it gates on `PHP_VERSION_ID`, reports failures through `PHP_SAPI` / `STDERR` / `PHP_EOL`, probes for APCu with `function_exists` + `ini_get` + `filter_var`, and builds paths with `strtr` and `strrpos`. [stdlib/platform.go](../../stdlib/platform.go) adds those, the rest of the `PHP_*` constant set, the `ENT_*` and `FILTER_VALIDATE_*` flags, `define`/`defined`/`constant`, `get_class`/`method_exists`/`spl_object_id`, and the SPL exception class names so `throw new \InvalidArgumentException(...)` reaches the error path it was written for.

`PHP_VERSION` reports 8.4.0, the release phpscript's semantics are tested against. That is not a claim of compatibility with it.

## Two engines for preg_*

Running minitpl's own compiler over its own test templates, and diffing the compiled PHP against what php 8.4 produces from the same sources, found two more defects, neither of them composer's.

`"\xEF\xBB\xBF"` was three literal characters rather than three bytes: the lexer decoded `\n`, `\t`, `\r` and the quote escapes and left every numeric form alone, so minitpl's UTF-8 BOM check never fired. Both quote styles also shared one escape table, which made `'a\nb'` a newline in a single-quoted literal where PHP leaves it as two characters. [lexer.go](../../parser/lexer.go) now decodes `\xHH`, `\NNN`, `\u{...}`, `\v`, `\f` and `\e` in a double-quoted literal, and only `\\` and `\'` in a single-quoted one.

The second defect was worse. PHP's regexes are PCRE; Go's `regexp` is RE2, which cannot express a backreference at all. `stdlib/regex.go` handled that by reporting "no match" for such a pattern, which is correct only when the construct is absent from the input. minitpl pairs `{block foo}` with `{/block}` through `\1`, so the construct is very much present, and every `{block}` and `{inline}` definition compiled to the wrong output without an error anywhere.

[stdlib/compat/regex.go](../../stdlib/compat/regex.go) now compiles each pattern with whichever engine can express it: RE2 where RE2 suffices, and [regexp2](https://github.com/dlclark/regexp2), a backtracking engine with PCRE's feature set, for backreferences, lookahead, lookbehind, and anything else RE2 rejects. RE2 stays the default because it is faster and cannot be made to backtrack catastrophically; the fallback carries a one-second match timeout for the same reason. Which engine ran is not observable from PHP. `preg_quote` came along with it. See [regexp.md](../reference/extensions/regexp.md).

## The formatter discarded code it could not print

Adding AST nodes surfaced a defect in `phpscript fmt`, which rewrites files in place: a node the printer had no spelling for was printed as `/* unsupported statement */` and the original was lost. A bare identifier had the same problem in the other direction: constants share the `Var` node with variables, so `__DIR__` was printed as `$__DIR__` and `require __DIR__ . "/ClassLoader.php"` stopped resolving.

`Var` now records which spelling the source used, and `formatter.Source` refuses to produce output at all when the printer meets a node it cannot render. Round-tripping composer's whole `vendor/composer/` tree through `phpscript fmt` and running the result in phpscript and in php 8.4 produces identical output to the unformatted sources.

## Verification

Six fixtures cover the work: `static_members`, `closure_capture`, `unset_and_vivify`, `psr4_autoloader` (a class loader in the shape composer generates one), `preg_engines`, and `string_escapes`. Each was diffed against php 8.4 and matches byte for byte. The end-to-end proof is the two demos: `demos/dbadmin` and `demos/example` both boot through `vendor/autoload.php` and render their pages.
