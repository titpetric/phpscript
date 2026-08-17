# Run composer's own autoloader

phpscript resolves composer projects today, but not by running composer's code.
The [composer](../../composer) package reads `composer.json` and
`vendor/composer/installed.json` and installs a Go class loader from the PSR-4,
PSR-0 and `files` entries it finds; `include "vendor/autoload.php"` is bound to
that implementation and composer's generated bootstrap is never parsed. See
[Composer support](../composer.md).

That shim is the right layer for the metadata, but it is not compatibility. It
cannot support `classmap` (composer only emits it as PHP), it will drift as
composer's format moves, and — more to the point — the reason it exists is that
the interpreter cannot run a fairly ordinary piece of PHP 5.3-era OO code. Every
package that ships a bootstrap of its own hits the same wall.

This issue tracks closing that gap in the runtime. The acceptance criteria are
the `.phpt` fixtures below: each one passes under PHP 8.4 today and fails under
phpscript today.

## Where it stands

The chain is `autoload.php` → `autoload_real.php` → `platform_check.php` +
`ClassLoader.php` + `autoload_static.php`. Every substantive file fails, and
all but one fail at **parse** time, before a statement runs:

| generated file                   | lines | first failure                                                   |
|----------------------------------|-------|-----------------------------------------------------------------|
| `autoload.php`                   | 22    | L22 `ComposerAutoloaderInit…::getLoader()` — static method call |
| `composer/autoload_real.php`     | 38    | L21 `self::$loader` — static property                           |
| `composer/autoload_static.php`   | 36    | L27 `getInitializer(ClassLoader $loader)` — typed parameter     |
| `composer/ClassLoader.php`       | 579   | L109 `self::initializeIncludeClosure()`                         |
| `composer/InstalledVersions.php` | 396   | L15 `use Composer\Autoload\ClassLoader;`                        |
| `composer/platform_check.php`    | 25    | runtime: `PHP_VERSION_ID >= 50600` → `<nil> >= int`             |
| `composer/autoload_psr4.php`     | 10    | parses and runs                                                 |
| `composer/installed.php`         | 32    | parses and runs                                                 |

The last two rows are why the shim is viable at all: composer's *data* is plain
`return array(...)`, and only its *machinery* is out of reach.

Reproduce with a project that has run `composer install`:

```bash
cd tests/fixtures
for f in vendor/autoload.php vendor/composer/*.php; do echo "== $f"; phpscript "$f"; done
```

## Three of these are silent

Most blockers are parse errors, which are safe: they stop. Three produce a wrong
answer with no error at all, and those are worth fixing on their own merits,
independently of composer — they will bite ordinary PHP too.

- **`array + array`** evaluates to an empty array. `ClassLoader::register()`
  prepends itself to the loader registry with it.
- **`$closure(args)`** — a closure held in a variable — evaluates to null
  without calling. This is `ClassLoader::loadClass`'s
  `$includeFile = self::$includeFile; $includeFile($file);`, the line that loads
  every class.
- **`PHP_VERSION_ID`** is undefined, so composer's version guard compares nil
  against an int. An undefined constant resolving to nil rather than raising is
  the general shape of this one.

## What is missing

Grouped roughly by how much runtime work each implies.

### Static class members

The largest gap: three of the six failing files stop here first, and a fourth
(`autoload_static.php`) is built on statics too. Needs static
property declarations and storage, `self::$x`, `static::$x`, `Class::$x` as both
read and write, and `Class::method()` / `self::method()` dispatch. The parser
currently only accepts `::CONST` after `::` and reports
`expected constant name after ::`.

### `use` imports in namespaced files

`use X\Y;` and `use X\Y as Z;`. phpscript enforces "namespaced files may only
declare symbols" and counts a `use` import as a statement, so
`InstalledVersions.php` cannot be parsed at all. The restriction itself is
reasonable — composer's files comply with it — the imports just need to be
classified as declarations.

### Parameter and return type declarations

`array $classMap`, `ClassLoader $loader`, `?Box $box`, `: int`. They can be
accepted and ignored, or accepted and enforced; either closes the parse error.

### First-class closures

- invoking a closure held in a variable or property: `$f($x)`, and `$f()` with
  no arguments (currently a parse error on its own)
- `use ($x)` capture by value and `use (&$x)` by reference
- `static function () {}`
- `\Closure::bind($fn, $scope, $class)` and the `::class` constant

