name: ucfirst, lcfirst, ucwords
description: >
  The byte-wise case functions touch the ASCII range only, leave an empty
  string and a leading digit alone, and ucwords splits on the default
  whitespace list or on the separators given.
---
<?php
var_dump(ucfirst(""));
var_dump(ucfirst("hello"));
var_dump(ucfirst("1abc"));
var_dump(ucfirst("Hello"));
var_dump(ucfirst("ábc"));
var_dump(lcfirst(""));
var_dump(lcfirst("Hello"));
var_dump(lcfirst("HELLO"));
var_dump(lcfirst("1ABC"));
var_dump(ucwords(""));
var_dump(ucwords("hello world-again"));
var_dump(ucwords("hello world-again", "-"));
var_dump(ucwords("hello world-again", " -"));
var_dump(ucwords("  leading spaces"));
var_dump(ucwords("ALREADY UPPER"));
var_dump(ucwords("1st place 2nd try"));
var_dump(ucwords("a\tb\nc\rd\fe\vf") === "A\tB\nC\rD\fE\vF");
var_dump(ucwords("a\tb", "-") === "A\tb");
---
string(0) ""
string(5) "Hello"
string(4) "1abc"
string(5) "Hello"
string(4) "ábc"
string(0) ""
string(5) "hello"
string(5) "hELLO"
string(4) "1ABC"
string(0) ""
string(17) "Hello World-again"
string(17) "Hello world-Again"
string(17) "Hello World-Again"
string(16) "  Leading Spaces"
string(13) "ALREADY UPPER"
string(17) "1st Place 2nd Try"
bool(true)
bool(true)
