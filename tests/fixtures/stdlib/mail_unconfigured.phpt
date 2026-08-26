name: mail without smtp configuration fails loudly and catchably
description: >
  The stdlib default binds mail() to a refusal naming the missing
  configuration: function_exists is true and the call throws catchably, so
  calling code keeps one spelling and its own log-the-link fallback. A host
  with an smtp block re-binds mail() to the configured sender over this
  default. The php runner is opted out: its mail() attempts delivery.
runner:
  php: false
---
<?php

var_dump(function_exists("mail"));
try {
	mail("a@example.com", "s", "b");
	echo "sent\n";
} catch (Exception $e) {
	echo "refused: no smtp configured\n";
}
---
bool(true)
refused: no smtp configured
