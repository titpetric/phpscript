name: condition syntax
flatstack: true
description: >
  Covers parenthesized PHP-style if conditions, phpscript's unwrapped short-if
  condition, and runtime compatibility for assignment expressions inside if
  conditions. Use `phpscript lint` to report assignment-in-condition syntax.
---
<?php
$foo = false;
if (!$foo) {
  echo "wrapped;";
}
if !$foo {
  echo "short;";
}
echo "before-error;";
if ($row = strlen("x")) {
  echo "assigned;";
}
---
wrapped;short;before-error;assigned;
