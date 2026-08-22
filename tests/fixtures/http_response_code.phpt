name: http_response_code
description: >
  http_response_code reports the status the response will be sent with, and
  stages it when given one. On the command line a script starts with no status,
  so the first report is false and the first set returns true for want of a
  previous status to hand back. A zero is not a status: PHP reads the call as
  the reporting one. Every call happens before any output, because PHP refuses
  to set a status once the response has started.
---
<?php

$seen = [];
$seen[] = http_response_code();
$seen[] = http_response_code(404);
$seen[] = http_response_code();
$seen[] = http_response_code(503);
$seen[] = http_response_code(0);
$seen[] = http_response_code();

echo json_encode($seen), "\n";
---
[false,true,404,404,503,503]
