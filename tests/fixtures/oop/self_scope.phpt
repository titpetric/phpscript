name: self resolves constants and static calls inside the class
description: >
  The self:: spellings a class uses on itself: constants read from static and
  instance methods, a static method calling another, recursion through
  self::, an instance method delegating to a static one, and the self::class
  and static::class names. Runs under php unchanged.
---
<?php

class Temperature {
	const FREEZING = 0;
	const BOILING = 100;
	const SCALE = "celsius";

	public $reading;

	function __construct($reading) {
		$this->reading = $reading;
	}

	public static function clamp($value) {
		if ($value < self::FREEZING) {
			return self::FREEZING;
		}
		if ($value > self::BOILING) {
			return self::BOILING;
		}
		return $value;
	}

	public static function describe($value) {
		return self::clamp($value) . " " . self::SCALE;
	}

	public static function span($lo, $hi, $step) {
		if ($lo >= $hi) {
			return array();
		}
		$rest = self::span($lo + $step, $hi, $step);
		array_unshift($rest, self::clamp($lo));
		return $rest;
	}

	public function label() {
		return self::describe($this->reading);
	}

	public function reset() {
		$this->reading = self::FREEZING;
		return "[" . $this->reading . "]";
	}

	public function name() {
		return self::class;
	}

	public static function late() {
		return static::class;
	}
}

echo Temperature::clamp(-40), " ", Temperature::clamp(150), " ", Temperature::clamp(36), "\n";
echo Temperature::describe(120), "\n";
echo implode(",", Temperature::span(0, 100, 25)), "\n";

$t = new Temperature(212);
echo $t->label(), "\n";
echo $t->reset(), "\n";
echo $t->name(), "\n";
echo Temperature::late(), "\n";
echo Temperature::FREEZING === 0 ? "typed" : "coerced", "\n";
---
0 100 36
100 celsius
0,25,50,75
100 celsius
[0]
Temperature
Temperature
typed
