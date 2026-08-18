name: exception (uncaught)
runner:
  php: false
description: >
  An exception has to be thrown to cause an error.
  An uncaught exception results in an internal server error. The host error
  body is the runtime's contract, not something the PHP CLI produces.
error: boom
---
<?php

new Exception("foo");

throw new Exception("boom");

echo "unreachable";
---
Internal Server Error
