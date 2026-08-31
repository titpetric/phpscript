name: the set-shaped array functions
description: >
  array_fill, array_fill_keys, array_combine, array_chunk, the diff and
  intersect pairs, and the three questions about a key. Covers what each one
  does to keys, since that is where they differ from one another: fill counts
  from a start index, chunk renumbers unless told not to, the value pairs keep
  the keys of the array they filter, and the key pairs never look at a value.
---
<?php

print_r(array_fill(5, 3, "x"));
print_r(array_fill(0, 0, "x"));
print_r(array_fill_keys(array("a", "b"), 0));
print_r(array_combine(array("a", "b"), array(1, 2)));

// Chunks are keyed from zero either way; $preserve_keys decides what happens
// inside them, and the last chunk is short unless the count divided evenly.
print_r(array_chunk(array(1, 2, 3, 4, 5), 2));
print_r(array_chunk(array("a" => 1, "b" => 2, "c" => 3), 2, true));

// The value pairs keep the keys of the array they filter, so the answer is
// not renumbered.
print_r(array_diff(array("a", "b", "c"), array("b")));
print_r(array_intersect(array("a", "b", "c"), array("b", "c", "d")));

// Values compare as strings here, which is why 0 and "0" are the same entry.
print_r(array_diff(array(0, 1), array("0")));

// With more than one array to compare against, diff wants a value in none of
// them and intersect wants it in all.
print_r(array_diff(array("a", "b", "c"), array("a"), array("b")));
print_r(array_intersect(array("a", "b", "c"), array("a", "b"), array("b", "c")));

// The key pairs never look at a value.
print_r(array_diff_key(array("a" => 1, "b" => 2), array("a" => 9)));
print_r(array_intersect_key(array("a" => 1, "b" => 2), array("a" => 9, "z" => 1)));

// An edge key without an internal pointer, and null when there is no edge.
var_dump(array_key_first(array("x" => 1, "y" => 2)));
var_dump(array_key_last(array("x" => 1, "y" => 2)));
var_dump(array_key_first(array()));
var_dump(array_key_last(array()));
var_dump(array_key_first(array(7 => "a", 3 => "b")));

// A list is keyed 0..n-1 in that order. An empty array is one; a gap, a string
// key or an order that starts elsewhere is not.
var_dump(array_is_list(array(1, 2)));
var_dump(array_is_list(array()));
var_dump(array_is_list(array(1 => 1)));
var_dump(array_is_list(array("a" => 1)));
var_dump(array_is_list(array(0 => "a", 2 => "b")));
?>
---
Array
(
    [5] => x
    [6] => x
    [7] => x
)
Array
(
)
Array
(
    [a] => 0
    [b] => 0
)
Array
(
    [a] => 1
    [b] => 2
)
Array
(
    [0] => Array
        (
            [0] => 1
            [1] => 2
        )

    [1] => Array
        (
            [0] => 3
            [1] => 4
        )

    [2] => Array
        (
            [0] => 5
        )

)
Array
(
    [0] => Array
        (
            [a] => 1
            [b] => 2
        )

    [1] => Array
        (
            [c] => 3
        )

)
Array
(
    [0] => a
    [2] => c
)
Array
(
    [1] => b
    [2] => c
)
Array
(
    [1] => 1
)
Array
(
    [2] => c
)
Array
(
    [1] => b
)
Array
(
    [b] => 2
)
Array
(
    [a] => 1
)
string(1) "x"
string(1) "y"
NULL
NULL
int(7)
bool(true)
bool(true)
bool(false)
bool(false)
bool(false)
