name: interfaces as a declaration contract
description: >
  An interface names method signatures and a class that says `implements` must
  declare every one of them itself. Reader names two, Listing extends it and
  adds a third, Writer names a method and a static one, and Store satisfies both
  contracts it lists. Nothing is inherited: every method Store answers is
  written in Store, and the interfaces contribute no body, no property and no
  constant. `instanceof` answers an interface name from the list the class
  declared, including the names those interfaces extend. A static call
  selects the interpreter fallback in flatstack; this fixture holds both engines
  to the same output.
---
<?php

interface Reader {
	function get($key);
	function has($key);
}

interface Listing extends Reader {
	function keys();
}

interface Writer {
	function put($key, $value);
	static function label(): string;
}

class Store implements Listing, Writer {
	private $rows;

	function __construct() {
		$this->rows = array();
	}

	function get($key) {
		return $this->has($key) ? $this->rows[$key] : "";
	}

	function has($key) {
		return isset($this->rows[$key]);
	}

	function keys() {
		return array_keys($this->rows);
	}

	function put($key, $value) {
		$this->rows[$key] = $value;
	}

	static function label(): string {
		return "store";
	}
}

$store = new Store;
$store->put("host", "localhost");
$store->put("port", "8080");

echo Store::label(), "\n";
echo $store->get("host"), ":", $store->get("port"), "\n";
echo $store->has("host") ? "yes" : "no", "\n";
echo $store->has("user") ? "yes" : "no", "\n";
echo implode(",", $store->keys()), "\n";
echo $store->get("user") === "" ? "empty" : "set", "\n";
echo $store instanceof Store ? "Store" : "not Store", "\n";
echo $store instanceof Writer ? "Writer" : "not Writer", "\n";
echo $store instanceof Listing ? "Listing" : "not Listing", "\n";
echo $store instanceof Reader ? "Reader" : "not Reader", "\n";
echo $store instanceof Countable ? "Countable" : "not Countable", "\n";
---
store
localhost:8080
yes
no
host,port
empty
Store
Writer
Listing
Reader
not Countable
