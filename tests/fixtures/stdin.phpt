name: stdin
description: >
  STDIN is a readable stream backed by the runtime input reader.
stdin: "name=catalogue&column_count=6"
---
<?php
$input = stream_get_contents(STDIN);
echo $input;
?>
---
name=catalogue&column_count=6
