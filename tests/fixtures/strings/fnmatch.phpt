name: fnmatch
description: >
  fnmatch matches a string against a shell wildcard pattern, which is what
  filtering a record's fields by name prefix needs: the row's post_* keys and
  nothing else. Covers the wildcards, a bracket expression and its negation, an
  escaped underscore, and the two flags that change a decision the matcher makes
  on every byte, FNM_PATHNAME and FNM_CASEFOLD.
---
<?php

$row = [
    "post_id" => 12,
    "post_title" => "Hello world",
    "author_id" => 7,
    "post_date" => "2026-08-31",
    "id" => 99,
];

// match_fields keeps the entries whose key matches the pattern, in the order
// the row established.
function match_fields(array $row, string $pattern): array
{
    $matched = [];
    foreach ($row as $key => $value) {
        if (fnmatch($pattern, $key)) {
            $matched[$key] = $value;
        }
    }
    return $matched;
}

print_r(match_fields($row, "post_*"));
print_r(match_fields($row, "*_id"));
print_r(match_fields($row, "post_?d"));
print_r(match_fields($row, "nothing_*"));

var_dump(fnmatch("post_*", "post_id"));
var_dump(fnmatch("post_*", "author_id"));
var_dump(fnmatch("[ap]*_id", "author_id"));
var_dump(fnmatch("[!a]*_id", "author_id"));
var_dump(fnmatch("*.txt", "notes/todo.txt"));
var_dump(fnmatch("*.txt", "notes/todo.txt", FNM_PATHNAME));
var_dump(fnmatch("POST_*", "post_id", FNM_CASEFOLD));
var_dump(fnmatch("post\\_*", "post_id"));
?>
---
Array
(
    [post_id] => 12
    [post_title] => Hello world
    [post_date] => 2026-08-31
)
Array
(
    [post_id] => 12
    [author_id] => 7
)
Array
(
    [post_id] => 12
)
Array
(
)
bool(true)
bool(false)
bool(true)
bool(false)
bool(true)
bool(false)
bool(true)
bool(true)
