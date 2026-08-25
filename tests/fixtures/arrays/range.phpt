name: range
description: >
  range builds an inclusive sequence of ints, floats or single characters,
  stepping by $step and taking its direction from the endpoints.
---
<?php
echo implode(",", range(1, 5)) . "\n";
echo implode(",", range(5, 1)) . "\n";
echo implode(",", range(0, 10, 2)) . "\n";
echo implode(",", range(10, 0, 3)) . "\n";
echo implode(",", range(4, 4)) . "\n";

// A float endpoint makes the whole range floats, whole or not.
var_dump(range(0.0, 2.0));
echo implode(",", range(0, 10, 2.5)) . "\n";
echo implode(",", range(1, 2, 0.25)) . "\n";

// A float step with no fractional part leaves an integer range integral.
var_dump(range(0, 4, 2.0));

// Two single-character strings give a character range.
echo implode(",", range("a", "e")) . "\n";
echo implode(",", range("e", "a")) . "\n";
echo implode(",", range("a", "i", 2)) . "\n";
echo implode(",", range("A", "D")) . "\n";
---
1,2,3,4,5
5,4,3,2,1
0,2,4,6,8,10
10,7,4,1
4
array(3) {
  [0]=>
  float(0)
  [1]=>
  float(1)
  [2]=>
  float(2)
}
0,2.5,5,7.5,10
1,1.25,1.5,1.75,2
array(3) {
  [0]=>
  int(0)
  [1]=>
  int(2)
  [2]=>
  int(4)
}
a,b,c,d,e
e,d,c,b,a
a,c,e,g,i
A,B,C,D
