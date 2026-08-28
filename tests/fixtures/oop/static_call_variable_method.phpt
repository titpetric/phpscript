name: a variable names the method of a static call
description: >
  `Mailer::$m($to)` calls the static method whose name is held in `$m`,
  from outside the class and through `self::` inside it — the variable
  spelling of a static call, not a static-property read followed by an
  invoke. Verified against php 8.5.
---
<?php

class Mailer {
	public static function send($to) {
		return "sent to " . $to;
	}

	public function deliver($to) {
		$m = 'send';
		return self::$m($to);
	}
}

$m = 'send';
echo Mailer::$m("alice"), "\n";
$mailer = new Mailer();
echo $mailer->deliver("bob"), "\n";
---
sent to alice
sent to bob
