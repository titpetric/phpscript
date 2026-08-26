name: extends records the parent name and confers nothing
description: >
  A documented won't-implement (docs/design.md): the child declares Animal as
  its parent, and nothing arrives through it. Its own members work, the
  parent's method raises a catchable undefined-method error, the parent's
  property is unset, and instanceof does not follow the parent name. php
  would inherit all of it, so the php runner is opted out; phpscript lint
  reports the extends clause as a finding.
runner:
  php: false
---
<?php

class Animal {
	public $legs = 4;
	public function speak() {
		return "generic noise";
	}
}

class Dog extends Animal {
	public function fetch() {
		return "ball";
	}
}

$dog = new Dog();
echo $dog->fetch(), "\n";
var_dump($dog instanceof Dog);
var_dump($dog instanceof Animal);
var_dump(isset($dog->legs));
try {
	$dog->speak();
} catch (Exception $e) {
	echo "no speak: ", $e->getMessage(), "\n";
}
---
ball
bool(true)
bool(false)
bool(false)
no speak: call to undefined method Dog::speak()
