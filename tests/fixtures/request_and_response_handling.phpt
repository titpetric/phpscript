---
name: request and response handling
description: Reads GET, POST, cookie, command-line, and stdin request data and sets response headers.

request:
  args:
    name: Alice
  get:
    greeting: Hello
  post:
    message: "from the form"
  cookie:
    session: abc123
  stdin: "additional input"

response:
  headers:
    X-Test-Result: passed
    Content-Type: text/plain
---
<?php

$name = isset($argv[1]) ? $argv[1] : 'Unknown';
$greeting = isset($_GET['greeting']) ? $_GET['greeting'] : '';
$message = isset($_POST['message']) ? $_POST['message'] : '';
$session = isset($_COOKIE['session']) ? $_COOKIE['session'] : '';
$stdin = trim(stream_get_contents(STDIN));

header('X-Test-Result: passed');
header('Content-Type: text/plain');

echo $greeting . ", " . $name . ": " . $message . " (" . $session . ") " . $stdin;
?>
---
Hello, Alice: from the form (abc123) additional input
