name: is_float and is_double report the one type, and no conversion
description: >
  Every other type predicate was registered - is_int, is_string, is_bool,
  is_array, is_object, is_null, is_numeric - and is_float was not, so a script
  could ask after each type but the one that needed asking about.

  The question it answers is the type held and never the text: "1.5" is a
  string, so both spellings answer false, which is the difference from
  is_numeric. is_double is php's older name for the same function, and a script
  written for php uses whichever its author learned.
---
<?php
foreach ([1.5, 0.0, 1, "1.5", true, null, [], 1e300] as $value) {
	echo var_export($value, true), ": is_float=", var_export(is_float($value), true),
		" is_double=", var_export(is_double($value), true),
		" is_int=", var_export(is_int($value), true), "\n";
}
echo is_float(1/3) ? "division is a float\n" : "division is not a float\n";
echo is_float(2 ** 63) ? "overflow is a float\n" : "overflow is not a float\n";
---
1.5: is_float=true is_double=true is_int=false
0.0: is_float=true is_double=true is_int=false
1: is_float=false is_double=false is_int=true
'1.5': is_float=false is_double=false is_int=false
true: is_float=false is_double=false is_int=false
NULL: is_float=false is_double=false is_int=false
array (
): is_float=false is_double=false is_int=false
1.0E+300: is_float=true is_double=true is_int=false
division is a float
overflow is a float
