name: php://input returns the raw request body
description: >
  The body the harness supplies is readable through file_get_contents and a
  handle, repeatedly, and json_decode covers the JSON API case. The php
  runner is opted out because the CLI SAPI has no request to carry a body.
runner:
  php: false
request:
  headers:
    Content-Type: application/json
  body: '{"a":1}'
---
<?php

echo file_get_contents("php://input"), "\n";
$decoded = json_decode(file_get_contents("php://input"), true);
var_dump($decoded["a"]);

$h = fopen("php://input", "r");
echo stream_get_contents($h), "\n";
fclose($h);

echo file_get_contents("php://input"), "\n";
var_dump(file_get_contents("php://memory"));
---
{"a":1}
int(1)
{"a":1}
{"a":1}
bool(false)
