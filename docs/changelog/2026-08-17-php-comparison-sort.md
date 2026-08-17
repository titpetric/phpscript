# sort() now orders values the way PHP compares them

**2026-08-17.** `sort()` and `rsort()` shipped in the [dbadmin migrations](./2026-08-17-dbadmin-migrations.md) change with a comparison that only knew about the runtime's number types: two `int`s or `float64`s compared numerically, everything else compared as strings. PHP's rule is wider, and the gap showed on the most ordinary input there is — a list that came out of `explode()`, where every element is a string:

```php
$parts = explode(",", "100,1,1000,10,9,20");
sort($parts);
echo implode(",", $parts);   // PHP: 1,9,10,20,100,1000
                             // was:  1,10,100,1000,20,9
```

PHP compares two numeric strings as numbers; phpscript compared them as text. The powers of ten alone (`"100,1,1000,10"`) hid it, since `1 < 10 < 100 < 1000` holds bytewise too — one `9` in the list is enough to separate the two orderings.

## The comparison

`phpCompare` in [stdlib.go](../../stdlib/stdlib.go) implements PHP 8's `<=>` and `sortLess` derives its boolean from it, so `sort`, `rsort` and any future `asort`/`ksort` share one rule:

| operands         | comparison                                             |
|------------------|--------------------------------------------------------|
| two arrays       | the shorter is smaller, then member by member          |
| array vs scalar  | the array is greater                                   |
| `null` vs string | the `null` becomes `""` and the two compare as strings |
| `null` or `bool` | both sides are cast to `bool`                          |
| numbers          | numerically, numeric strings included                  |
| anything else    | both sides are cast to string, compared bytewise       |

A numeric string is PHP's, not Go's: whitespace may surround an optional sign, digits, a fractional part and an exponent, so `" 12 "` is 12 and `"1."` is 1, while `"0x1A"`, `"1_000"`, `"INF"` and `"NaN"` are strings that sort as text. `strconv.ParseFloat` accepts all four, which is why the syntax is scanned before it is called.

Two integers compare as `int64` rather than through a `float64`, so ids near 2^63 keep their last digits. Past that, two integer strings that round to the same `float64` compare by digits, as PHP does — `"9223372036854775808"` is greater than `"9223372036854775807"`, while `"1e20"` stays equal to `"100000000000000000000"`.

## Verification

`TestPHPCompare` in [stdlib_internal_test.go](../../stdlib/stdlib_internal_test.go) pins 48 pairs to the value php 8.4 prints for `$x <=> $y`, and the `sort` fixture covers the `explode()` case end to end. Beyond the committed tests, the implementation was diffed against php 8.4 over every ordered pair drawn from a 61-value pool (3717 comparisons, byte-identical output) and over 3000 random lists.

One class of input cannot agree, in either direction: PHP's comparison is not transitive, and where it cycles the result depends on the sort algorithm rather than the comparison. `"0x1A"` is less than `"9"` (text), `"9"` is less than `" 12 "` (numbers), and `" 12 "` is less than `"0x1A"` (text again) — php itself returns four different orderings for the six permutations of those three values. Every pairwise answer matches; the arrangement of a cycle does not.
