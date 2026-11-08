name: feof answers the read that came back empty
description: >
  php's end of file is a read that came back short, never the position: a seek
  to the end leaves the handle open, a read past the end sets it, a rewind
  clears it again, and a read that fills its buffer exactly leaves it open. A
  write handle stays open. php://memory keeps the same rule.
---
<?php

$name = "feof_fixture.txt";
file_put_contents($name, "line1\nline2\nend");

$f = fopen($name, "r");
echo feof($f) ? "eof;" : "open;";
fseek($f, 0, SEEK_END);
echo ftell($f), ";", feof($f) ? "eof;" : "open;";
var_dump(fread($f, 5));
echo feof($f) ? "eof;" : "open;";
rewind($f);
echo feof($f) ? "eof;" : "open;";
echo strlen(fread($f, 100)), ";", feof($f) ? "eof;" : "open;";
rewind($f);
echo strlen(fread($f, 15)), ";", feof($f) ? "eof;" : "open;";
var_dump(fread($f, 1));
echo feof($f) ? "eof;" : "open;";
rewind($f);
while (($line = fgets($f)) !== false) {
    echo "[", rtrim($line, "\n"), "]";
}
echo ";", feof($f) ? "eof;" : "open;";
rewind($f);
echo strlen(stream_get_contents($f)), ";", feof($f) ? "eof" : "open", "\n";
fclose($f);

$w = fopen($name, "a");
echo feof($w) ? "eof;" : "open;";
fwrite($w, "!");
echo feof($w) ? "eof" : "open", "\n";
fclose($w);

$m = fopen("php://memory", "w+");
fwrite($m, "abcdef");
rewind($m);
echo strlen(fread($m, 100)), ";", feof($m) ? "eof;" : "open;";
rewind($m);
echo strlen(fread($m, 6)), ";", feof($m) ? "eof" : "open", "\n";
fclose($m);

unlink($name);
---
open;15;open;string(0) ""
eof;open;15;eof;15;open;string(0) ""
eof;[line1][line2][end];eof;15;eof
open;open
6;eof;6;open
