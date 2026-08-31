name: the program execution functions
description: >
  The four ways to run a command differ in what they do with the output. exec
  collects it, returning the last line and appending every line to $output;
  system writes it out as it arrives and returns the last line; passthru writes
  it through untouched and returns null; shell_exec returns all of it, or null
  when there was none. All but shell_exec report the exit status through
  $result_code, which PHP passes by reference. $output is appended to rather
  than replaced, so a second call adds to what the first collected. Every
  command has fixed output, so nothing here depends on the machine it runs on.
---
<?php

$out = array();
$code = null;
echo exec("printf 'a\nb\nc\n'; exit 3", $out, $code), "|";
print_r($out);
echo $code, "|";

$ok = null;
echo exec("echo hello", $out, $ok), "|", $ok, "|";
print_r($out);

$status = null;
echo system("printf 'x\ny\n'; exit 4", $status), "|", $status, "|";

$passed = null;
passthru("printf 'p\nq\n'; exit 5", $passed);
echo $passed, "|";

var_dump(shell_exec("printf 's\nt\n'"));
var_dump(shell_exec("exit 7"));
echo escapeshellcmd("a; rm -rf /"), "|", escapeshellcmd("echo 'a b'"), "|";
echo escapeshellarg("it's"), "\n";
?>
---
c|Array
(
    [0] => a
    [1] => b
    [2] => c
)
3|hello|0|Array
(
    [0] => a
    [1] => b
    [2] => c
    [3] => hello
)
x
y
y|4|p
q
5|string(4) "s
t
"
NULL
a\; rm -rf /|echo 'a b'|'it'\''s'
