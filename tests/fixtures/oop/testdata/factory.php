<?php

// Support file for anonymous_class_include.phpt: it declares no name, and the
// only way to reach what it builds is the value it returns. The file that
// includes this one declares TestInterface.
return new class implements TestInterface {
	public $label = "anonymous";
	private $seen = 0;

	public function name() {
		return $this->label;
	}

	public function greet($who) {
		$this->seen = $this->seen + 1;
		return "hello " . $who;
	}

	public function calls() {
		return $this->seen;
	}
};
