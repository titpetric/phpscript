name: new self() is refused, not resolved
description: >
  A documented won't-implement (docs/design.md): new does not resolve the
  self keyword to the enclosing class, so the construction fails loudly with
  an undefined-class error instead of minting an instance. Spell the class
  name: new G(). php constructs the instance, so the php runner is opted out.
runner:
  php: false
error: 'new: undefined class "self"'
---
<?php

class G {
	public $tag = "made";
	public static function make() {
		$o = new self();
		return $o->tag;
	}
}

echo G::make(), "\n";
---
Internal Server Error
