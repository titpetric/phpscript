name: token_get_all over interpolated strings
description: >
  A double-quoted literal that embeds an expression is not one token. It opens
  with a bare quote, reports each literal run as T_ENCAPSED_AND_WHITESPACE with
  its escapes still written as escapes, reports simple syntax as the tokens it
  is made of, opens complex syntax with T_CURLY_OPEN and reports the expression
  inside as ordinary PHP, and closes with the quote. A literal that embeds
  nothing, and every single-quoted literal, stays one
  T_CONSTANT_ENCAPSED_STRING.
---
<?php

$cases = array(
	"<" . "?php \"a\$b c\";",
	"<" . "?php \"x{\$a['k']}y\";",
	"<" . "?php \"\$o->p!\";",
	"<" . "?php \"\$a[k]\";",
	"<" . "?php \"\$a[0]\";",
	"<" . "?php \"\$a[\$i]\";",
	"<" . "?php \"a\\nb\$c\";",
	"<" . "?php \"plain\";",
	"<" . "?php 'no \$x';",
	"<" . "?php \"\\\$a\";",
);

foreach ($cases as $src) {
	echo "--- ", $src, "\n";
	foreach (token_get_all($src) as $tok) {
		if (is_array($tok)) {
			echo "  ", token_name($tok[0]), " |", $tok[1], "|\n";
		} else {
			echo "  CHAR |", $tok, "|\n";
		}
	}
}
---
--- <?php "a$b c";
  T_OPEN_TAG |<?php |
  CHAR |"|
  T_ENCAPSED_AND_WHITESPACE |a|
  T_VARIABLE |$b|
  T_ENCAPSED_AND_WHITESPACE | c|
  CHAR |"|
  CHAR |;|
--- <?php "x{$a['k']}y";
  T_OPEN_TAG |<?php |
  CHAR |"|
  T_ENCAPSED_AND_WHITESPACE |x|
  T_CURLY_OPEN |{|
  T_VARIABLE |$a|
  CHAR |[|
  T_CONSTANT_ENCAPSED_STRING |'k'|
  CHAR |]|
  CHAR |}|
  T_ENCAPSED_AND_WHITESPACE |y|
  CHAR |"|
  CHAR |;|
--- <?php "$o->p!";
  T_OPEN_TAG |<?php |
  CHAR |"|
  T_VARIABLE |$o|
  T_OBJECT_OPERATOR |->|
  T_STRING |p|
  T_ENCAPSED_AND_WHITESPACE |!|
  CHAR |"|
  CHAR |;|
--- <?php "$a[k]";
  T_OPEN_TAG |<?php |
  CHAR |"|
  T_VARIABLE |$a|
  CHAR |[|
  T_STRING |k|
  CHAR |]|
  CHAR |"|
  CHAR |;|
--- <?php "$a[0]";
  T_OPEN_TAG |<?php |
  CHAR |"|
  T_VARIABLE |$a|
  CHAR |[|
  T_NUM_STRING |0|
  CHAR |]|
  CHAR |"|
  CHAR |;|
--- <?php "$a[$i]";
  T_OPEN_TAG |<?php |
  CHAR |"|
  T_VARIABLE |$a|
  CHAR |[|
  T_VARIABLE |$i|
  CHAR |]|
  CHAR |"|
  CHAR |;|
--- <?php "a\nb$c";
  T_OPEN_TAG |<?php |
  CHAR |"|
  T_ENCAPSED_AND_WHITESPACE |a\nb|
  T_VARIABLE |$c|
  CHAR |"|
  CHAR |;|
--- <?php "plain";
  T_OPEN_TAG |<?php |
  T_CONSTANT_ENCAPSED_STRING |"plain"|
  CHAR |;|
--- <?php 'no $x';
  T_OPEN_TAG |<?php |
  T_CONSTANT_ENCAPSED_STRING |'no $x'|
  CHAR |;|
--- <?php "\$a";
  T_OPEN_TAG |<?php |
  T_CONSTANT_ENCAPSED_STRING |"\$a"|
  CHAR |;|
