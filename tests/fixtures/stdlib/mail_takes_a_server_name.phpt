name: Mail takes the name of a server, not the settings of one
description: >
  The constructor used to accept the connection settings as an array, which
  meant a password had to be written into php source to use it at all. It takes
  a name now, and the old spelling is refused rather than coerced: a string
  parameter would read the array as the empty name and resolve it to "default",
  so a script written against the old binding would go on sending, silently,
  through whatever server the host configured. The php runner is opted out:
  Mail is a host binding and the name does not exist in php.
runner:
  php: false
---
<?php

try {
	$mail = new Mail(array(
		"host"     => "mail.example.com",
		"username" => "noreply@example.com",
		"password" => "secret",
	));
	echo "constructed\n";
} catch (Throwable $e) {
	echo $e->getMessage(), "\n";
}
?>
---
Mail(): argument #1 ($name) must be the name of a configured server, not the settings of one: connection settings are the host's, and belong in the mail block of its configuration
