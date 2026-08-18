# Value semantics documented, chained assignment linted

**2026-08-18.** Assignment in phpscript binds a name to the value it was given and never copies it, because there is no copy step anywhere in the runtime. For scalars and objects that is exactly PHP. For arrays it is not, and for `&` it is a promise the runtime does not keep. All of it was previously spread across three compatibility tables as one-line "not implemented" rows that said what is missing without saying what happens instead. [Value semantics](../reference/types/value-semantics.md) is the missing page.

`&` parses and is then discarded, so every spelling of it binds by value:

```php
$a = 1;   $b = &$a;               $b = 2;   echo $a;   // phpscript: 1   PHP: 2
$x = 1;   function bump(&$n) {...}  bump($x); echo $x;   // phpscript: 1   PHP: 2
$c = 0;   $f = function () use (&$c) {...};   $f();      // phpscript: 0   PHP: 99
```

## foreach got both of PHP's semantics

`foreach ($a as &$v)` used to be a parse error, and `foreach ($a as $v)` handed the body the element rather than a copy of it, which for an array element meant `$v["k"] = 1` edited the source where PHP leaves it alone. Both spellings now mean what they mean in PHP:

```php
$rows = array(array("n" => 1));
foreach ($rows as $row)  { $row["n"] = 99; }    // $rows unchanged
foreach ($rows as &$ref) { $ref["n"] = 99; }    // $rows[0]["n"] is 99
```

Neither uses a reference, because there are none. A by-reference loop writes the loop variable back into the element after each iteration, before `break`, `continue` or `return` is acted on, so a write made just before leaving the body survives. A by-value loop copies the element on the way in, but only when the body assigns through the loop variable, since that is the only way the copy can be observed. `model.AssignsTo` answers that once per loop rather than once per iteration, so a read-only loop still costs nothing.

The bytecode engine implements the same thing rather than falling back to the interpreter: two new opcodes (`opCopyValue`, `opIterSet`), an iterator that remembers the container and current key it came from, and write-back emitted at each point control leaves the body, including before a `return`, which unwinds every enclosing by-reference loop at once. `tests/fixtures/pass_by_ref.phpt` runs under both engines against one set of expectations, which is what holds them together. Implementing it also needed `unset` in the bytecode engine, which had been an interpreter-only statement.

One PHP behaviour is deliberately not reproduced: PHP leaves the loop variable a live reference to the last element after the loop, so a second loop reusing the name overwrites it (`1,2,3` becomes `1,2,2`). That trap needs a reference to spring.

One by-reference path does work, and the chapter documents it because it is not obvious from either side: `byRefArgs` in [transpile.go](../../runner/transpile.go) names the argument positions that are outputs, per function, and the transpiler emits a setter closure in place of a plain variable at one of those positions. That is how `preg_match($p, $s, $matches)` fills `$matches`. It is arranged at compile time rather than in the value model, which is why an argument that is not a plain variable falls back to by-value, and why the table is not part of the host binding API.

## The opposite problem

`&` promises sharing and does not deliver it. A plain array assignment delivers sharing nobody asked for:

```php
$a = array(1);
$b = $a;
$b[] = 2;
echo count($a);          // phpscript: 2   PHP: 1
```

PHP arrays are values with copy-on-write; phpscript's are `*model.Array` handles. Code that reads a shared array is unaffected. Code that writes to a copy it believes it owns is not, and it fails silently, because nothing about the shape says two names are involved.

## The lint rule

`phpscript lint` now reports the shape this most often hides behind:

```php
$inlines = $blocks = array();     // chained assignment binds one value to several names
```

That line is from minitpl's own compiler, and it is why two of its twelve test templates compile differently under phpscript than under php 8.4: `$inlines[$name] = ...` also writes into `$blocks`, so every `{inline}` is treated as a `{block}` as well.

The rule fires on an `*model.Assign` whose value is an `*model.AssignExpr`: the direct `$a = $b = value` chain, in any scope, once per statement rather than once per link. It does not try to tell an array chain from a scalar one: the type of the right-hand side is not known until the statement runs, and for objects and scalars the chain is only a readability question. Extending the statement walker for it also fixed a gap where `foreach` bodies and `for` init/post clauses were never visited by any rule.

## Test cleanup

`BenchmarkMinitpl`, `TestMinitplProfiles` and `BenchmarkFlatstackMinitplImportSwap` still read `tests/fixtures/test-minitpl.php`, deleted in 6584b83 when the vendored engine was dropped in favour of installing it with composer under `demos/`. They have been failing `go test -bench=.` ever since and are now removed; `tests/` depends on no composer package and no template engine.
