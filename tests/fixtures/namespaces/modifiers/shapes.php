<?php

namespace App\Shapes;

abstract class Unit {
	const METERS = "m";
	const FEET = "ft";
}

final class Sealed {
	private $name;

	function __construct($name) {
		$this->name = $name;
	}

	function label() {
		return $this->name . " (" . Unit::METERS . ")";
	}
}

readonly class Point {
	public int $x;
	public int $y;

	function __construct($x, $y) {
		$this->x = $x;
		$this->y = $y;
	}

	function sum() {
		return $this->x + $this->y;
	}
}

final readonly class Tag {
	public string $text;

	function __construct($text) {
		$this->text = $text;
	}

	function shout() {
		return strtoupper($this->text);
	}
}
