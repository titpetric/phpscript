name: numeric strings
description: Arithmetic on numeric strings keeps PHP's int/float distinction by the string's spelling; leading-numeric junk converts by prefix.
---
<?php
var_dump("5.5" + 1);
var_dump("1e2" * 2);
var_dump("5" + "5.5");
var_dump(" 5" + 1);
var_dump("5x" + 1);
var_dump("2" ** "3");
var_dump("1e1" ** 2);
var_dump("10" / "4");
var_dump("2.5" % 2);
---
float(6.5)
float(200)
float(10.5)
int(6)
int(6)
int(8)
float(100)
float(2.5)
int(0)
