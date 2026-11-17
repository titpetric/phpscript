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
echo len($ages), ":", $ages["ada"] === null ? "gone" : "here", "\n";
---
3:3
3
2:ada:37
0:0
grace
2:41
1:gone
