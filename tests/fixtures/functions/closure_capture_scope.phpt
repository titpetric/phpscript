name: a closure carries its use list and nothing else
description: >
  The negative half of closure_capture.phpt. An invocation gets a frame holding
  the `use (...)` values, the parameters, and the `$this` a closure written in a
  method binds: the enclosing frame's other variables are unset inside it, a
  nested closure re-captures or sees nothing, and a `static function` has no
  receiver. get_defined_vars() reads the frame back, so the capture set is
  stated rather than implied. php defines the expected output.
---
<?php

function make($kept)
{
	$dropped = "not in the use list";
	return function ($name) use ($kept) {
		$vars = get_defined_vars();
		ksort($vars);
		return $name . ": " . $kept
			. ", dropped=" . var_export(isset($dropped), true)
			. ", frame=" . implode("+", array_keys($vars));
	};
}

echo make("kept")("one"), "\n";

$snapshot = "before";
$read = function () use ($snapshot) {
	return $snapshot;
};
$snapshot = "after";
echo "snapshot: ", $read(), "\n";

$outer = "outer";
$nested = function () use ($outer) {
	$inner = function () {
		return var_export(isset($outer), true);
	};
	return $outer . ", inner sees it=" . $inner();
};
echo "nested: ", $nested(), "\n";

class Holder
{
	public $prop = "property";

	public function bound()
	{
		$local = "local";
		return function () use ($local) {
			$vars = get_defined_vars();
			ksort($vars);
			return $this->prop . ", " . $local . ", frame=" . implode("+", array_keys($vars));
		};
	}

	public function unbound()
	{
		return static function () {
			return var_export(isset($this), true);
		};
	}
}

$holder = new Holder;
echo "bound: ", ($holder->bound())(), "\n";
echo "static: ", ($holder->unbound())(), "\n";
?>
---
one: kept, dropped=false, frame=kept+name
snapshot: before
nested: outer, inner sees it=false
bound: property, local, frame=local
static: false
