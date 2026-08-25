name: array_column_flip
description: >
  array_column projects one field out of a list of rows, optionally keyed by
  another field, and array_flip exchanges keys and values.
---
<?php
$rows = array(
	array("id" => 3, "name" => "ann", "role" => "admin"),
	array("id" => 5, "name" => "bob", "role" => "user"),
	array("id" => 8, "name" => "cid", "role" => "user"),
);

// Without $index_key the result is a plain list.
echo json_encode(array_column($rows, "name")) . "\n";

// With one it is keyed by that column of each row.
echo json_encode(array_column($rows, "name", "id")) . "\n";

// A null $column_key selects whole rows, which re-keys the input.
echo json_encode(array_column($rows, null, "name")) . "\n";

// Rows missing the column are skipped; rows missing the index are appended.
$sparse = array(
	array("id" => 1, "name" => "ann"),
	array("id" => 2),
	array("name" => "cid"),
);
echo json_encode(array_column($sparse, "name")) . "\n";
echo json_encode(array_column($sparse, "name", "id")) . "\n";

// array_flip exchanges keys and values.
echo json_encode(array_flip(array("a", "b", "c"))) . "\n";
echo json_encode(array_flip(array("one" => 1, "two" => 2))) . "\n";

// A numeric-string value becomes the integer key it spells.
$flipped = array_flip(array("x" => "7", "y" => "z"));
var_dump(array_key_exists(7, $flipped));
echo json_encode($flipped) . "\n";

// The last of two equal values wins the key.
echo json_encode(array_flip(array("a" => "same", "b" => "same"))) . "\n";
---
["ann","bob","cid"]
{"3":"ann","5":"bob","8":"cid"}
{"ann":{"id":3,"name":"ann","role":"admin"},"bob":{"id":5,"name":"bob","role":"user"},"cid":{"id":8,"name":"cid","role":"user"}}
["ann","cid"]
{"1":"ann","2":"cid"}
{"a":0,"b":1,"c":2}
{"1":"one","2":"two"}
bool(true)
{"7":"x","z":"y"}
{"same":"b"}
