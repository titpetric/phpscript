name: Runtime\Go areas bind go's standard library packages on import
description: >
  `use Runtime\Go\Strings` installs the strings package's functions under the
  alias, and `use Runtime\Go\Path` the lexical path functions. The default
  runtime carries neither. php has no Runtime namespace, so only phpscript
  defines the expected output.
runner:
  php: false
---
<?php

use Runtime\Go\Path;
use Runtime\Go\Strings;

echo implode("|", Strings\Fields("  a b   c ")), "\n";
echo Strings\ToUpper("go"), " ", Strings\Repeat("ab", 2), " ", Strings\Index("chicken", "ken"), "\n";
echo Path\Base("/a/b.txt"), " ", Path\Dir("/a/b.txt"), " ", Path\Ext("/a/b.txt"), " ", Path\Join("a", "b", "../c"), "\n";
var_dump(Path\IsAbs("a/b"));
---
a|b|c
GO abab 4
b.txt /a .txt a/c
bool(false)
