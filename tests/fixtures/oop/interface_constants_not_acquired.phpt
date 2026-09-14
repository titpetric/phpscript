name: an implementing class does not acquire interface constants
runner:
  php: false
description: >
  PHP copies interface constants onto every implementing class, so there
  Store::LIMIT prints 10. phpscript inherits nothing (docs/design.md): the
  constant resolves only through the name it was declared on, and the class
  spelling fails as an undefined class constant. Only phpscript defines the
  expected output.
error: "undefined class constant Store::LIMIT"
---
<?php

interface Quota {
	const LIMIT = 10;
}

class Store implements Quota {
}

echo Store::LIMIT;
---
Internal Server Error
