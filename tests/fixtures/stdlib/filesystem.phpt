name: filesystem writes
description: >
  Covers the write side of the filesystem shims against real PHP: creating a
  file through a handle, copying it, renaming it, changing its mode, and
  removing it. Every one of them answers with a bool, and a call that cannot do
  its work answers false rather than failing the script.
---
<?php

$a = "filesystem_fixture_a.txt";
$b = "filesystem_fixture_b.txt";
$c = "filesystem_fixture_c.txt";
$missing = "filesystem_fixture_missing.txt";

$handle = fopen($a, "w");
fwrite($handle, "content");
fclose($handle);
echo file_exists($a) ? "created;" : "missing;";

echo copy($a, $b) ? "copied;" : "copy-failed;";
echo file_get_contents($b) . ";";
echo file_exists($a) ? "source-left;" : "source-gone;";

echo rename($b, $c) ? "renamed;" : "rename-failed;";
echo file_exists($b) ? "b-left;" : "b-gone;";
echo file_get_contents($c) . ";";

echo chmod($c, 0640) ? "chmod;" : "chmod-failed;";

echo unlink($a) ? "removed-a;" : "remove-a-failed;";
echo unlink($c) ? "removed-c;" : "remove-c-failed;";
echo file_exists($missing) ? "leftovers;" : "clean;";
---
created;copied;content;source-left;renamed;b-gone;content;chmod;removed-a;removed-c;clean;
