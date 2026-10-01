name: regexp compilation and the string method set
description: >
  Regexp\Compile and Regexp\CompilePOSIX are host bindings with no PHP
  counterpart, so the expected output is the runtime's contract rather than
  PHP's. The value is a *regexp.Regexp, so the class a script sees is Regexp,
  a compile failure arrives as the error regexp.Compile returned, and
  literal_prefix()'s two Go results arrive as a PHP list.
runner:
  php: false
---
<?php

$rx = new Regexp\Compile('(\w+)@(\w+)\.com');

echo get_class($rx), "\n";
echo $rx instanceof Regexp ? "instance\n" : "not an instance\n";

// String() is the pattern, so the value renders as itself.
echo $rx, "\n";
echo $rx->num_subexp(), "\n";

echo $rx->match_string("tit@example.com") ? "match\n" : "no match\n";
echo $rx->find_string("mail tit@example.com now"), "\n";
echo implode("|", $rx->find_string_submatch("mail tit@example.com now")), "\n";
echo $rx->replace_all_string("a@b.com c@d.com", "<$1>"), "\n";

// Two Go results arrive as a list, not as the first one.
list($prefix, $complete) = (new Regexp\Compile('abc\d+'))->literal_prefix();
echo $prefix, ":", $complete ? "complete" : "partial", "\n";

// Named groups are numbered and named, which preg_match does not report.
$named = new Regexp\Compile('(?P<user>\w+)@(?P<host>\w+)');
echo implode(",", $named->subexp_names()), "\n";
echo $named->subexp_index("host"), "\n";

// POSIX takes the leftmost-longest match where Perl syntax takes the first.
echo (new Regexp\Compile('a|ab'))->find_string("xabz"), "\n";
echo (new Regexp\CompilePOSIX('a|ab'))->find_string("xabz"), "\n";

echo implode("/", (new Regexp\Compile(',\s*'))->split("a, b,c", -1)), "\n";

try {
	new Regexp\Compile('(unclosed');
	echo "compiled\n";
} catch (Exception $e) {
	echo $e->getMessage(), "\n";
}
---
Regexp
instance
(\w+)@(\w+)\.com
2
match
tit@example.com
tit@example.com|tit|example
<a> <c>
abc:partial
,user,host
2
a
ab
a/b/c
error parsing regexp: missing closing ): `(unclosed`
