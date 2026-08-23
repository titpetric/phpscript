name: a class constant as a property default
description: >
  A property or static default is written inside the class body, so self:: in
  one resolves to that class rather than to whatever scope the new happened to
  be written in. E_ALL is the constant this most often shows up through, and it
  is not every bit set: PHP 8 dropped E_STRICT from it.
---
<?php
class Level {
	const INFO = 3;
	const LABEL = "level";
	public $threshold = self::INFO;
	public $name = self::LABEL;
	public static $shared = self::INFO;
	public function bump() { return $this->threshold + self::INFO; }
}

$level = new Level();
echo $level->threshold, "\n";
echo $level->name, "\n";
echo Level::$shared, "\n";
echo $level->bump(), "\n";
echo E_ALL, "\n";
echo E_ALL & ~E_NOTICE, "\n";
---
3
level
3
6
30719
30711
