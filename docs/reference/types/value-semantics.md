# Value semantics

| Value                 | phpscript              | PHP                  | Same?                         |
|-----------------------|------------------------|----------------------|-------------------------------|
| `null`, scalars       | immutable              | immutable            | yes                           |
| Objects               | handle                 | handle               | yes                           |
| Arrays                | handle                 | value, copy-on-write | **no**                        |
| `foreach ($a as &$v)` | write-back             | a reference          | yes, except the dangling `$v` |
| `&$x` anywhere else   | parsed, then discarded | a reference          | **no**                        |
| Binding outputs       | compile-time setter    | a reference          | same effect, different means  |

Assignment in phpscript binds a name to the value on the right. It never copies the value, because there is no copy step anywhere in the runtime: a PHP variable is a name in a flat per-frame table (`runner.Scope`) mapping to a Go value, and assignment writes that map.

For scalars this is indistinguishable from PHP: you cannot mutate an `int` in either language, only rebind the name. For objects the behaviour is PHP's too, since a PHP object variable holds a handle. Arrays are where the two diverge.

`foreach` is the exception in the other direction: it implements both of PHP's loop semantics and `&` means there exactly what it means in PHP.

## Arrays are handles, not values

A PHP array is a value with copy-on-write: assigning one, or passing it to a function, leaves the other side holding an array it owns. phpscript's arrays are `*model.Array` pointers and both sides hold the same array.

```php
$a = array(1);
$b = $a;
$b[] = 2;
echo count($a);                 // phpscript: 2   PHP: 1

function grow($arr) { $arr[] = "x"; }
grow($a);
echo count($a);                 // phpscript: 3   PHP: unchanged
```

Code that only reads a shared array behaves the same in both and this survives in practice: most calls only read an array argument. Code that writes to an array it holds as its own diverges, and it fails silently, because nothing in the shape of `$b = $a` says two names are now one array.

Copy explicitly where independence matters:

```php
$copy = array_merge(array(), $original);
```

## Chained assignment allocates once per name

The parser splits a chain that ends in an array literal into one assignment per name and each name gets an allocation of its own:

```php
$inlines = $blocks = array();   // parsed as: $blocks = array(); $inlines = array();
```

The literal says what to build, and names nothing already built and building once per name is what PHP's copy amounts to here and `$inlines[$k] = ...` leaves `$blocks` alone. `phpscript fmt` prints the split form, which is the statement's meaning written out. The order is PHP's, right to left, which is observable when the targets overlap:

```php
$r['k'] = $r = array();         // clears $r, then puts an array under "k"
```

The split evaluates the literal once per name and it applies only where a script cannot see the difference: `array()`, `array(1, 2)`, `['k' => $v]`. A chain ending in anything else keeps its old meaning and `phpscript lint` reports it:

```php
$a = $b = array(next($rows));   // chained assignment binds one value to several names
$dba = $dbb = new Database();   // chained assignment binds one value to several names
$m = $n = $rows;                // chained assignment binds one value to several names
```

Splitting the first would advance the array pointer twice. The second and third are handles the two names really do share -- PHP shares the object too, and `$rows` is the same array under a second name -- so the finding is a question about the code, and no divergence to repair.

A chain that ends in a scalar literal is neither split nor reported: a string or a number has no interior for two names to share.

```php
$r['y'] = $r['m'] = $r['d'] = '00';   // no finding
```

A `for` clause holds one statement for its init and one for its post and the split skips a chain written there and `phpscript lint` reports it instead.

## foreach binds a copy, or the element

`foreach ($a as $v)` binds a copy of each element and `foreach ($a as &$v)` binds the element itself, as in PHP:

```php
$rows = array(array("n" => 1));

foreach ($rows as $row)  { $row["n"] = 99; }    // $rows unchanged
foreach ($rows as &$ref) { $ref["n"] = 99; }    // $rows[0]["n"] is 99
unset($ref);
```

Neither uses a reference, because there are none to use. A by-reference loop writes the loop variable back into the element after each iteration, before it acts on `break`, `continue` or `return` and a write made just before leaving the body survives, as it would in PHP. A by-value loop copies the element on the way in, and only when the body actually assigns through the loop variable: the copy is what makes the difference observable and a read-only loop does not pay for one. The copy descends into nested arrays, since they are values in PHP as well, and stops at an object, because an object is a handle in both.

The loop writes back only to a script-owned array. A native Go collection returned by a binding belongs to the host and a by-reference loop over one reads it without writing to it.

One PHP behaviour is not reproduced. In PHP the loop variable of a by-reference loop is still a reference to the last element after the loop ends and a second loop reusing the name overwrites it:

```php
$a = array(1, 2, 3);
foreach ($a as &$v) {}
foreach ($a as $v) {}
echo implode(",", $a);            // phpscript: 1,2,3    PHP: 1,2,2
```

`unset($v)` after the loop is the PHP idiom that avoids it, and it remains good practice, but the trap itself needs a live reference to spring, and phpscript has none.

## `&` elsewhere does not create a reference

Outside `foreach`, `&` has no work to do. No part of the value model can stand for "the storage another name uses" and the parser consumes it wherever PHP allows it and then discards it. Source written for PHP still parses, and every spelling binds by value:

```php
$a = 1;
$b = &$a;                         // parsed; $b is a copy
$b = 2;
echo $a;                          // phpscript: 1    PHP: 2

function bump(&$n) { $n = $n + 1; }
$x = 1;
bump($x);
echo $x;                          // phpscript: 1    PHP: 2

$c = 0;
$f = function () use (&$c) { $c = 99; };
$f();
echo $c;                          // phpscript: 0    PHP: 99
```

Reference returns (`function &f()`) are unavailable.

To send a value back, return it:

```php
function bump($n) { return $n + 1; }
$x = bump($x);
```

## Output parameters

One write-back path does work, the one PHP's own standard library leans on, the `$matches` argument of `preg_match`:

```php
preg_match_all("/(\d)/", "a1b2", $matches);
echo implode(",", $matches[1]);      // 1,2
```

It works because the compiler arranges it at compile time, outside the value model. `byRefArgs` in [model/byref.go](../../../model/byref.go) lists the argument positions that are outputs, per function name, and both engines read it from there. When an engine emits a call to one of them and the argument at that position is a plain variable, a setter goes in place of the variable's value:

```text
preg_match_all($p, $s, $matches)   ->   preg_match_all($p, $s, __ref("matches"))
```

`__ref` is `Runtime.helperRef`, which returns a `func(any)` closed over the calling scope. The Go shim receives that closure as its trailing argument and calls it with whatever the variable should hold; the closure writes the name back into the frame. It captures the scope by value, not through the evaluation's scope reference and a shim may call the setter after the expression that produced it has finished.

Two things follow from doing this at compile time. An argument that is not a plain variable, such as `preg_match($p, $s, $rows["m"])`, arrives by value like any other expression, because there is no name to write back to. And the table is a package-level variable in `model` and not part of the host API and a binding outside the standard library declares no output parameter; one with more than one value to send back returns a collection. See [Bindings](../extensions/bindings.md).
