<?php

class TestCase {
	var $assertions = 0
	fn assertTrue($val, $message = "") {
		$ok = is_bool($val) && $val
		if !$ok {
			if $message {
				throw new Exception($message);
			}
			throw new Exception("value expected true");
		}
		$this->assertions = $this->assertions + 1;
	}

	fn assertEquals($k, $v, $message = "") {
		$ok = $k == $v
		if !$ok {
			if $message {
				throw new Exception($message);
			}
			throw new Exception(sprintf("values differ, %v != %v", $k, $v));
		}

		$this->assertions = $this->assertions + 1;
	}

	fn assertFileEquals($f1, $f2) {
		return file_get_contents($f1) == file_get_contents($f2);
	}
}
