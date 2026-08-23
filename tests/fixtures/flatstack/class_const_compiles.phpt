name: class constants compile to bytecode
description: >
  A class constant read as a value is a *model.ClassConst node. The flatstack
  compiler lowers it, resolving self, static and parent to the class whose
  method is being compiled, which is what the interpreter's resolveClassName
  does at run time. Constants are how PHP 5-compatible code spells an enum, so
  before this every class declaring one fell back to the interpreter whole.
---
<?php

class Level
{
	const DEBUG = 0;
	const INFO = 1;
	const NAME = "level";
	const LABEL = self::NAME . ":info";

	function name()
	{
		return self::NAME;
	}

	function label()
	{
		return static::LABEL;
	}

	function which()
	{
		return self::class;
	}

	function above($level)
	{
		return $level >= self::INFO;
	}
}

class Wire
{
	const SEP = "|";

	function join($a, $b)
	{
		return $a . self::SEP . $b . Level::NAME;
	}
}

$level = new Level();
echo $level->name(), "\n";
echo $level->label(), "\n";
echo $level->which(), "\n";
echo $level->above(Level::DEBUG) ? "yes" : "no", "\n";
echo $level->above(Level::INFO) ? "yes" : "no", "\n";

$wire = new Wire();
echo $wire->join("a", "b"), "\n";
echo Level::DEBUG, ",", Level::INFO, ",", Level::LABEL, "\n";
echo Wire::class, "\n";
---
level
level:info
Level
no
yes
a|blevel
0,1,level:info
Wire
