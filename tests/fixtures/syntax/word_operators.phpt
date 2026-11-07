name: word operators
description: >
  Covers and, or and xor: results as bool, the precedence below assignment,
  the order among the three, short-circuit evaluation of and and or against
  xor running both operands, the keyword in any case, and the idiom of acting
  on a failed step.
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
