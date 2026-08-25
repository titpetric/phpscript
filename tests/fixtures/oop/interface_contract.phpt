name: a violated interface contract raises a RuntimeException
runner:
  php: false
description: >
  Store says `implements Reader` and declares get() but not has(). phpscript
  checks the contract where it registers classes, before anything runs, and
  raises a RuntimeException naming the class, the interface and the missing
  method; uncaught, execution aborts and the host renders an "Internal Server
  Error". PHP reports the same condition as a compile-time fatal error, which is
  why only phpscript defines the expected output.
error: class Store does not declare method has() required by interface Reader
---
<?php

interface Reader {
	function get($key);
	function has($key);
}

class Store implements Reader {
	function get($key) {
		return $key;
	}
}

$store = new Store;
echo $store->get("host"), "\n";
---
Internal Server Error
