name: null coalescing
description: >
  Covers ?? and ??=: null, unset, false, 0 and the empty string on the left,
  missing keys and properties at any depth, right associativity, precedence
  against the ternary, a right side that runs only when needed, a call on the
  left that runs once, and an assignment on the right.
---
<?php

// The left side is used unless it is null or unset; false, 0 and "" stay.
$a = null;
echo $a ?? "null", "\n";
echo $undefined ?? "undef", "\n";
$f = false; $z = 0; $e = "";
var_dump($f ?? "x", $z ?? "x", $e ?? "x");

// Missing keys and properties, at any depth, fall through without a notice.
$m = ['k' => null, 'j' => 5, 'n' => ['d' => 7]];
echo $m['k'] ?? "nullk", $m['j'] ?? "none", $m['zz'] ?? "missing", "\n";
echo $m['n']['d'] ?? "no", $m['n']['q']['r'] ?? "deep", "\n";
class P { public $x = null; public $y = 3; public $o = null; }
$o = new P;
echo $o->x ?? "px", $o->y ?? "py", $o->w ?? "pw", $o->o->deep ?? "pd", "\n";

// Right associative, and tighter than the ternary.
echo $nothing ?? $m['k'] ?? "chain", "\n";
echo $undefined2 ?? 1 ? "t" : "f", "\n";
echo ($a ?? 0) + 1, "\n";
echo null ?? false ?? "never", "|\n";

// The right side runs only when the left is null.
function loud($v) { echo "eval $v\n"; return $v; }
echo loud("l") ?? loud("r"), "\n";
echo loud(null) ?? loud("r"), "\n";

// A call on the left runs once.
function pick() { echo "pick\n"; return ['k' => 'v']; }
echo pick()['k'] ?? "none", "\n";
echo pick()['q'] ?? "none", "\n";

// `??=` assigns only while the target is null or unset.
$a ??= 5;
$a ??= 9;
echo $a, "\n";
$m['k'] ??= "filled";
$m['new'] ??= "created";
$m['j'] ??= "kept";
echo $m['k'], $m['new'], $m['j'], "\n";
$o->x ??= "set";
$o->x ??= "again";
echo $o->x, "\n";
$u ??= loud("rhs");
$u ??= loud("skipped");
echo $u, "\n";
$chain = $none ??= $other ??= "deep";
echo $chain, $none, $other, "\n";

// An assignment on the right of `??` binds to the variable beside it.
$lazy ?? $lazy = loud("init");
$lazy ?? $lazy = loud("again");
echo $lazy, "\n";

// In an argument, an interpolation and a return.
function name($n) { return $n ?? "anon"; }
echo name(null), name("bob"), "\n";
echo "v=" . ($m['zz'] ?? "d") . "\n";
---
null
undef
bool(false)
int(0)
string(0) ""
nullk5missing
7deep
px3pwpd
chain
t
1
|
eval l
l
eval 
eval r
r
pick
v
pick
none
5
filledcreated5
set
eval rhs
rhs
deepdeepdeep
eval init
init
anonbob
v=d
