name: the surplus count names the arguments a script wrote
description: >
  A binding that records its work on the request's trace declares a Go context
  as its first parameter, which the runtime injects: the script never writes it
  and no PHP signature carries it. The refusal reports the parameters a script
  may fill and the arguments it wrote, so a caller who removes one argument from
  a call the message refused reaches a call that runs. `mail()` is the shape, at
  PHP's three arguments: $additional_headers and $additional_params are not
  accepted here (see the Mail divergences in docs/README.md), so the fourth
  argument this fixture passes is a surplus phpscript refuses and php delivers
  on. The php runner is opted out for that reason, and no mail server is
  configured because the count is read before the delivery is attempted.
runner:
  php: false
---
<?php

try {
	mail("ana@example.test", "Subject", "Body", "Reply-To: lead@example.test");
	echo "not reached\n";
} catch (Throwable $e) {
	echo $e->getMessage(), "\n";
}

echo "still running\n";
?>
---
mail() expects at most 3 arguments, 4 given
still running
