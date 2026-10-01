name: a regexp method returning bytes answers a string
description: >
  Half of regexp.Regexp's method set is declared over []byte, and a PHP string
  is a byte string, so the runtime reads a binding's []byte as a string rather
  than as a list of integers. The expected output is the runtime's contract:
  these are host bindings with no PHP counterpart.
runner:
  php: false
---
<?php

$rx = new Regexp\Compile('(\w+)@(\w+)\.com');
$subject = "mail a@b.com and c@d.com now";

$found = $rx->find($subject);
echo $found, "\n";
echo strlen($found), "\n";
echo gettype($found), "\n";
echo is_string($found) ? "string" : "not a string", "\n";
echo is_array($found) ? "array" : "not an array", "\n";
echo $found === "a@b.com" ? "identical" : "different", "\n";
echo $found[0], $found[1], $found[2], "\n";

// A submatch is a list of byte strings, and each element reads as its text.
echo implode("|", $rx->find_submatch($subject)), "\n";
echo count($rx->find_submatch($subject)), "\n";

$pairs = array();
foreach ($rx->find_all_submatch($subject, -1) as $i => $match) {
	foreach ($match as $group => $text) {
		$pairs[] = $i . "." . $group . "=" . $text;
	}
}
echo implode(" ", $pairs), "\n";

echo $rx->replace_all($subject, "<$2>"), "\n";

// No match is the nil slice, which reads as the empty string and is falsey.
$missing = $rx->find("nothing here");
echo "[", $missing, "]\n";
echo $missing ? "truthy" : "falsey", "\n";
echo strlen($missing), "\n";
---
a@b.com
7
string
string
not an array
identical
a@b
a@b.com|a|b
3
0.0=a@b.com 0.1=a 0.2=b 1.0=c@d.com 1.1=c 1.2=d
mail <b> and <d> now
[]
falsey
0
