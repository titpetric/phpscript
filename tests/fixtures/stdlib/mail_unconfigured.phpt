name: mail without a configured server fails loudly and catchably
description: >
  A runtime whose host configured no mail servers resolves through a provider
  holding none, so mail() and `new Mail` both exist and both refuse catchably
  naming the server they looked for. function_exists is true either way, which
  is what lets calling code keep one spelling and its own log-the-link
  fallback. A host with a mail block hands the runtime its servers and the same
  two calls deliver. The php runner is opted out: its mail() attempts delivery.
runner:
  php: false
---
<?php

var_dump(function_exists("mail"));

try {
	mail("a@example.com", "s", "b");
	echo "sent\n";
} catch (Exception $e) {
	echo $e->getMessage(), "\n";
}

try {
	$mail = new Mail;
	echo "constructed\n";
} catch (Exception $e) {
	echo $e->getMessage(), "\n";
}
?>
---
bool(true)
no configuration found for mail server: default
no configuration found for mail server: default
