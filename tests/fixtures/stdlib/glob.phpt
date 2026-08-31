name: glob
description: >
  glob lists the paths matching a shell wildcard, sorted, in the same spelling
  the pattern was written in. Every pattern here is scoped under glob_tree/ so
  the listing is the committed support tree and nothing else: filesystem.phpt
  creates and removes files in this same folder, and an unscoped pattern would
  see them when the two fixtures run together. A pattern matching nothing, and
  one naming a directory that does not exist, both answer with an empty array
  rather than false.
---
<?php

print_r(glob("glob_tree/*.txt"));
print_r(glob("glob_tree/*"));
print_r(glob("glob_tree/*/*.txt"));
print_r(glob("glob_tree/o?e.txt"));
print_r(glob("glob_tree/[nt]*"));
print_r(glob("glob_tree/*.json"));
print_r(glob("glob_tree/missing/*"));

var_dump(count(glob("glob_tree/*.txt")));
var_dump(in_array("glob_tree/one.txt", glob("glob_tree/*.txt")));
?>
---
Array
(
    [0] => glob_tree/one.txt
    [1] => glob_tree/two.txt
)
Array
(
    [0] => glob_tree/drafts
    [1] => glob_tree/notes.md
    [2] => glob_tree/one.txt
    [3] => glob_tree/two.txt
)
Array
(
    [0] => glob_tree/drafts/three.txt
)
Array
(
    [0] => glob_tree/one.txt
)
Array
(
    [0] => glob_tree/notes.md
    [1] => glob_tree/two.txt
)
Array
(
)
Array
(
)
int(2)
bool(true)
