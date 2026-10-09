name: string interpolation, escapes and offsets
description: >
  The cases the simple and complex fixtures leave open: an escaped backslash
  before a name interpolates the name, a backslash before {$ suppresses the
  complex open while the dollar still interpolates, {\$ is a literal brace and
  dollar, a property holding an array is subscripted inside braces, a string
  variable is indexed by character, and a single-quoted literal interpolates
  nothing while unescaping only the backslash itself.
---
<?php

class Box {
	var $arr = null;
}

$name = "Ada";
$i = 7;
$box = new Box();
$box->arr = array("k" => "boxed");

echo "backslash=\\$name\n";
echo "openbrace=\{$i}\n";
echo "dollarbrace={\$i}\n";
echo "proparr={$box->arr['k']}\n";
echo "strchar=$name[0]\n";
echo "strcharbrace={$name[1]}\n";
echo 'single={$name}', "\n";
echo 'subscript=$box->arr[k]', "\n";
echo 'escape=\$name', "\n";
echo 'doubled=\\$name', "\n";
---
backslash=\Ada
openbrace=\{7}
dollarbrace={$i}
proparr=boxed
strchar=A
strcharbrace=d
single={$name}
subscript=$box->arr[k]
escape=\$name
doubled=\$name
