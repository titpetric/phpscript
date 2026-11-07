name: assignment as a value
description: >
  An assignment used as a value over every lvalue shape: an element, a
  property, a static property, a nested element and a chain, each answering
  what it stored, plus the compound operators, which answer the updated value.
  The target resolves before the value, the order a key or a receiver with a
  side effect reads.
---
<?php

// An assignment used as a value, over each lvalue shape.
$a = [];
$x = $a['k'] = 1;
echo "index: ", $x, $a['k'], "\n";

$b = [];
$y = $b['k'] ??= 2;
$z = $b['k'] ??= 9;
echo "coalesce: ", $y, $z, $b['k'], "\n";

class Holder { public $p = null; public static $s = null; }
$h = new Holder;
$w = $h->p = 3;
$v = $h->p ??= 9;
echo "prop: ", $w, $v, $h->p, "\n";

$t = Holder::$s = 4;
$u = Holder::$s ??= 9;
echo "static: ", $t, $u, Holder::$s, "\n";

// A chain of two element targets, and one in a condition.
$c = [];
$d = [];
$c['x'] ??= $d['y'] ??= 'deep';
echo "chain: ", $c['x'], $d['y'], "\n";

$rows = [];
if (($rows['n'] = 5) > 4) {
	echo "cond: ", $rows['n'], "\n";
}

$cache = [];
echo "arg: ", strlen($cache['s'] ??= "abcd"), $cache['s'], "\n";

// A compound operator answers the value the target holds afterwards.
$counts = ['hits' => 1];
$after = $counts['hits'] += 2;
echo "compound: ", $after, $counts['hits'], "\n";

// A nested element, and a property of an element.
$deep = [];
$got = $deep['a']['b'] ??= 'leaf';
echo "deep: ", $got, $deep['a']['b'], "\n";

$list = [new Holder];
$prop = $list[0]->p ??= 'set';
echo "elemprop: ", $prop, $list[0]->p, "\n";

// The target resolves before the value: the key runs first.
function t($tag, $value = null)
{
	echo $tag;
	return $value;
}

$order = [];
$order[t("K", "k")] = t("V", 1);
echo "\n";

$step = ['j' => 1];
$stepped = $step[t("K2", "j")] += t("V2", 2);
echo "|", $stepped, $step['j'], "\n";

class Text { public $p = "a"; }
$text = new Text;
$joined = t("R", $text)->p .= t("S", "b");
echo "|", $joined, $text->p, "\n";

$i = 0;
$m = [];
$m[$i++] = $i;
echo json_encode($m), $i, "\n";
---
index: 11
coalesce: 222
prop: 333
static: 444
chain: deepdeep
cond: 5
arg: 4abcd
compound: 33
deep: leafleaf
elemprop: setset
KV
K2V2|33
RS|abab
[1]1
