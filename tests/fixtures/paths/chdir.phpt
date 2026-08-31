name: chdir moves what a relative path resolves against
description: >
  chdir changes the directory relative paths resolve against, for includes and
  for the filesystem functions alike, and getcwd reports where it is. A
  directory that does not exist is refused with false and leaves the working
  directory where it was. The assertions are relative — a basename and a file
  read — because php answers getcwd with a host path and phpscript answers with
  one written from the source filesystem's root, which is the one divergence
  here. Nothing climbs above the fixture directory, which is phpscript's root
  and would be php's parent.
---
<?php

var_dump(chdir("workdir"));
echo basename(getcwd()), "|";

include "lib/greet.php";
echo workdir_greet(), "|";
echo trim(file_get_contents("note.txt")), "|";

var_dump(chdir("does-not-exist"));
echo basename(getcwd()), "|";

var_dump(chdir(".."));
echo trim(file_get_contents("workdir/note.txt")), "|";
echo file_exists("note.txt") ? "leaked" : "clean", "\n";
?>
---
bool(true)
workdir|from lib|note in workdir|bool(false)
workdir|bool(true)
note in workdir|clean
