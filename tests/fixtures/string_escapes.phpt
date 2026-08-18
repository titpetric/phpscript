name: string escape sequences
description: >
  A double-quoted literal decodes the C-style escapes and the numeric forms
  (\x41, \101, \u{1F600}); a single-quoted one recognises only \\ and \', which
  is what keeps a Windows path and a single-quoted regex intact.
---
<?php

$bom = "\xEF\xBB\xBF";
echo "bom=" . strlen($bom) . "\n";
echo "octal=" . "\101\102" . " len=" . strlen("\101\102") . "\n";
echo "hex=" . "\x41\x9" . "|\n";
echo "unicode=" . "\u{48}\u{49}" . " len=" . strlen("\u{1F600}") . "\n";
echo "control=" . strlen("\n\t\r\v\f\e\0") . "\n";
echo "quoted=" . "a\\b\$c\"d" . "|\n";
echo "unknown=" . "\q\x\u" . "|\n";
echo 'single=a\nb\\c\'d' . "|\n";
echo 'path=C:\Users\name' . "|\n";
---
bom=3
octal=AB len=2
hex=A	|
unicode=HI len=4
control=7
quoted=a\b$c"d|
unknown=\q\x\u|
single=a\nb\c'd|
path=C:\Users\name|
