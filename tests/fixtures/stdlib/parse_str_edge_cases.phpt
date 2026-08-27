name: parse_str follows PHP's bracket rules, sharp edges included
description: >
  The rules are a pile of special cases: mangling that applies to the
  top-level name only, trailing text discarded rather than rejected, and an
  unterminated first bracket meaning something different from a later one.
  Every expectation is php's own output.
---
<?php

// Mangling is top-level only: '.', ' ' and '[' become '_' there and survive
// inside brackets.
parse_str("a.b=1&c d=2&e[f.g]=3", $o);
echo implode(",", array_keys($o)), "\n";
echo $o["e"]["f.g"], "\n";

// An unterminated first bracket is one flat key; a later one ends the path.
parse_str("a[b=1&c[=2", $o);
echo implode(",", array_keys($o)), "\n";
parse_str("x[b][c=1", $o);
echo $x = $o["x"]["b"], "\n";

// Trailing text after a closed bracket is discarded.
parse_str("a[b]c=1&d[e]]=2", $o);
echo $o["a"]["b"], $o["d"]["e"], "\n";

// The last assignment decides the shape.
parse_str("a=1&a[b]=2", $o);
echo $o["a"]["b"], "\n";
parse_str("a[b]=1&a=2", $o);
echo $o["a"], "\n";

// Append follows the highest integer key.
parse_str("a[]=1&a[3]=x&a[]=y", $o);
echo implode(",", array_keys($o["a"])), "\n";

// Decoding happens before the brackets are read.
parse_str("k%5Ba%20b%5D=1", $o);
echo $o["k"]["a b"], "\n";

// A malformed escape stays literal, where net/url would drop the field.
parse_str("a=%zz&b=%41", $o);
echo $o["a"], ",", $o["b"], "\n";

// An empty variable name is dropped.
parse_str("=5&[]=6&keep=7", $o);
echo implode(",", array_keys($o)), "\n";

// Bracket keys follow the same canonical-integer rule as any array key.
parse_str("a[08]=x&a[8]=y", $o);
echo count($o["a"]), "\n";
---
a_b,c_d,e
3
a_b,c_
1
12
2
2
0,3,4
1
%zz,A
keep
2
