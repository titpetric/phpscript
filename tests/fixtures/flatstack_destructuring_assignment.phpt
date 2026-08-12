name: flatstack destructuring assignment
description: List destructuring assignments execute through native flat bytecode.
flatstack: true
---
<?php
$arr = [10, 20, 30];
list($a, $b) = $arr;
list($x, $y, $z) = $arr;
echo $a . ":" . $b . ":" . $x . ":" . $z;
?>
---
10:20:10:30
