name: go slice types
runner:
  php: false
description: >
  A file that imported the go runtime declares a `[]T` type, on a parameter, on
  a return type and on a property, and the engine reads the slice behind it by
  index and in a foreach. php has no spelling for the type, so it rejects the
  file and no php run can answer for this output.
---
<?php

use Runtime\Go;

class Row
{
    public []int64 $counts;

    public function __construct([]int64 $counts)
    {
        $this->counts = $counts;
    }

    public function total(): int64
    {
        $sum = 0;
        foreach ($this->counts as $count) {
            $sum += $count;
        }
        return int64($sum);
    }
}

// A slice from make takes the appends its capacity holds, as go's does.
function fill([]int64 $counts, int $n): []int64
{
    for ($i = 0; $i < $n; $i++) {
        $counts = append($counts, int64($i));
    }
    return $counts;
}

$row = new Row(fill(make([]int64, 0, 4), 4));
echo len($row->counts), ",", cap($row->counts), ",", $row->total(), "\n";
var_dump($row->counts[2]);

// A nested type is a slice of slices, and the element type of a class is the
// one php carries an instance as.
$rows = make([][]int64, 1);
$rows[0] = $row->counts;
echo len($rows), ",", len($rows[0]), "\n";

$objects = make([]Row, 1);
$objects[0] = $row;
echo len($objects), ",", $objects[0]->total(), "\n";
---
4,4,6
int(2)
1,4
1,6
