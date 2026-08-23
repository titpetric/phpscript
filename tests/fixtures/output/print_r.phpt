name: print_r
description: >
  print_r indents four spaces, renders true as 1 and false and null as nothing,
  and follows a nested array with a blank line. Its float precision is echo's,
  so 0.1+0.2 is 0.3 where var_dump shows every digit.
---
<?php
print_r(array(1, "a" => true, "b" => false, "c" => null));
print_r(array("a" => array("b" => 1), "c" => 2));
echo print_r(array(1), true);
echo print_r(0.1 + 0.2, true), "\n";
---
Array
(
    [0] => 1
    [a] => 1
    [b] => 
    [c] => 
)
Array
(
    [a] => Array
        (
            [b] => 1
        )

    [c] => 2
)
Array
(
    [0] => 1
)
0.3
