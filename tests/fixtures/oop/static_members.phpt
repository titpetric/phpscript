name: static methods and properties
description: >
  Class::method() and Class::$property, in the Class:: and self:: spellings.
  A static is storage on the class rather than on an instance: every instance
  and every static call reads and writes the same bag, and it outlives the
  instance that seeded it.
---
<?php

class Registry {
	public static $entries = array();
	public static $count = 0;
	public static $index = array();

	public static function add($name, $value) {
		self::$entries[$name] = $value;
		self::$count = self::$count + 1;
		self::$index[$name[0]][$name] = $value;
	}

	public static function all() {
		return self::$entries;
	}

	public function seed() {
		self::add("seeded", "yes");
	}
}

Registry::add("alpha", 1);
Registry::add("beta", 2);

$registry = new Registry;
$registry->seed();

foreach (Registry::all() as $name => $value) {
	echo $name . "=" . $value . "\n";
}
echo "count=" . Registry::$count . "\n";
echo "index=" . Registry::$index["a"]["alpha"] . Registry::$index["b"]["beta"] . "\n";

Registry::$count = 99;
echo "count=" . Registry::$count . "\n";
echo "name=" . Registry::class . "\n";
---
alpha=1
beta=2
seeded=yes
count=3
index=12
count=99
name=Registry
