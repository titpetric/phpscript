name: a global is read in the global scope and nowhere else
runner:
  php: false
description: >
  $argv and $argc are globals, so the file body reads them and a function, a
  closure and a method do not: `global` parses and binds nothing here
  (docs/design.md), and both engines answer the same. The bytecode engine used
  to answer one from any frame, because an uninitialised slot fell through to
  the host's global table instead of being seeded into the top-level frame. A
  superglobal is the other rule and is read from every frame. The harness
  supplies the request, which the PHP CLI SAPI has no equivalent of.

request:
  args:
    name: Alice
---
<?php

function reader()
{
	return "argv=" . var_export(isset($argv), true)
		. " argc=" . var_export(isset($argc), true)
		. " server=" . var_export(isset($_SERVER), true);
}

class Holder
{
	public function read()
	{
		global $argv;
		return "argv=" . var_export(isset($argv), true);
	}
}

echo "top: argv=", var_export(isset($argv), true),
	" argc=", var_export(isset($argc), true),
	" name=", $argv[1], "\n";
echo "function: ", reader(), "\n";

$closure = function () {
	return "argv=" . var_export(isset($argv), true)
		. " server=" . var_export(isset($_SERVER), true);
};
echo "closure: ", $closure(), "\n";

$capturing = function () use ($argv) {
	return "argv=" . var_export(isset($argv), true) . " name=" . $argv[1];
};
echo "capture: ", $capturing(), "\n";
echo "method: ", (new Holder)->read(), "\n";
?>
---
top: argv=true argc=true name=Alice
function: argv=false argc=false server=true
closure: argv=false server=true
capture: argv=true name=Alice
method: argv=false
