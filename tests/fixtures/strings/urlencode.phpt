name: urlencode, urldecode, rawurlencode, rawurldecode
description: >
  urlencode writes a space as '+' and escapes '~', where rawurlencode writes
  '%20' and leaves '~' alone; only urldecode reads '+' back as a space.
---
<?php
echo urlencode("a b~c.d-e_f*g"), "\n";
echo rawurlencode("a b~c.d-e_f*g"), "\n";
echo urlencode("key=value&other"), "\n";
echo rawurlencode("key=value&other"), "\n";
echo urldecode("a+b%20c%7Ed"), "\n";
echo rawurldecode("a+b%20c%7Ed"), "\n";
echo urldecode("100%25 %zz %4"), "\n";
var_dump(urldecode(urlencode("a b~c")) === "a b~c");
var_dump(rawurldecode(rawurlencode("a b~c")) === "a b~c");
---
a+b%7Ec.d-e_f%2Ag
a%20b~c.d-e_f%2Ag
key%3Dvalue%26other
key%3Dvalue%26other
a b c~d
a+b c~d
100% %zz %4
bool(true)
bool(true)
