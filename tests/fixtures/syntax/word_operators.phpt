name: word operators
description: >
  Covers and, or and xor: results as bool, the precedence below assignment,
  the order among the three, short-circuit evaluation of and and or against
  xor running both operands, the keyword in any case, the idiom of acting on a
  failed step, the symbol forms and ?? binding tighter, and a word form under
  return.
---
<?php

var_dump(true and false);
var_dump(false or true);
var_dump(true xor true);
var_dump(true xor false);

// They bind looser than assignment: the variable gets the left operand.
$x = true and false;
var_dump($x);
$y = false or true;
var_dump($y);
$z = (true xor true);
var_dump($z);

// `and` binds tighter than `xor`, `xor` tighter than `or`.
$t = 1 and 0 or 1;
var_dump($t);
var_dump(1 or 0 and 0);
var_dump(1 xor 1 or 1);
var_dump(0 or 1 xor 1);

// `and` and `or` short-circuit; `xor` runs both operands.
function loud($v) { echo "eval $v\n"; return $v; }
var_dump(loud(1) xor loud(1));
var_dump(loud(0) and loud(1));
var_dump(loud(1) or loud(0));
if (loud(true) and !loud(false)) { echo "both\n"; }

// Any case spells them.
var_dump(true AND false, false Or true, true XoR false);

// The idiom: act when a step fails.
function fail($m) { echo $m, "\n"; return false; }
function check($v) { return $v > 0; }
$ok = check(0) or fail("failed");
var_dump($ok);
check(0) or fail("check failed");
check(1) or fail("never");

// The symbol forms bind tighter than the word forms.
var_dump(true and false || true);
var_dump(false || true and false);
var_dump(true && false or true);

// `??` binds tighter than the word forms, and assignment sits between them.
$u = null;
var_dump($u ?? false or true);
$w = $u ?? 'd' and false;
var_dump($w);

// `return` takes the whole expression, word forms included.
function both() { return 0 or 1; }
function first() { return 1 and 0; }
var_dump(both(), first());

// In a loop condition, and under one `and` with two assignments.
$i = 0;
while ($i < 3 and $i !== 2) { $i++; }
echo $i, "\n";
$a = 1 and $b = 2;
var_dump($a, $b);

// `!` binds tighter than the word forms.
var_dump(!false and !true);
---
bool(false)
bool(true)
bool(false)
bool(true)
bool(true)
bool(false)
bool(false)
int(1)
bool(true)
bool(true)
bool(false)
eval 1
eval 1
bool(false)
eval 0
bool(false)
eval 1
bool(true)
eval 1
eval 
both
bool(false)
bool(true)
bool(true)
failed
bool(false)
check failed
bool(true)
bool(false)
bool(true)
bool(true)
string(1) "d"
bool(true)
bool(false)
2
int(1)
int(2)
bool(false)
