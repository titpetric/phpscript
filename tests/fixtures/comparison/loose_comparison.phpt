name: loose and strict comparison across types
description: >
  PHP 8's comparison table on the operand pairs a request carries: a numeric
  string from $_REQUEST against an int or float, a bool against a number, null
  against 0, "" and false, and two numeric strings that spell the same number
  differently. == and != coerce; === and !== are identity and never do; the
  relational operators read the same table; a non-numeric string never
  compares numerically. Both the literal and the variable spelling of every
  pair go through the same path, and both engines answer what php answers.

  Regression for issue 88: the interpreter used to hand comparison operands to
  expr, which types them as Go values, so `$copy == 1` against "1" answered
  false silently and the literal `"1" == 1` aborted the statement with a
  mismatched-types message, while switch, in_array, arithmetic and the
  flatstack host already coerced. https://github.com/titpetric/phpscript/issues/88
---
<?php

// The spelling every mobius call site had to use while == did not coerce:
// cast, then compare strictly. It keeps working.
$copy = "1";
var_dump((int)$copy === 1);

// The loose comparison switch and in_array always performed.
switch ($copy) {
    case 1:
        echo "switch matched\n";
        break;
}
var_dump(in_array($copy, [1, 2]));

// Arithmetic coerces, as docs/README.md says it does.
var_dump($copy + 1);

// == and != coerce across types, through a variable and as a literal alike.
var_dump($copy == 1);
var_dump(1 == $copy);
var_dump($copy != 1);
var_dump("1" == 1);
var_dump(1 == "1");
var_dump("1.5" == 1.5);

$f = "1.5";
var_dump($f == 1.5);

// A bool operand drags the other side to bool.
var_dump(true == 1);
$b = true;
var_dump($b == 1);
var_dump("a" == true);

// null loosely equals 0, "" and false.
$n = null;
var_dump($n == 0);
var_dump(null == "");
var_dump($n == "");
var_dump($n == false);

// Two numeric strings compare numerically.
$a = "1";
$z = "01";
var_dump($a == $z);
var_dump("10" == "1e1");

// The relational operators take the same table.
var_dump($copy < 2);
var_dump($copy >= 1);
var_dump("2" >= 1);
var_dump($b < 2);
var_dump($n < 1);

// Strict comparison never coerces, in either spelling.
var_dump($copy === 1);
var_dump($copy !== 1);
var_dump("1" === 1);
var_dump("1" !== 1);

// int against float is numeric.
var_dump(1 == 1.0);
$i = 1;
$g = 1.5;
var_dump($i < $g);

// PHP 8 does not compare a non-numeric string numerically.
var_dump("abc" == 0);

// Arrays compare pairwise; identity wants the same order and types.
var_dump([1] == [1]);
var_dump([1] === [1]);
var_dump(["a" => 1, "b" => 2] == ["b" => 2, "a" => 1]);
var_dump(["a" => 1, "b" => 2] === ["b" => 2, "a" => 1]);
var_dump([1] === ["1"]);
?>
---
bool(true)
switch matched
bool(true)
int(2)
bool(true)
bool(true)
bool(false)
bool(true)
bool(true)
bool(true)
bool(true)
bool(true)
bool(true)
bool(true)
bool(true)
bool(true)
bool(true)
bool(true)
bool(true)
bool(true)
bool(true)
bool(true)
bool(true)
bool(false)
bool(true)
bool(false)
bool(true)
bool(false)
bool(true)
bool(true)
bool(true)
bool(false)
bool(true)
bool(true)
bool(true)
bool(false)
bool(false)
