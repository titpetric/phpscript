name: an anonymous class is held to the contract it declares
runner:
  php: false
description: >
  Saying `implements` binds an anonymous class the same way it binds a written
  one: it must declare every method the interface names. The declaration is not
  a statement, so the check reads it from the list the parser collected rather
  than by walking the file, and it runs where classes are registered, before
  anything executes. PHP reports the same condition as a compile-time fatal
  error, which is why only phpscript defines the expected output. The class name
  in the message is the one the parser synthesized, because the source gave it
  none.
error: does not declare method has() required by interface Reader
---
<?php

interface Reader {
	function get($key);
	function has($key);
}

$store = new class implements Reader {
	function get($key) {
		return $key;
	}
};

echo $store->get("host"), "\n";
---
Internal Server Error
