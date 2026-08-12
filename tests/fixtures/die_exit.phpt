name: die_exit
flatstack: true
description: die interrupts execution without producing an HTTP-style host error.
---
<?php
echo "before";
die(7);
echo "after";
exit;
echo "again";
---
before
