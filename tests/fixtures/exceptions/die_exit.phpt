name: die_exit
description: >
  die interrupts execution without producing an HTTP-style host error. A string
  argument is a message and is printed; an integer argument is an exit status.
---
<?php
echo "before";
die(7);
echo "after";
exit;
echo "again";
---
before
