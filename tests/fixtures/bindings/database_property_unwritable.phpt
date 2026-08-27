name: database binding property is not writable
serial: true
runner:
  php: false
description: >
  Only an exported field is a writable property. Assigning anything else throws
  rather than quietly doing nothing: an assignment that looks like it took
  effect and did not is the worst outcome for a property deciding what a client
  may do.
error: not a writable object property
---
<?php
$db = new Database("sqlite_test");
$db->transaction = 1;
echo "assigned";
---
Internal Server Error
