name: preg backreferences and lookaround
description: >
  Patterns RE2 cannot express — a backreference, a lookahead, a lookbehind, an
  atomic group — are compiled by the backtracking engine, and produce the same
  $matches shape as the ones RE2 handles. The backreference case is the one a
  template compiler needs: a tag paired with its own closing tag.
---
<?php

$template = "{inline item}body one{/inline}\n{block page}body two{/block}\n";
$found = preg_match_all("/\{(block|inline) ([a-zA-Z0-9_-]+)\}(.*?)\{\/\\1\}/s", $template, $matches);

echo "found=" . $found . "\n";
foreach ($matches[0] as $index => $whole) {
	echo $matches[1][$index] . ":" . $matches[2][$index] . "=" . $matches[3][$index] . "\n";
}

echo preg_match("/foo(?=bar)/", "foobar") . preg_match("/foo(?=bar)/", "foobaz") . "\n";
echo preg_match("/(?<=a)b/", "ab") . preg_match("/(?<=x)b/", "ab") . "\n";
echo preg_match("/(?>a+)b/", "aaab") . "\n";

echo preg_replace("/(\w+) (\w+)/", "\\2 \$1", "hello world") . "\n";
echo preg_quote("a.b(c)d") . "\n";

preg_match_all("/(a)(b)?/", "ab a", $optional);
echo count($optional) . "|" . implode(",", $optional[0]) . "|" . implode(",", $optional[2]) . "|\n";

echo preg_match("/ABC/i", "xxabcxx") . preg_match("/^b/m", "a\nb") . "\n";
---
found=2
inline:item=body one
block:page=body two
10
10
1
world hello
a\.b\(c\)d
3|ab,a|b,|
11
