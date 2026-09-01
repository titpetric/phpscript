name: a foreach survives the calls made inside it
description: >
  The compiler numbers foreach iterators per function, so a callee stands on
  the numbers its caller is using and a recursive call reuses them exactly.
  The iterator state used to be one map for the whole run, so the callee's
  opIterInit overwrote the caller's loop and its opIterClose deleted it: the
  outer loop ended after the first element that made a call. Saving the map on
  the call frame, the way locals are saved, is what keeps the loops apart. A
  recursive directory walk is the shape this was found in, so the fixture walks
  a nested array the same way.
---
<?php

// The recursive case: the inner call runs its own foreach over the same
// iterator number the outer loop is standing on.
function walk(array $node, string $indent = ""): void
{
    foreach ($node as $key => $value) {
        if (is_array($value)) {
            echo $indent, $key, "/\n";
            walk($value, $indent . "    ");
            continue;
        }
        echo $indent, $key, "=", $value, "\n";
    }
}

walk([
    "a" => 1,
    "sub" => ["b" => 2, "deeper" => ["c" => 3, "d" => 4], "e" => 5],
    "f" => 6,
]);

// The non-recursive case: a different function, looping over its own array,
// called from inside a loop.
function inner(int $n): int
{
    $total = 0;
    foreach ([1, 2, 3] as $x) {
        $total += $x * $n;
    }
    return $total;
}

foreach ([1, 2, 3] as $n) {
    echo "inner(", $n, ")=", inner($n), "\n";
}

// A method called from inside a loop is the same frame problem.
class Adder
{
    public function sum(array $xs): int
    {
        $total = 0;
        foreach ($xs as $x) {
            $total += $x;
        }
        return $total;
    }
}

$adder = new Adder();
foreach ([[1, 2], [3, 4], [5, 6]] as $pair) {
    echo "sum=", $adder->sum($pair), "\n";
}

// A call that throws unwinds past the callee's loop and leaves the caller's
// standing, which is the same bookkeeping on the exception path.
function boom(array $xs): void
{
    foreach ($xs as $x) {
        throw new RuntimeException("at " . $x);
    }
}

foreach ([7, 8] as $n) {
    try {
        boom([$n]);
    } catch (RuntimeException $e) {
        echo "caught ", $e->getMessage(), "\n";
    }
}
?>
---
a=1
sub/
    b=2
    deeper/
        c=3
        d=4
    e=5
f=6
inner(1)=6
inner(2)=12
inner(3)=18
sum=3
sum=7
sum=11
caught at 7
caught at 8
