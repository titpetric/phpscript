name: a top-level const declaration
description: >
  `const` is a statement keyword: `const NAME = value;` at the top level of
  a file declares into the same constant table define() writes to, with the
  value evaluated once at the declaration. The comma-separated list form
  declares each entry in order, and one entry may read the one before it.
  define() and a class const are here to show all three spellings of a
  constant agree. Guards the fix for
  https://github.com/titpetric/phpscript/issues/85.
---
<?php

// define() declares a global constant at run time.
define("LAYOUT_DATE", "2006-01-02");
echo LAYOUT_DATE, "\n";

// const inside a class declaration.
class Layout
{
    const DATETIME = "2006-01-02 15:04:05";
}

echo Layout::DATETIME, "\n";

// The same declaration at the top level of a file.
const LAYOUT_DATETIME = "2006-01-02 15:04:05";

echo LAYOUT_DATETIME, "\n";

// The list form declares each entry in order, so a later entry reads an
// earlier one.
const LAYOUT_YEAR = "2006", LAYOUT_MONTH = LAYOUT_YEAR . "-01";

echo LAYOUT_YEAR, "\n";
echo LAYOUT_MONTH, "\n";
?>
---
2006-01-02
2006-01-02 15:04:05
2006-01-02 15:04:05
2006
2006-01