`Closure::bind` is how `autoload_static.php` injects the prefix maps into the
loader's private properties and how `ClassLoader` builds its scope-isolated
include helper.

### Assignment inside a condition

`if ($file = $this->findFile($class))` and
`while (false !== $lastPos = strrpos($subPath, '\\'))`. The whole PSR-4 walk in
`findFileWithExtension` is built on the second form. Note that phpscript's own
`lint` deliberately flags this pattern — if that stays, the runtime still has to
parse and evaluate it, and lint can keep discouraging it in first-party code.

### Standard library and constants

Missing outright: `PHP_VERSION_ID`, `PHP_VERSION`, `PHP_SAPI`, `PHP_EOL`,
`PHP_INT_MAX`, `defined()`, `headers_sent()`, `ini_get()`, `strtr()`,
`strrpos()`, `spl_autoload_unregister()`, `unset()` on array elements,
`func_num_args()`, `stream_resolve_include_path()`, `apcu_fetch()`/`apcu_add()`.

`apcu_*` and `stream_resolve_include_path` are only reached when the loader is
configured for them, so stubs are enough. The rest are on the hot path.

### Already working

`spl_autoload_register` (including the 3-argument prepend form and
`array("Class", "method")` callables), `require_once`, `__DIR__`, `include`
returning a value, `RuntimeException` and try/catch, `isset($arr[$k])`,
`file_exists`, `strpos`, `substr`, `DIRECTORY_SEPARATOR`, `fwrite(STDERR, …)`,
and nested array literals.

## Acceptance criteria

Eleven fixtures. Every one has been checked against PHP 8.4.8: the expected
section is what stock PHP prints. Every one fails under phpscript at the time of
writing, with the error quoted above each.

Drop them into `tests/fixtures/` as work lands. They are not committed yet
precisely because they fail — adding them now would redden the suite.

<details>
<summary><code>static_members.phpt</code> — <code>line 8: expected constant name after ::</code></summary>

```
name: static class members
description: >
  Static properties and static methods, reached through self::, static:: and
  Class::. composer's autoload_real.php caches its loader in a private static
  property and is entered through a static method call.
---
<?php

class Counter {
	private static $count = 0;
	public static $label = "counter";

	public static function bump() {
		self::$count = self::$count + 1;
		return self::$count;
	}

	public static function total() {
		return static::$count;
	}
}

echo Counter::bump(), "\n";
echo Counter::bump(), "\n";
echo Counter::total(), "\n";
echo Counter::$label, "\n";

Counter::$label = "renamed";
echo Counter::$label, "\n";
?>
---
1
2
2
counter
renamed
```

</details>

<details>
<summary><code>use_imports.phpt</code> — <code>line 5: namespaced files may only declare symbols</code> (in the included file)</summary>

Needs a companion declaration file, `tests/fixtures/code/namespaced_helper.php`:

```php
<?php

namespace App\Support;

use RuntimeException as Boom;

class Helper
{
	public static function name()
	{
		return "helper";
	}

	public static function boom()
	{
		try {
			throw new Boom("bang");
		} catch (\Exception $e) {
			return "caught " . $e->getMessage();
		}
	}
}
```

```
name: use imports in a namespaced file
description: >
  A namespaced declaration file imports symbols with `use`, optionally aliased.
  composer's InstalledVersions.php opens with two of them, so the file cannot be
  parsed at all today: `use` is rejected as a statement in a namespaced file.
  Requires code/namespaced_helper.php.
---
<?php

include("code/namespaced_helper.php");

echo App\Support\Helper::name(), "\n";
echo \App\Support\Helper::boom(), "\n";
?>
---
helper
caught bang
```

</details>

<details>
<summary><code>typed_parameters.phpt</code> — <code>line 7: expected parameter $var</code></summary>

```
name: parameter type declarations
description: >
  Scalar, array, class and nullable parameter types, plus a return type.
  composer's ClassLoader declares `addClassMap(array $classMap)` and
  `getInitializer(ClassLoader $loader)`; today either signature is a parse
  error.
---
<?php

class Box {
	public $value = "boxed";
}

function take_array(array $items) {
	return count($items);
}

function take_object(Box $box) {
	return $box->value;
}

function take_string(string $s, $fallback = null) {
	return $s . ":" . ($fallback === null ? "none" : $fallback);
}

function take_nullable(?Box $box) {
	return $box === null ? "null" : $box->value;
}

function returns_int($a, $b): int {
	return $a + $b;
}

echo take_array(array(1, 2, 3)), "\n";
echo take_object(new Box), "\n";
echo take_string("s"), "\n";
echo take_nullable(null), "\n";
echo returns_int(2, 3), "\n";
?>
---
3
boxed
s:none
null
5
```

