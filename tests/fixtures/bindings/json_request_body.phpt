name: decoding a POST body without a string of it
description: >
  How a request body reaches a JSON\Decoder: fopen("php://input") is the body
  as a stream, and the decoder reads it. This is what a JSON API endpoint does
  instead of file_get_contents("php://input") followed by json_decode, and it
  is why the decoder takes a reader rather than a string. The harness supplies
  the body, which the php cli SAPI has no equivalent of.
runner:
  php: false

request:
  headers:
    Content-Type: application/json
  body: '{"name":"Anna","hours":7.5,"tags":["billable","design"],"note":null}'
---
<?php

$body = fopen("php://input", "r");
$request = new JSON\Decoder($body);
$row = $request->decode();

// What arrives is an ordinary PHP array: countable, indexable, iterable.
var_dump(is_array($row));
var_dump(count($row));
var_dump($row["name"]);
var_dump($row["hours"]);
var_dump($row["note"]);
var_dump($row["tags"][1]);

// The keys are the document's, in the order the document wrote them. This is
// the assertion that says so: a decoder building a Go map would answer these
// in a different order on every run.
print_r(array_keys($row));

// The values, listed by sorted key, so this comparison says nothing about
// order and the one above says everything about it.
$keys = array_keys($row);
sort($keys);
foreach ($keys as $key) {
    $value = $row[$key];
    echo $key, ": ", is_array($value) ? implode(",", $value) : var_export($value, true), "\n";
}

// The body held one value, so there is nothing left to read.
var_dump($request->more());

// The answer goes out the same way: an encoder over php://output writes the
// response as it is built, with no string of it in between.
$response = new JSON\Encoder(fopen("php://output", "w"));
$response->encode(array(
	"ok" => true,
	"echo" => $row["name"],
	"hours" => $row["hours"],
));
?>
---
bool(true)
int(4)
string(4) "Anna"
float(7.5)
NULL
string(6) "design"
Array
(
    [0] => name
    [1] => hours
    [2] => tags
    [3] => note
)
hours: 7.5
name: 'Anna'
note: NULL
tags: billable,design
bool(false)
{"ok":true,"echo":"Anna","hours":7.5}
