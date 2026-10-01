name: a closure reaches a regexp method declaring a Go callback
description: >
  ReplaceAllStringFunc takes a func(string) string, not the uniform callable
  shape, and a PHP closure is converted to it at the argument boundary. The
  expected output is the runtime's contract: Regexp\Compile is a host binding
  with no PHP counterpart. A closure that throws reaches the caller's catch,
  because an error raised inside a Go callback crosses the boundary as a panic
  the host turns back into a throwable.
runner:
  php: false
---
<?php

$rx = new Regexp\Compile('\w+@\w+\.com');
$subject = "mail a@b.com and c@d.com now";

echo $rx->replace_all_string_func($subject, function ($match) {
	return strtoupper($match);
}), "\n";

// A declared function reaches it by name, as it does preg_replace_callback.
function redact($match) {
	return str_repeat("*", strlen($match));
}
echo $rx->replace_all_string_func($subject, "redact"), "\n";

// The byte-slice half of the pair takes func([]byte) []byte, and a closure
// returning a string fills it.
echo $rx->replace_all_func($subject, function ($match) {
	return "<" . $match . ">";
}), "\n";

try {
	$rx->replace_all_string_func($subject, function ($match) {
		throw new Exception("no replacement for " . $match);
	});
	echo "replaced\n";
} catch (Exception $e) {
	echo $e->getMessage(), "\n";
}
---
mail A@B.COM and C@D.COM now
mail ******* and ******* now
mail <a@b.com> and <c@d.com> now
no replacement for a@b.com