</details>

<details>
<summary><code>closures.phpt</code> — <code>line 6: unexpected token 7(",")@6</code></summary>

```
name: closures beyond the inline callback
description: >
  A closure stored in a variable and then invoked, `use` capture by value and by
  reference, and a static closure. ClassLoader::loadClass reads its include
  helper out of a static property and calls it: `$includeFile($file)`. Today
  invoking a closure held in a variable silently evaluates to null, and the
  no-argument form `$f()` is a parse error.
---
<?php

$twice = function ($x) {
	return $x . $x;
};
echo $twice("ab"), "\n";

$greeting = "hi";
$greet = function ($who) use ($greeting) {
	return $greeting . " " . $who;
};
echo $greet("there"), "\n";

$calls = 0;
$count = function () use (&$calls) {
	$calls = $calls + 1;
};
$count();
$count();
echo $calls, "\n";

$plain = static function ($v) {
	return "static:" . $v;
};
echo $plain("s"), "\n";

$noargs = function () {
	return "noargs";
};
echo $noargs(), "\n";
?>
---
abab
hi there
2
static:s
noargs
```

</details>

<details>
<summary><code>closure_bind.phpt</code> — <code>line 16: expected ")", got 7(",")@16</code></summary>

```
name: Closure::bind and the ::class constant
description: >
  composer's autoload_static.php injects its prefix maps with
  `\Closure::bind($fn, null, ClassLoader::class)`, which needs both the
  Closure class and the ::class constant. `::class` currently reports
  "undefined class constant".
---
<?php

class Loader {
	private $prefixes = array();

	public function dump() {
		return implode(",", $this->prefixes);
	}
}

echo Loader::class, "\n";

$loader = new Loader;
$init = \Closure::bind(function () use ($loader) {
	$loader->prefixes = array("App\\", "Acme\\");
}, null, Loader::class);
$init();

echo $loader->dump(), "\n";

$unbound = \Closure::bind(static function ($file) {
	return "included:" . $file;
}, null, null);
echo $unbound("x.php"), "\n";
?>
---
Loader
App\,Acme\
included:x.php
```

</details>

<details>
<summary><code>assignment_in_condition.phpt</code> — <code>line 12: expected ")", got 7("=")@12</code></summary>

```
name: assignment inside a condition
description: >
  `if ($file = expr())` and `while (false !== $pos = strrpos(...))` are how
  ClassLoader::findFileWithExtension walks a class name down to its PSR-4
  prefix. Both are parse errors today.
---
<?php

function lookup($key) {
	$table = array("a" => "found-a", "b" => "found-b");
	return isset($table[$key]) ? $table[$key] : false;
}

if ($hit = lookup("a")) {
	echo $hit, "\n";
}

if (!$miss = lookup("zz")) {
	echo "miss\n";
}

$path = "App\\Support\\Deep\\Thing";
while (false !== $pos = strrpos($path, "\\")) {
	$path = substr($path, 0, $pos);
	echo $path, "\n";
}
?>
---
found-a
miss
App\Support\Deep
App\Support
App
```

</details>

<details>
<summary><code>array_union.phpt</code> — <strong>silent</strong>: <code>output mismatch: got "0\n,,\n\n"</code></summary>

```
name: array union operator
description: >
  `$a + $b` keeps every entry of $a and adds the keys of $b that $a does not
  have. ClassLoader::register prepends itself to the loader registry with it.
  Today the result is an empty array and no error is raised, so the bug is
  silent.
---
<?php

$left = array("x" => 1, "y" => 2);
$right = array("y" => 99, "z" => 3);
$union = $left + $right;

echo count($union), "\n";
echo $union["x"], ",", $union["y"], ",", $union["z"], "\n";

$lists = array("a", "b") + array("c", "d", "e");
echo implode(",", $lists), "\n";

$acc = array("first" => 1);
$acc = array("zero" => 0) + $acc;
echo implode(",", array_keys($acc)), "\n";
?>
---
3
1,2,3
a,b,e
zero,first
```

</details>

<details>
<summary><code>unset_array_key.phpt</code> — <code>unset(__index(v_map, "b")): cannot call nil</code></summary>

