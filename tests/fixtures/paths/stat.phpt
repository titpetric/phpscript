name: the questions a script asks about a path
description: >
  is_file, is_dir, filesize, filetype, realpath, scandir and pathinfo, against
  the fixture's own folder. They resolve a path the way file_get_contents
  resolves it, so the answers agree with what a read of the same name would
  reach. pathinfo is the one that never touches the filesystem, so it answers
  about a path that is not there.
---
<?php

// The fixture's own folder is the root, so a bare name is the file beside it.
var_dump(is_file("stat.phpt"));
var_dump(is_dir("stat.phpt"));
var_dump(is_file("nothing-here"));
var_dump(is_dir("nothing-here"));
var_dump(is_readable("stat.phpt"));

// filesize and filetype answer about the same file, and answer false about a
// name that is not there rather than 0 or "".
var_dump(filesize("nothing-here"));
var_dump(filetype("stat.phpt"));
var_dump(filetype("nothing-here"));

// A file is its own size in bytes; the fixture reads its own length rather
// than a number that would change with every edit.
var_dump(filesize("stat.phpt") === strlen(file_get_contents("stat.phpt")));

// realpath answers false for what is not there.
var_dump(realpath("nothing-here"));

// scandir lists "." and ".." first, as php lists them, then the names sorted.
$names = scandir(".");
var_dump($names[0]);
var_dump($names[1]);
var_dump(in_array("stat.phpt", $names));
var_dump(scandir("nothing-here"));

// pathinfo never touches the filesystem, so these paths need not exist. A
// name with no dot carries no extension key at all.
print_r(pathinfo("/var/www/html/index.inc.php"));
print_r(pathinfo("noext"));
print_r(pathinfo("dir/"));
?>
---
bool(true)
bool(false)
bool(false)
bool(false)
bool(true)
bool(false)
string(4) "file"
bool(false)
bool(true)
bool(false)
string(1) "."
string(2) ".."
bool(true)
bool(false)
Array
(
    [dirname] => /var/www/html
    [basename] => index.inc.php
    [extension] => php
    [filename] => index.inc
)
Array
(
    [dirname] => .
    [basename] => noext
    [filename] => noext
)
Array
(
    [dirname] => .
    [basename] => dir
    [filename] => dir
)
