name: unset removes a key from a map a binding returned
description: >
  get_defined_functions() answers a native Go map, the shape a Database row
  and a compact() result have too, and unset($map[$key]) on one was a silent
  no-op on both engines: the key stayed and isset() kept answering true, so a
  script that stripped a column from a row before json_encode() still sent
  it. compact() is not used here because the bytecode engine skips a program
  that calls it, and this has to run on all three.
---
<?php

$functions = get_defined_functions();
unset($functions["user"]);
echo (isset($functions["user"]) ? "kept" : "removed") . "|" . count($functions) . "\n";
echo implode(",", array_keys($functions)) . "\n";

unset($functions["missing"]);
echo count($functions) . "\n";
---
removed|1
internal
1