```
name: unset on array elements and variables
description: >
  ClassLoader::unregister drops itself from the registry with
  `unset(self::$registeredLoaders[$this->vendorDir])`. unset() on an array
  element is currently an undefined function call.
---
<?php

$map = array("a" => 1, "b" => 2, "c" => 3);
unset($map["b"]);
echo implode(",", array_keys($map)), "\n";
echo count($map), "\n";
echo isset($map["b"]) ? "still set" : "gone", "\n";

$nested = array("outer" => array("inner" => 1, "other" => 2));
unset($nested["outer"]["inner"]);
echo implode(",", array_keys($nested["outer"])), "\n";

$scalar = "value";
unset($scalar);
echo isset($scalar) ? "still set" : "gone", "\n";
?>
---
a,c
2
gone
other
gone
```

</details>

<details>
<summary><code>php_version_constants.phpt</code> — <code>(v_PHP_VERSION_ID) >= (50600): &lt;nil&gt; &gt;= int</code></summary>

```
name: predefined PHP version and environment constants
description: >
  composer guards its bootstrap with `PHP_VERSION_ID < 50600` and reports
  through PHP_EOL and PHP_SAPI. All of these are undefined today, so the guard
  compares nil against an int and platform_check.php aborts.
---
<?php

echo PHP_VERSION_ID >= 50600 ? "supported" : "too old", "\n";
echo is_int(PHP_VERSION_ID) ? "int" : "not int", "\n";
echo strlen(PHP_VERSION) > 0 ? "has version" : "no version", "\n";
echo PHP_EOL === "\n" ? "eol ok" : "eol wrong", "\n";
echo strlen(PHP_SAPI) > 0 ? "has sapi" : "no sapi", "\n";
echo defined("PHP_VERSION_ID") ? "defined" : "undefined", "\n";
echo defined("HHVM_VERSION") ? "defined" : "undefined", "\n";
echo PHP_INT_MAX > 0 ? "int max ok" : "int max wrong", "\n";
?>
---
supported
int
has version
eol ok
has sapi
defined
undefined
int max ok
```

</details>

<details>
<summary><code>string_lookup_functions.phpt</code> — <code>strtr(...): cannot call nil</code></summary>

```
name: strtr and strrpos
description: >
  ClassLoader turns a class name into a path with
  `strtr($class, '\\', DIRECTORY_SEPARATOR)` and walks its namespace with
  strrpos. Neither function is registered today.
---
<?php

echo strtr("App\\Support\\Thing", "\\", "/"), "\n";
echo strtr("abc", array("a" => "1", "bc" => "2")), "\n";

var_dump(strrpos("App\\Support\\Thing", "\\"));
var_dump(strrpos("no-separator", "\\"));
echo strrpos("aXbXc", "X"), "\n";
?>
---
App/Support/Thing
12
int(11)
bool(false)
3
```

Note this one also needs `var_dump`, which is not implemented either.

</details>

<details>
<summary><code>spl_autoload_unregister.phpt</code> — <code>spl_autoload_unregister(v_first): cannot call nil</code></summary>

```
name: spl_autoload_unregister
description: >
  composer's autoload_real.php registers a temporary loader that pulls in
  ClassLoader.php, then unregisters it. Without unregister the bootstrap loader
  stays in the queue for the life of the request.
---
<?php

$log = "";
$first = function ($class) {
	echo "first:" . $class . "\n";
};
$second = function ($class) {
	echo "second:" . $class . "\n";
};

spl_autoload_register($first);
spl_autoload_register($second);

class_exists("Missing\\One", true);

spl_autoload_unregister($first);

class_exists("Missing\\Two", true);
?>
---
first:Missing\One
second:Missing\One
second:Missing\Two
```

</details>

## Definition of done

All eleven fixtures pass, and then, from a project that has run
`composer install`, every generated file parses and runs:

```bash
cd tests/fixtures
for f in vendor/autoload.php vendor/composer/*.php; do echo "== $f"; phpscript "$f"; done
```

with `tests/fixtures/test-minitpl.php` still rendering `Hello, phpscript!` while
composer's real `ClassLoader` does the loading. At that point `composer.Register`
can drop
its `RegisterInclude` binding and let `vendor/autoload.php` load normally,
keeping the Go loader only as a fast path — or delete it. `classmap` support
comes for free at that point, since the classmap is generated PHP.

Worth landing in stages; the fixtures are independent of one another and every
one of them is a real PHP feature, not a composer quirk. The three silent cases
should go first.
