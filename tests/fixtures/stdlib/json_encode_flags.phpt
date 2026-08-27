name: json_encode takes no flags and does not escape a forward slash
description: >
  php escapes a forward slash unless JSON_UNESCAPED_SLASHES is passed; this
  runtime writes it as itself and defines no JSON_* constant. php cannot run
  the fixture, since the expectations are the divergence.
runner:
  php: false
---
<?php

echo json_encode(array("path" => "a/b")), "\n";
echo json_encode("https://example.com/x"), "\n";

var_dump(defined("JSON_UNESCAPED_SLASHES"));
var_dump(defined("JSON_PRETTY_PRINT"));
var_dump(defined("JSON_THROW_ON_ERROR"));

try {
	json_encode(array(1), 128);
} catch (Throwable $e) {
	echo "refused\n";
}
---
{"path":"a/b"}
"https://example.com/x"
bool(false)
bool(false)
bool(false)
refused
