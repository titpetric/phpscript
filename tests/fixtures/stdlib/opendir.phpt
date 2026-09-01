name: opendir, readdir and closedir
description: >
  The other way a script reads a directory: opendir hands out a handle, readdir
  walks it one name at a time until false, closedir gives it back. Every
  listing is scoped under glob_tree/ so it is the committed support tree and
  nothing else, for the reason glob.phpt gives: filesystem.phpt creates and
  removes files in this same folder. readdir lists "." and ".." and the walk
  skips them by hand, as php scripts do; the names are sorted because php
  answers in the directory's own order.
---
<?php

// A recursive listing, written the way a php script writes one. is_dir picks
// the entries to descend into, is_file the ones to print, and file_exists
// answers about both.
function walk(string $dir, string $indent = ""): void
{
    $handle = opendir($dir);
    if ($handle === false) {
        echo $indent, "cannot open ", $dir, "\n";
        return;
    }

    $names = [];
    while (($name = readdir($handle)) !== false) {
        if ($name === "." || $name === "..") {
            continue;
        }
        $names[] = $name;
    }
    closedir($handle);

    // php lists a directory in its own order, so a walk that wants a stable
    // listing sorts it.
    sort($names);

    foreach ($names as $name) {
        $path = $dir . "/" . $name;
        if (is_dir($path)) {
            echo $indent, $name, "/\n";
            walk($path, $indent . "    ");
            continue;
        }
        echo $indent, $name, " file=", var_export(is_file($path), true),
            " exists=", var_export(file_exists($path), true), "\n";
    }
}

walk("glob_tree");

// readdir lists "." and ".." as php does.
$handle = opendir("glob_tree");
$all = [];
while (($name = readdir($handle)) !== false) {
    $all[] = $name;
}
closedir($handle);
sort($all);
print_r($all);

// opendir answers false for a file, and for a name that is not there. The @
// keeps php's warning off stderr; phpscript parses it as a no-op.
var_dump(@opendir("glob_tree/one.txt"));
var_dump(@opendir("glob_tree/missing"));

// The questions the walk asks, answered directly.
var_dump(file_exists("glob_tree"), is_dir("glob_tree"), is_file("glob_tree"));
var_dump(file_exists("glob_tree/drafts/three.txt"), is_file("glob_tree/drafts/three.txt"));
var_dump(file_exists("glob_tree/nope"), is_file("glob_tree/nope"), is_dir("glob_tree/nope"));
?>
---
drafts/
    three.txt file=true exists=true
notes.md file=true exists=true
one.txt file=true exists=true
two.txt file=true exists=true
Array
(
    [0] => .
    [1] => ..
    [2] => drafts
    [3] => notes.md
    [4] => one.txt
    [5] => two.txt
)
bool(false)
bool(false)
bool(true)
bool(true)
bool(false)
bool(true)
bool(true)
bool(false)
bool(false)
bool(false)
