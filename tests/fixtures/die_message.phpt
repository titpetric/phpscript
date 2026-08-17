name: die_message
flatstack: true
description: >
  A string passed to die/exit is a message: it is printed before execution
  stops, and it is not read as an exit status.
---
<?php
function bail($message) {
	die($message);
}

echo "before:";
bail("stopped here");
echo "after";
---
before:stopped here
