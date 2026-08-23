name: casts compile to bytecode
description: >
  A cast is a *model.Cast node. The flatstack compiler lowers it and the host
  applies the same conversion the interpreter does, so a method normalising
  request input runs on bytecode instead of dropping its whole program back to
  the interpreter. tests/fixtures/arithmetic/casts.phpt covers the conversion
  table itself; this one covers the compiled path.
---
<?php

class Request
{
	public $page = 0;
	public $verbose = false;
	public $tags = array();

	function fill($page, $verbose, $tag)
	{
		$this->page = (int)$page;
		$this->verbose = (bool)$verbose;
		$this->tags = (array)$tag;
		return $this;
	}

	function describe()
	{
		return (string)$this->page . ":" . ($this->verbose ? "on" : "off") . ":" . count($this->tags);
	}
}

$request = new Request();
echo $request->fill("12abc", 1, "one")->describe(), "\n";
echo $request->fill("0", "", array("a", "b"))->describe(), "\n";
echo is_int((int)"7") ? "int" : "not-int", "\n";
echo (float)"2.5" + 1, "\n";
echo (string)true, ":", (string)false, ":done\n";
---
12:on:1
0:off:2
int
3.5
1::done
