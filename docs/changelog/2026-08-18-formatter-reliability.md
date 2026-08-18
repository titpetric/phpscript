# The formatter stopped deleting code it could not print

**2026-08-18.** `phpscript fmt` rewrites a file in place, so everything the printer cannot see is deleted from it. The parser keeps what the runtime needs and drops the rest, and the rest turned out to be most of what a reader of the file wants: comments, imports, type hints, the quoting of a string literal. Formatting `demos/dbadmin` removed all four.

```php
// before formatting
declare(strict_types=1);

namespace App;

use MiniTPL\Compiler;

class Loader {
	/** @var string the vendor directory */
	private ?string $dir = null;

	public function register(string $prefix): void {
		// resolve the prefix first
		$html = '<span class="badge">OK</span>';
	}
}
```

```php
// after formatting, before this change
namespace App;

class Loader {
	private $dir = null;

	public function register($prefix) {
		$html = "<span class=\"badge\">OK</span>";
	}
}
```

The file no longer stated its imports, its properties and parameters lost their types, and the string literal was rewritten into a double-quoted one, which in real PHP would have interpolated a `$` had one been in it. Everything above is kept now, and the file formats back to itself.

## Comments are placed by line

The AST holds no comments, and the printer knew two places to put them back: the file header, and the line above a class or function declaration. Every other comment was deleted, including the note above a property, the explanation inside a loop, and the reason on the end of a line.

The two special cases are gone. The formatter now consumes the comment stream of the source in order and writes a comment above the first statement that starts below it, or appends it to the line it was written on. Placing them needs positions the AST did not carry, so the parser records the line of the namespace declaration, of `else`, `catch` and `finally`, and of a `case` arm. Blank lines follow from the same source lines: a blank line above a statement is kept because the author wrote it, not because the two statements are far apart, which was wrong as soon as a comment stood between them.

## try/catch produced a file php refuses to parse

The runtime has no exception hierarchy, so the parser consumed the type filter of a catch clause and dropped it. The printer then wrote `} catch ($e) {`, which phpscript accepts and PHP does not, so every formatted file with a try/catch failed `php -l`. The filter is kept now, and a qualified `catch (\RuntimeException $e)`, which the parser rejected outright, parses.

## Formatting the same file twice gave two results

`break` and `continue` were empty structs. Go gives every zero-sized allocation the same address, so all of them were one key in the source-span map and the last one parsed set the line for every other. The formatter reads those spans to keep an author's blank lines, so it inserted a blank line above unrelated `break` statements, and a second formatting pass moved it again. Both statements carry their line now, which also gives them an address of their own.

## Arrays and class members

An array literal with more than two key/value pairs is printed one entry per line, indented one level, with a trailing comma, and so is one that does not fit in 100 columns from its own indent (issue #9). A list of values without keys stays on one line unless it is too wide. Class members are printed as constants, then properties, then methods, and the blank lines between them are the author's, not the printer's (issue #8). String literals keep the quoting they were written with (issue #10).

## A file it cannot format no longer fails the run

A directory of PHP holds code phpscript does not support: a PHPUnit test case that extends a base class, `E_ALL ^ E_NOTICE`, a `??`. The command stopped at the first such file, so the files below it in the walk were never formatted. Those files are reported and left alone now, and the walk continues:

```
$ phpscript fmt ./vendor/...
vendor/titpetric/minitpl/code/MiniTPL/Compiler.php
skipped vendor/titpetric/minitpl/test/bootstrap.php: line 3: unexpected character '^'
```

Before any file is rewritten, its formatted output has to parse and formatting it again has to produce the same text. A file that fails either check is skipped rather than written, so a printer defect that only shows on the second pass cannot replace working code. `phpscript fmt -l` runs the same formatting and reports what it would rewrite, writing nothing.

`formatter/testdata` holds the cases as input and expected output; the test that reads them also asserts both checks for every case. Every PHP file in the repository was formatted and passed to `php -l` while this was written, which is how the catch clause defect was found.
