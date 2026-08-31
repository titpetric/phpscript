name: a request body reaches php://input, and STDIN stays empty
description: >
  request.body is the raw body a fixture states, and php://input is what reads
  it - which is how a JSON endpoint is tested without the body existing as a
  string first. STDIN is a separate stream and stays empty unless request.stdin
  fills it, so a fixture that states a body does not find it on stdin as well.
  The php column sits this one out: the cli SAPI maps php://input onto the
  process stdin, so the body would have to arrive there and the two streams
  could not be told apart.
runner:
  php: false

request:
  headers:
    Content-Type: application/json
  body: '{"id":7}'
---
<?php

// php://input is the body, and it rewinds: opening it twice reads it twice,
// the way php has since 5.6.
echo file_get_contents("php://input"), "\n";
echo file_get_contents("php://input"), "\n";

$handle = fopen("php://input", "r");
echo stream_get_contents($handle), "\n";
fclose($handle);

// STDIN is empty, not the body. An empty stream reads as "" and reports the
// end immediately rather than blocking.
var_dump(stream_get_contents(STDIN));

$in = fopen("php://stdin", "r");
var_dump($in);
?>
---
{"id":7}
{"id":7}
{"id":7}
string(0) ""
bool(false)
