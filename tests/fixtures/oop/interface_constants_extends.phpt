name: an extending interface does not answer for its parent's constants
runner:
  php: false
description: >
  PHP resolves Quota::LIMIT through `interface Quota extends Base`. phpscript
  inherits nothing (docs/design.md): the constant answers only through Base,
  the name it was declared on, and the extender's spelling fails as an
  undefined class constant. Only phpscript defines the expected output.
error: "undefined class constant Quota::LIMIT"
---
<?php

interface Base {
	const LIMIT = 10;
}

interface Quota extends Base {
}

echo Quota::LIMIT;
---
Internal Server Error
