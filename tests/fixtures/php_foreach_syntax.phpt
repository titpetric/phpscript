name: if + foreach + else syntax usage
description: >
  Conditions in php allow to skip braces making the following example a common
  pattern to manage output in case no array data is available to foreach over.
---
<?php

function printArr($arr) {
  if (!empty($arr))
  foreach ($arr as $v) {
    echo $v."\n";
  } else {
    echo "No data";
  }
}

printArr([1,2,3]);
printArr([]);

?>
---
1
2
3
No data