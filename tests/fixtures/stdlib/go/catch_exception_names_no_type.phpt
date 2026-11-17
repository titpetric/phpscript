name: the go runtime refuses a catch clause naming an exception class
description: >
  The go runtime installs no php throwable class and its catch clauses name
  go's error alone. A clause naming Exception fails the parse, before the body
  runs. php parses the clause and the import blocks it there, so only
  phpscript defines the failure.
runtime: go
error: the go runtime catches error
---
<?php

try {
    throw "boom";
} catch (Exception $e) {
    echo "caught";
}
---
Internal Server Error
