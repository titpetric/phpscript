name: exec runs a command and collects its output
description: >
  exec returns the last line of stdout and appends each line to the $output
  array: PHP writes it by reference, this runtime appends into the shared
  array, and both spell the same result. escapeshellarg quotes for the shell
  and posix_getpid names a live process.
---
<?php

echo exec("echo hello"), "\n";
$out = array();
exec("printf 'a\nb\nc\n'", $out);
print_r($out);
echo exec("printf 'first\nlast'"), "\n";
echo escapeshellarg("a b"), "\n";
echo escapeshellarg("it's"), "\n";
var_dump(posix_getpid() > 0);
---
hello
Array
(
    [0] => a
    [1] => b
    [2] => c
)
last
'a b'
'it'\''s'
bool(true)
