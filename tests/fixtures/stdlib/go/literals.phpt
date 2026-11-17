name: go composite literals infer a slice or a map
runtime: go
description: >
  A bracketed literal is a []any and a braced one a map[string]any, and a
  literal that names its element type builds the go slice or map. Indexing and
  assignment read and write the container, and delete removes a map key. php
  writes no braced map, so only phpscript defines this output.
---
<?php

$list = [1, 2, 3];
echo len($list), ":", $list[2], "\n";

$rows = [[1, 2], [3]];
echo $rows[1][0], "\n";

$user = {"name": "ada", "age": 36};
$user["age"] = 37;
echo len($user), ":", $user["name"], ":", $user["age"], "\n";

// An empty literal still names its shape.
echo len([]), ":", len({}), "\n";

// A literal naming its element type builds the go slice, and a map literal
// naming its key and value types builds the go map.
$names = []string{"ada", "grace"};
echo $names[1], "\n";

$ages = map[string]int{"ada": 36, "grace": 40};
$ages["grace"] = 41;
echo len($ages), ":", $ages["grace"], "\n";

delete($ages, "ada");
echo len($ages), ":", $ages["ada"] == null ? "gone" : "here", "\n";

// A type nests, and a braced entry elides the element type it repeats. Both
// spellings build each level, converting every entry on the way in.
$rows = [][]string{{"a", "b"}, {"c"}};
echo $rows[0][1], $rows[1][0], "\n";

// A dims-only literal takes its base type from the values.
$grid = [][]{{"x"}, {"y", "z"}};
echo $grid[0][0], $grid[1][1], "\n";

$evens = map[string][]int{"evens": {2, 4}};
echo len($evens["evens"]), ":", $evens["evens"][1], "\n";

$users = []map[string]int{{"ada": 36}};
echo $users[0]["ada"], "\n";
---
3:3
3
2:ada:37
0:0
grace
2:41
1:gone
bc
xz
2:4
36
