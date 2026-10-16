name: a retained func_get_args slice is the frame's own
description: >
  A method stores func_get_args() past its own return, and later calls on the
  same object run with their own arguments. The stored slice has to keep the
  values of the call that made it: the closure engine boxes helper-call
  arguments into a reused arena, and the frame's copy is what keeps a slice a
  script retained from reading a later call's values. php defines the expected
  output.
---
<?php

class Recorder
{
	public $saved;

	public function keep($a, $b)
	{
		$this->saved = func_get_args();
	}

	public function other($x, $y, $z)
	{
		return $x + $y + $z;
	}
}

$recorder = new Recorder;
$recorder->keep("first", "second");
echo "sum: ", $recorder->other(7, 8, 9), "\n";
echo "kept: ", implode(",", $recorder->saved), "\n";

$recorder->saved[0] = "edited";
$recorder->keep("third", "fourth");
echo "edit: ", $recorder->saved[0], ",", $recorder->saved[1], "\n";
?>
---
sum: 24
kept: first,second
edit: third,fourth
