name: exception (uncaught)
description: >
  An uncaught exception results in an internal server error.
error: boom
---
<?php

new Exception("foo");

throw new Exception("boom");

echo "unreachable";
---
Internal Server Error
