name: strpos family offsets
description: >
  The strpos family takes an $offset, and a negative one counts from the end of
  the subject. Every one of them returns false rather than -1 when the needle is
  absent, which is why a caller has to compare with ===.
---
<?php
var_dump(strpos("abcabc", "b", 2));
var_dump(strpos("abcabc", "b", -2));
var_dump(strpos("abc", "z"));
var_dump(strrpos("abcabc", "b", -2));
var_dump(strrpos("abcabc", "b", 2));
var_dump(stripos("Hello", "L", 3));
var_dump(stripos("Hello", "L"));
var_dump(strripos("HeLlo", "l"));
var_dump(stripos("abc", "Z"));
---
int(4)
int(4)
bool(false)
int(4)
int(4)
int(3)
int(2)
int(3)
bool(false)
