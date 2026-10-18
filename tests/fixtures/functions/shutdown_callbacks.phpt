name: every spelling of a callable registers as a shutdown callback
description: >
  register_shutdown_function() takes a callable in any spelling a script writes
  one: a function by name, a closure, an object and a method as a two-element
  array, and a first-class callable. Each runs once after the body, in
  registration order, and the run ends with the status it would have had without
  them. The name is the spelling that was accepted at registration and then
  refused when the callbacks ran, which reported a finished script as a failure
  after its output. Expected output is php 8.5's.
---
<?php

class Mailer
{
	public $name;

	public function __construct($name)
	{
		$this->name = $name;
	}

	public function flush()
	{
		echo "flush ", $this->name, "\n";
	}
}

function sweep()
{
	echo "sweep\n";
}

$mailer = new Mailer("outbox");

register_shutdown_function("sweep");
register_shutdown_function(function () {
	echo "closure\n";
});
register_shutdown_function(array($mailer, "flush"));
register_shutdown_function(sweep(...));

echo "body\n";
?>
---
body
sweep
closure
flush outbox
sweep
