name: storage constructor error (uncaught)
runner:
  php: false
description: >
  A constructor returning an error surfaces as a thrown error: `new FailStorage`
  fails. Uncaught, execution aborts and the host renders an "Internal Server
  Error". The following echo never runs.
error: boom
---
<?php
$storage = new FailStorage;
echo "unreachable";
---
Internal Server Error
