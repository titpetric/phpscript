name: exception (caught)
description: >
  A caught exception can be handled with it's `getCode` and `getMessage`
  methods. If you use the variable `$e` itself, it only prints the message.
---
<?php

$err = new Exception("Message");
$err404 = new Exception("Not found", 404);

function printExp($e) {
	$code = $e->getCode();
	$message = $e->getMessage();
	echo sprintf("Exception: %s\n     Code: %d\n", $message, $code);
}

printExp($err);
printExp($err404);
---
Exception: Message
     Code: 0
Exception: Not found
     Code: 404
