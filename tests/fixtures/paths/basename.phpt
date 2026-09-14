name: basename takes the suffix argument and answers the empty and root paths like PHP
description: >
  basename() strips an optional suffix when the name ends with it, keeps a
  name that is only the suffix, matches case-sensitively, and answers ""
  for "" and for "/". dirname() takes the optional $levels argument.
---
<?php

// The suffix is stripped when the name ends with it.
echo basename("/srv/issues/088-wizard.yml", ".yml"), "\n";
echo basename("report.csv", ".csv"), "\n";
echo basename(".yml", ".yml"), "\n";
echo basename("a/b.YML", ".yml"), "\n";
echo basename("a/b.yml/", ".yml"), "\n";

// Without a suffix: the empty and root paths answer "".
var_dump(basename(""));
var_dump(basename("/"));
var_dump(basename("a/b/"));
var_dump(basename("."));

// dirname walks up $levels parents.
echo dirname("/a/b/c/d"), "\n";
echo dirname("/a/b/c/d", 2), "\n";
echo dirname("/a/b/c/d", 3), "\n";
echo dirname("a", 1), "\n";
?>
---
088-wizard
report
.yml
b.YML
b
string(0) ""
string(0) ""
string(1) "b"
string(1) "."
/a/b/c
/a/b
/a
.
