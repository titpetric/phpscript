<?php
class Compiler {
	const E_FILENAME_EMPTY = "Filename can't be empty, tried to render %q";

	var $hooks;
	var $_open_tag;
	var $_close_tag;
}

class Template {
	var $hooks;

	var $_open_tag;
	var $_close_tag;

	function __construct() {
	}

	function render() {
	}
}

class Mixed {
	function first() {
	}

	private $stack = array();
	public static $instances = 0;

	const VERSION = "1.0";

	function second() {
	}
}
