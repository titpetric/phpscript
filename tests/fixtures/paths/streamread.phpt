name: reading a handle short of all of it
description: >
  fread, fgets, feof, fseek, ftell and rewind against a file the fixture writes
  first, so the bytes are stated here rather than depended on. Covers the two
  ways a loop ends - fgets answering false and feof answering true - and the
  seek that makes a second pass over the same handle possible.
---
<?php

$path = "streamread.tmp";
file_put_contents($path, "alpha\nbeta\ngamma");

// fread takes a length and gives back what it found, which at the end of the
// handle is the empty string rather than false.
$f = fopen($path, "r");
var_dump(ftell($f));
var_dump(fread($f, 5));
var_dump(ftell($f));
var_dump(feof($f));

// A seek from the end, then a read of more than is left.
fseek($f, -5, SEEK_END);
var_dump(ftell($f));
var_dump(fread($f, 100));
var_dump(feof($f));
var_dump(fread($f, 10));

// rewind puts the handle back at the start, which is what lets the same
// handle be read twice.
var_dump(rewind($f));
var_dump(ftell($f));
fclose($f);

// fgets keeps the newline that ends a line, and answers false when there is
// no line left. The last line of a file with no trailing newline is still a
// line.
$f = fopen($path, "r");
while (($line = fgets($f)) !== false) {
    var_dump($line);
}
var_dump(feof($f));
fclose($f);

// $length bounds a line, so a long line arrives in pieces. php counts the
// terminating NUL it never hands over, which is why a length of 4 reads three
// characters.
$f = fopen($path, "r");
var_dump(fgets($f, 4));
var_dump(fgets($f, 4));
fclose($f);

unlink($path);
?>
---
int(0)
string(5) "alpha"
int(5)
bool(false)
int(11)
string(5) "gamma"
bool(true)
string(0) ""
bool(true)
int(0)
string(6) "alpha
"
string(5) "beta
"
string(5) "gamma"
bool(true)
string(3) "alp"
string(3) "ha
"
