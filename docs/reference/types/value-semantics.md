# Value semantics

| Value                 | phpscript              | PHP                  | Same?                         |
|-----------------------|------------------------|----------------------|-------------------------------|
| `null`, scalars       | immutable              | immutable            | yes                           |
| Objects               | handle                 | handle               | yes                           |
| Arrays                | handle                 | value, copy-on-write | **no**                        |
| `foreach ($a as &$v)` | write-back             | a reference          | yes, except the dangling `$v` |
| `&$x` anywhere else   | parsed, then discarded | a reference          | **no**                        |
| Binding outputs       | compile-time setter    | a reference          | same effect, different means  |

Assignment in phpscript binds a name to the value it was given. It never copies
the value, because there is no copy step anywhere in the runtime: a PHP variable
is a name in a flat per-frame table (`runner.Scope`) mapping to a Go value, and
assignment writes that map.

For scalars this is indistinguishable from PHP: you cannot mutate an `int` in
either language, only rebind the name. For objects it is also PHP's behaviour,
since a PHP object variable holds a handle. Arrays are where the two diverge.

`foreach` is the exception in the other direction: it implements both of PHP's
loop semantics, so `&` means there exactly what it means in PHP.

## Arrays are handles, not values

A PHP array is a value with copy-on-write: assigning one, or passing it to a
function, gives the other side an array it owns. phpscript's arrays are
`*model.Array` pointers, so both sides hold the same array.

```php
$a = array(1);
$b = $a;
$b[] = 2;
echo count($a);                 // phpscript: 2   PHP: 1

function grow($arr) { $arr[] = "x"; }
grow($a);
echo count($a);                 // phpscript: 3   PHP: unchanged
```

Code that reads a shared array is unaffected, which is why this survives in
practice: most array arguments are read. Code that writes to an array it
believes it owns is not, and it fails silently, because nothing in the shape
of `$b = $a` says two names are now one array.

Copy explicitly where independence matters:

```php
$copy = array_merge(array(), $original);
```

## Chained assignment allocates once per name

A chain that ends in an array literal is split by the parser into one assignment
per name, so each name gets an allocation of its own:

```php
$inlines = $blocks = array();   // parsed as: $blocks = array(); $inlines = array();
```

The literal says what to allocate rather than naming something already
allocated, so allocating once per name is what PHP's copy amounts to here, and
`$inlines[$k] = ...` no longer writes into `$blocks`. `phpscript fmt` prints the
split form, which is the statement's meaning written out. The order is PHP's,
right to left, which is observable when the targets overlap:

```php
$r['k'] = $r = array();         // clears $r, then puts an array under "k"
```

The split needs to evaluate the literal once per name, so it applies only when
doing that cannot be noticed: `array()`, `array(1, 2)`, `['k' => $v]`. A chain
ending in anything else keeps its old meaning and `phpscript lint` reports it:

```php
$a = $b = array(next($rows));   // chained assignment binds one value to several names
$dba = $dbb = new Database();   // chained assignment binds one value to several names
$m = $n = $rows;                // chained assignment binds one value to several names
```

Splitting the first would advance the array pointer twice. The second and third
are handles the two names really do share -- PHP shares the object too, and
`$rows` is the same array under a second name -- so the finding is a question
about the code rather than a divergence to repair.

A chain that ends in a scalar literal is neither split nor reported: a string or
a number has no interior for two names to share.

```php
$r['y'] = $r['m'] = $r['d'] = '00';   // no finding
```

A `for` clause holds one statement for its init and one for its post, so a chain
written there is not split and is reported instead.

## foreach binds a copy, or the element

`foreach ($a as $v)` gives the body a copy of each element and
`foreach ($a as &$v)` gives it the element itself, as in PHP:

```php
$rows = array(array("n" => 1));

foreach ($rows as $row)  { $row["n"] = 99; }    // $rows unchanged
foreach ($rows as &$ref) { $ref["n"] = 99; }    // $rows[0]["n"] is 99
unset($ref);
```

Neither is done with a reference, because there are none to use. A by-reference
loop writes the loop variable back into the element after each iteration,
before `break`, `continue` or `return` is acted on, so a write made just before
leaving the body is kept, as it would be in PHP. A by-value loop copies the
element on the way in, and only when the body actually assigns through the loop
variable: the copy is what makes the difference observable, so a read-only loop
does not pay for one. Nested arrays are copied too, since they are values in
PHP as well; objects inside are not, because they are handles in both.

Only a script-owned array is written back to. A native Go collection returned by
a binding belongs to the host, so a by-reference loop over one reads it without
writing to it.

One PHP behaviour is deliberately not reproduced. In PHP the loop variable of a
by-reference loop is still a reference to the last element after the loop ends,
so a second loop reusing the name overwrites it:

```php
$a = array(1, 2, 3);
foreach ($a as &$v) {}
foreach ($a as $v) {}
echo implode(",", $a);            // phpscript: 1,2,3    PHP: 1,2,2
```

`unset($v)` after the loop is the PHP idiom that avoids it, and it remains good
practice, but the trap itself needs a live reference to spring, and phpscript
has none.

## `&` elsewhere does not create a reference

Outside `foreach` there is nothing for `&` to do. Nothing in the value model can
stand for "the storage another name uses", so it is consumed by the parser
wherever PHP allows it and then discarded. Source written for PHP still parses,
and every spelling binds by value:

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

To hand a value back, return it:

```php
function bump($n) { return $n + 1; }
$x = bump($x);
```

## Output parameters

One write-back path does work, and it is the one PHP's own standard library
leans on, the `$matches` argument of `preg_match`:

```php
preg_match_all("/(\d)/", "a1b2", $matches);
echo implode(",", $matches[1]);      // 1,2
```

It works because it is arranged at compile time rather than in the value model.
`byRefArgs` in [transpile.go](../../../runner/transpile.go) lists the argument
positions that are outputs, per function name. When the transpiler emits a call
to one of them and the argument at that position is a plain variable, it emits a
setter in place of the variable's value:

```text
preg_match_all($p, $s, $matches)   ->   preg_match_all($p, $s, __ref("matches"))
```

`__ref` is `Runtime.helperRef`, which returns a `func(any)` closed over the
calling scope. The Go shim receives that closure as its trailing argument and
calls it with whatever the variable should hold; the closure writes the name
back into the frame. It captures the scope by value rather than through the
evaluation's scope reference, so a shim may call the setter after the expression
that produced it has finished.

Two things follow from doing this at compile time. An argument that is not a
plain variable, such as `preg_match($p, $s, $rows["m"])`, is passed by value like
any other expression, because there is no name to write back to. And the table is a
package-level variable in `runner` rather than part of the host API, so a
binding outside the standard library cannot declare an output parameter; one
that needs to hand several values back should return a collection instead. See
[Bindings](../extensions/bindings.md).
