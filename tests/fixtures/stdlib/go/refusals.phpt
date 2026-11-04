name: go refusals
runner:
  php: false
description: >
  A go builtin refuses what it cannot answer for by throwing a
  RuntimeException, which is what a php catch filters on, and panic throws the
  same one. php has no panic() to compare against, so no php run can answer for
  this output.
---
<?php

use Runtime\Go;

try {
    panic("boom");
} catch (RuntimeException $e) {
    echo $e->getMessage(), "\n";
}

// make allocates the element types php renders a value of. A sized width is a
// conversion and not an element type, and the message names the ones it has.
try {
    make([]int8, 1);
} catch (RuntimeException $e) {
    echo $e->getMessage(), "\n";
}

try {
    make([]int, 2, 1);
} catch (RuntimeException $e) {
    echo $e->getMessage(), "\n";
}

try {
    append("abc", 1);
} catch (RuntimeException $e) {
    echo $e->getMessage(), "\n";
}

try {
    int64(1, 2);
} catch (RuntimeException $e) {
    echo $e->getMessage(), "\n";
}
---
panic: boom
Runtime\Go\make: cannot allocate a []int8; the element types are any, bool, float64, int, int64, string and a class name
Runtime\Go\make: a length of 2 and a capacity of 1 make no []int
Runtime\Go\append: cannot append to a string
Runtime\Go\int64: takes one value and no more
