name: JSON\Encoder and JSON\Decoder over a stream
description: >
  The pair as Go's json.Encoder and json.Decoder: an encoder writes a value and
  a newline after it, a decoder reads one value per call, and more() is what
  ends the loop. A stream of values back to back is the shape this exists for -
  json_encode and json_decode would need the whole document as a string first.
  php has no such classes, so it sits this one out, and the expected block is
  what this runtime answers rather than what php answers.
runner:
  php: false
---
<?php

// An encoder writes a value and the newline that separates it from the next,
// which is what makes a file of them readable one at a time.
$path = "json_stream.tmp";
$out = fopen($path, "w");
$enc = new JSON\Encoder($out);
$enc->encode(array("id" => 1, "name" => "Anna"));
$enc->encode(array("id" => 2, "name" => "Pieter"));
$enc->encode(array(1, 2, 3));
$enc->encode("a bare string is a value too");
$enc->encode(null);
fclose($out);

echo file_get_contents($path);

// One call, one value, in the order they were written. more() is the loop's
// condition: the end of the stream is not a value, and null is.
echo "\n";
$in = fopen($path, "r");
$dec = new JSON\Decoder($in);
$seen = 0;
while ($dec->more()) {
	$value = $dec->decode();
	$seen = $seen + 1;
	echo $seen, ": ", var_export($value, true), "\n";
}
fclose($in);
var_dump($seen);
var_dump($dec->more());

// A whole number is an int and a fractional one is a float, the way
// json_decode reads them, so a row written back out reads as it arrived.
echo "\n";
$path2 = "json_stream_numbers.tmp";
file_put_contents($path2, '{"hours":7,"rate":92.5,"big":9007199254740993}');
$dec2 = new JSON\Decoder(fopen($path2, "r"));
$row = $dec2->decode();
var_dump($row["hours"]);
var_dump($row["rate"]);
var_dump($row["big"]);

// The keys are the document's, in its order.
print_r(array_keys($row));

// set_indent makes the encoder write a value across lines.
echo "\n";
$pretty = new JSON\Encoder(fopen("php://output", "w"));
$pretty->set_indent("", "  ");
$pretty->encode(array("a" => 1, "b" => array("c" => 2)));

unlink($path);
unlink($path2);
?>
---
{"id":1,"name":"Anna"}
{"id":2,"name":"Pieter"}
[1,2,3]
"a bare string is a value too"
null

1: array (
  'id' => 1,
  'name' => 'Anna',
)
2: array (
  'id' => 2,
  'name' => 'Pieter',
)
3: array (
  0 => 1,
  1 => 2,
  2 => 3,
)
4: 'a bare string is a value too'
5: NULL
int(5)
bool(false)

int(7)
float(92.5)
int(9007199254740993)
Array
(
    [0] => hours
    [1] => rate
    [2] => big
)

{
  "a": 1,
  "b": {
    "c": 2
  }
}
