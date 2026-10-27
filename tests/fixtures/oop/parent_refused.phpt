name: "parent:: is refused, not resolved to the enclosing class"
description: >
  A documented won't-implement (docs/design.md): extends records the parent
  name and builds nothing, so `parent` names no class at run time. The
  reference fails loudly instead of calling the class's own method. php
  resolves it against the declared parent and answers the parent's method, so
  the php runner is opted out. Compose the members instead, or call the class
  that carries the behaviour by name.
runner:
  php: false
error: 'cannot use "parent": no inheritance at runtime'
---
<?php

class Animal {
	public function speak() {
		return "generic noise";
	}
}

class Dog extends Animal {
	public function speak() {
		return parent::speak();
	}
}

$dog = new Dog();
echo $dog->speak(), "\n";
---
Internal Server Error
