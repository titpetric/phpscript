name: token_get_all
description: >
  Exercise token_get_all() from PHP the way minitpl's compiler does in
  _split_exp(): foreach with keys, is_array() on the elements, $tok[0] compared
  against the T_* constants, $tok[1] / $tok[2] reads, count(), and writing an
  element back into both the token and the token list. The tokenizer returns
  native Go slices ([]any of []any), so this fixture pins the PHP-visible
  behaviour of that shape.
---
<?php
$code = str_replace(".", "__1", "<" . "?php if (\$this->_vars.user) { ?" . ">");
$tokens = token_get_all($code);

echo "count:" . count($tokens) . "\n";

$variables = array();
$arrows = 0;
foreach ($tokens as $k => $tok) {
	if (is_array($tok)) {
		echo "arr:" . count($tok) . ":" . token_name($tok[0]) . ":" . $tok[1] . ":" . $tok[2] . "\n";
		if ($tok[0] == T_OBJECT_OPERATOR) {
			$arrows = $arrows + 1;
		}
		if ($tok[0] == T_VARIABLE) {
			$variables[] = str_replace("__1", ".", $tok[1]);
		}
		// minitpl rewrites the id in place and stores the token back.
		$tok[0] = token_name($tok[0]);
		$tokens[$k] = $tok;
	} else {
		echo "chr:" . $tok . "\n";
	}
}

echo "arrows:" . $arrows . "\n";
echo "vars:" . implode(",", $variables) . "\n";
echo "rewritten:" . $tokens[0][0] . "\n";
---
count:13
arr:3:T_OPEN_TAG:<?php:1
arr:3:T_WHITESPACE: :1
arr:3:T_IF:if:1
arr:3:T_WHITESPACE: :1
chr:(
arr:3:T_VARIABLE:$this:1
arr:3:T_OBJECT_OPERATOR:->:1
arr:3:T_STRING:_vars__1user:1
chr:)
arr:3:T_WHITESPACE: :1
chr:{
arr:3:T_WHITESPACE: :1
arr:3:T_CLOSE_TAG:?>:1
arrows:1
vars:$this
rewritten:T_OPEN_TAG
