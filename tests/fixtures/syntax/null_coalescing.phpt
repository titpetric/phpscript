name: null coalescing
description: >
  Covers ?? and ??=: null, unset, false, 0 and the empty string on the left,
  missing keys and properties at any depth, right associativity, precedence
  against the ternary and against concatenation, a right side that runs only
  when needed, a call on the left that runs once, an assignment on the right, a
  static property and a class constant, the levels ??= builds under a missing
  key, a string offset no byte sits at, which isset, empty and ?? all read as
  unset, and a ??= target whose index or receiver runs once.
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

// A static property, a class constant, and a static the class declared null.
class Stat { public static $v = null; public static $w = 4; const K = 9; }
var_dump(Stat::$v ?? 'd', Stat::$w ?? 'd', Stat::K ?? 'd');
Stat::$v ??= 'set';
Stat::$v ??= 'again';
echo Stat::$v, "\n";

// `??=` builds the levels a missing key needs.
$n = [];
$n['a']['b'] ??= 1;
$n['a']['c'] ??= 2;
$n['x']['y']['z'] ??= 'q';
echo json_encode($n), "\n";

// Concatenation binds tighter, and the left side is the whole string.
$u = null;
var_dump("a" . $u ?? "b");

// A call inside the index, and a call to the left of it.
$list = [1, 2, 3];
echo $list[count($list) - 1] ?? 'last', "\n";
class Box { public $d = ['k' => 'v']; public function all() { return $this->d; } }
$box = new Box;
echo $box->all()['k'] ?? 'd', $box->all()['q'] ?? 'd', "\n";

// A string offset no byte sits at is unset, the way a missing key is, and
// `isset`, `empty` and `??` all read it that way.
$s = "abc";
var_dump(isset($s[1]), isset($s[10]), isset($s["x"]), isset($s[-1]), isset($s[-9]));
echo $s[1] ?? "none", $s[10] ?? "none", $s["x"] ?? "none", $s[-1] ?? "none", "\n";
var_dump(empty($s[10]), empty($s[1]));

// `??=` leaves a target that holds something alone.
$s[0] ??= "z";
var_dump($s);

// The target runs once, whether the store happens or not: a call in the index,
// an increment, and the receiver of a property.
class Calls { public static $n = 0; }
function key_once() { Calls::$n += 1; return 'k'; }
$held = ['k' => 'held'];
$held[key_once()] ??= 'stored';
$fresh = [];
$fresh[key_once()] ??= 'stored';
echo $held['k'], $fresh['k'], " calls: ", Calls::$n, "\n";
$t = [1 => 'x'];
$i = 1;
$t[$i++] ??= 'first';
$t[$i++] ??= 'second';
echo json_encode($t), " i=", $i, "\n";
class Side { public $p = null; public $q = 'set'; public static $one = null; }
function side() { Calls::$n += 1; if (Side::$one === null) { Side::$one = new Side; } return Side::$one; }
Calls::$n = 0;
side()->p ??= 'filled';
side()->q ??= 'ignored';
echo side()->p, side()->q, " calls: ", Calls::$n, "\n";

// The same target shapes in a loop, inside a function and under a property.
Calls::$n = 0;
$seen = [];
foreach (['x', 'y', 'x'] as $letter) {
	$seen[$letter] ??= 0;
	$seen[$letter] += 1;
}
echo $seen['x'], $seen['y'], "\n";
function fill($rows) { $rows[key_once()] ??= 'filled'; return $rows['k']; }
echo fill([]), fill(['k' => 'held']), "\n";
class Tree { public $kids = []; }
$tree = new Tree;
$tree->kids[key_once()][key_once()] ??= 'leaf';
echo $tree->kids['k']['k'], " calls: ", Calls::$n, "\n";
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
string(1) "d"
int(4)
int(9)
set
{"a":{"b":1,"c":2},"x":{"y":{"z":"q"}}}
string(1) "a"
3
vd
bool(true)
bool(false)
bool(false)
bool(true)
bool(false)
bnonenonec
bool(true)
bool(false)
string(3) "abc"
heldstored calls: 2
{"1":"x","2":"second"} i=3
filledset calls: 4
21
filledheld
leaf calls: 4
