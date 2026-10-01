name: a PREG_OFFSET_CAPTURE offset reads back through substr
description: >
  The offsets PREG_OFFSET_CAPTURE reports are byte offsets, so substr takes
  them directly and the pair round trips: the text at the reported offset is
  the text the match reported. mb_substr counts characters, so a byte offset
  has to be converted with mb_strlen(substr($s, 0, $offset)) first, which is
  the conversion PHP requires too.

  The multi-byte character sits before the first match rather than inside it,
  because RE2 keeps \w ASCII-only where PCRE's u modifier widens it; that
  divergence is documented separately and is not what this covers.
---
<?php

$subject = "maíl tit@example.com and bob@example.com now";

preg_match_all('/(\w+)@([\w.]+)/', $subject, $m, PREG_OFFSET_CAPTURE);

foreach ($m[0] as $match) {
	list($text, $offset) = $match;
	// The byte offset indexes the subject substr slices.
	var_dump($offset);
	var_dump(substr($subject, $offset, strlen($text)) === $text);
	// mb_substr counts characters, so the offset is converted first.
	$chars = mb_strlen(substr($subject, 0, $offset));
	var_dump(mb_substr($subject, $chars, mb_strlen($text)) === $text);
}

// A group's offset reads back the same way.
list($user, $userOffset) = $m[1][0];
var_dump($user);
var_dump($userOffset);
var_dump(substr($subject, $userOffset, strlen($user)));

// A match after a two-byte character starts at the byte it starts at, which is
// one more than the character count before it.
preg_match('/example/', $subject, $one, PREG_OFFSET_CAPTURE);
var_dump($one[0][1]);
var_dump(mb_strlen(substr($subject, 0, $one[0][1])));
var_dump(substr($subject, $one[0][1], 7));

// A group that did not participate reports -1 and no text.
preg_match('/(a)(b)?/', "a", $opt, PREG_OFFSET_CAPTURE | PREG_UNMATCHED_AS_NULL);
var_dump($opt[2]);
?>
---
int(6)
bool(true)
bool(true)
int(26)
bool(true)
bool(true)
string(3) "tit"
int(6)
string(3) "tit"
int(10)
int(9)
string(7) "example"
array(2) {
  [0]=>
  NULL
  [1]=>
  int(-1)
}
