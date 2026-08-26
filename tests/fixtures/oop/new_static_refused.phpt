name: new static() is refused, not resolved
description: >
  A documented won't-implement (docs/design.md): late static binding has
  nothing to bind late without inheritance, and new does not resolve the
  static keyword to the enclosing class, so the factory idiom fails loudly
  with an undefined-class error. Spell the class name: new F(). php
  constructs the instance, so the php runner is opted out.
runner:
  php: false
error: 'new: undefined class "static"'
---
<?php

class F {
	public $tag = "made";
	public static function make() {
		$o = new static();
		return $o->tag;
	}
}

echo F::make(), "\n";
---
Internal Server Error
