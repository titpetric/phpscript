<?php

namespace App\Support;

class SomeClass {
	const KIND = "support";

	public $tag;

	function __construct($tag) {
		$this->tag = $tag;
	}

	static function get() {
		return "SomeClass::get";
	}

	function label() {
		return $this->tag . " (" . self::KIND . ")";
	}
}
