<?php

namespace App\Registry;

abstract class Store {
	const KIND = "store";

	function describe() {
		return "a store";
	}
}

final class Counter extends Store implements \Countable {
	const KIND = "counter";

	private $label;
	private $items;

	function __construct($label) {
		$this->label = $label;
		$this->items = array();
	}

	function name() {
		return $this->label;
	}

	function add($item) {
		$this->items[] = $item;
	}

	function count(): int {
		return count($this->items);
	}

	function describe() {
		return "counter " . $this->label;
	}
}
