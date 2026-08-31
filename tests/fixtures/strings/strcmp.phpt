name: strcmp and its three siblings
description: >
  The four byte comparisons, with the numbers php itself answers rather than a
  normalised -1/0/1: a disagreement answers the difference between the two
  bytes, and only a tie broken by length normalises. Covers unsigned byte order,
  the ASCII-only folding the case-insensitive pair does, and strncmp's reading
  of a length past the end of both strings or below zero.
---
<?php

// A disagreement answers the byte difference. "A" is 65 and "a" is 97, so the
// answer is -32 rather than -1, which is what a sort using the sign sees as
// "A sorts first" either way.
$pairs = array(
    array("a", "b"),
    array("b", "a"),
    array("a", "a"),
    array("abc", "abd"),
    array("A", "a"),
    array("Z", "a"),
    array("10", "9"),
    array("_", "A"),
);
foreach ($pairs as $pair) {
    $x = $pair[0];
    $y = $pair[1];
    echo "strcmp(", $x, ",", $y, ")=", strcmp($x, $y);
    echo " strcasecmp=", strcasecmp($x, $y), "\n";
}

// A tie broken by length normalises, whatever the length difference is.
echo "\n";
var_dump(strcmp("abc", "ab"));
var_dump(strcmp("a", "abcdefgh"));
var_dump(strcmp("abcdefgh", "a"));
var_dump(strcmp("", "a"));
var_dump(strcmp("", ""));

// Bytes compare unsigned: 255 sorts after 1. The bytes are built with chr()
// rather than written as "\xff", because a \x escape above 7f is one byte to
// php and to the flat stack but a re-encoded rune to the runtime engine - a
// difference about escapes, not about comparison.
echo "\n";
$high = "a" . chr(255) . "b";
$low = "a" . chr(1) . "b";
var_dump(strcmp($high, $low));

// The folding is ASCII only: A-umlaut and a-umlaut are the same letter in
// UTF-8, and these functions do not decode a rune to find that out.
var_dump(strcasecmp(chr(0xc3) . chr(0x84), chr(0xc3) . chr(0xa4)));
var_dump(strcasecmp("A" . chr(255) . "b", "a" . chr(1) . "b"));

// strncmp stops at $length; past the end of both it compares what is there,
// and at zero it compares nothing.
echo "\n";
var_dump(strncmp("abc", "abd", 2));
var_dump(strncmp("abc", "abd", 3));
var_dump(strncmp("abc", "abcdef", 10));
var_dump(strncmp("abc", "abd", 0));
var_dump(strncasecmp("ABC", "abd", 2));
var_dump(strncasecmp("ABC", "abd", 3));

// The sign is what a comparator uses, and it is stable across all four.
echo "\n";
$rows = array("01TEST04", "01TEST02", "01TEST10", "01TEST01");
usort($rows, function ($a, $b) {
    return strcmp($a, $b);
});
echo implode(" ", $rows), "\n";
?>
---
strcmp(a,b)=-1 strcasecmp=-1
strcmp(b,a)=1 strcasecmp=1
strcmp(a,a)=0 strcasecmp=0
strcmp(abc,abd)=-1 strcasecmp=-1
strcmp(A,a)=-32 strcasecmp=0
strcmp(Z,a)=-7 strcasecmp=25
strcmp(10,9)=-8 strcasecmp=-8
strcmp(_,A)=30 strcasecmp=-2

int(1)
int(-1)
int(1)
int(-1)
int(0)

int(254)
int(-32)
int(254)

int(0)
int(-1)
int(-1)
int(0)
int(0)
int(-1)

01TEST01 01TEST02 01TEST04 01TEST10
