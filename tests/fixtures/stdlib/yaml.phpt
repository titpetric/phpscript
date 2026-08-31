name: yaml_decode reads a settings file into arrays
description: >
  A mapping decodes to an array keyed by its field names, a sequence to a
  list, and a whole number to an int - which is what a `max_pages: 400`
  compared against a count has to be. Keys read back sorted and write out in
  the order the array set them.

  The semantics are Go's, from goccy/go-yaml: `yes` is the string "yes", not
  a bool, and the round trip is what that library round-trips. There is no
  PHP counterpart to hold this to - YAML is a PECL extension there - so the
  php runtime is skipped.
runner:
  php: false
---
<?php

$settings = yaml_decode("crawler:\n  base_url: http://localhost:8080\n  max_pages: 400\n  enabled: true\n");
print_r($settings);
var_dump($settings["crawler"]["max_pages"]);
var_dump($settings["crawler"]["enabled"]);

// A sequence is a list, and the values inside it are converted too.
print_r(yaml_decode("ports:\n  - 80\n  - 8080\n"));

// Scalars at the top level, and the empty document.
var_dump(yaml_decode("42"));
var_dump(yaml_decode("3.5"));
var_dump(yaml_decode("yes"));
var_dump(yaml_decode(""));

// A comment is not data.
print_r(yaml_decode("# only a comment\nkey: value\n"));

// Invalid input raises rather than answering null, the way json_decode does.
try {
	yaml_decode("key: [unclosed\n");
	echo "accepted\n";
} catch (Throwable $e) {
	echo "refused\n";
}

// The round trip: what yaml_encode writes, yaml_decode reads back.
$row = array("name" => "Anna", "hours" => 7, "tags" => array("a", "b"));
$text = yaml_encode($row);
echo $text;
print_r(yaml_decode($text));
?>
---
Array
(
    [crawler] => Array
        (
            [base_url] => http://localhost:8080
            [enabled] => 1
            [max_pages] => 400
        )

)
int(400)
bool(true)
Array
(
    [ports] => Array
        (
            [0] => 80
            [1] => 8080
        )

)
int(42)
float(3.5)
string(3) "yes"
NULL
Array
(
    [key] => value
)
refused
name: Anna
hours: 7
tags:
- a
- b
Array
(
    [hours] => 7
    [name] => Anna
    [tags] => Array
        (
            [0] => a
            [1] => b
        )

)
