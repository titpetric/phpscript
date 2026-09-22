name: Mail selects a server the host configured, by name
description: >
  `new Mail` asks for "default" and `new Mail($name)` asks for the server the
  host named, both resolved against the mail block of the suite's
  phpscript.yml. A name nobody configured is refused at construction, before a
  script has composed a message, and the error names only the server that was
  asked for. The php runner is opted out: Mail is a host binding and the name
  does not exist in php, so this fixture is the contract.
runner:
  php: false
---
<?php

$mail = new Mail;
var_dump($mail instanceof Mail);

$campaigns = new Mail("marketing");
var_dump($campaigns instanceof Mail);

// Names are resolved case-insensitively, the way connection names are.
$upper = new Mail("MARKETING");
var_dump($upper instanceof Mail);

try {
	$missing = new Mail("transactional");
	echo "constructed\n";
} catch (Exception $e) {
	echo $e->getMessage(), "\n";
}
?>
---
bool(true)
bool(true)
bool(true)
no configuration found for mail server: transactional
