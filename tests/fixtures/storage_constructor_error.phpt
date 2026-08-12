name: storage constructor error (uncaught)
flatstack: true
description: >
  A constructor returning an error surfaces as a thrown error: `new FailStorage`
  fails. Uncaught, execution aborts and the host renders an "Internal Server
  Error" — the following echo never runs.
error: boom
---
<?php
$storage = new FailStorage;
echo "unreachable";
---
Internal Server Error
