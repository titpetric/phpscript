name: "self:: forwards $this to an instance method"
description: >
  Inside an instance method, self::inner() is $this->inner() in everything but
  spelling, so the callee sees the current instance. Runs under php unchanged.
---
<?php

class W {
	public $v = 7;
	public function inner() { return "v=" . $this->v; }
	public function outer() { return self::inner(); }
}
$w = new W();
echo $w->outer(), "\n";
---
v=7
