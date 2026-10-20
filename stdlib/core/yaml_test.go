package core_test

import (
	"strings"
	"testing"
)

// TestYAMLDecode covers the shapes a document arrives in, which are the ones
// json_decode answers with: a mapping is an array keyed by its field names and
// a sequence is a list, so a settings file reads the same in either spelling.
func TestYAMLDecode(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_dump(yaml_decode("a: 1\nb: two\nc:\n  - 1\n  - 2\n"));`)

	want := "array(3) {\n" +
		"  [\"a\"]=>\n  int(1)\n" +
		"  [\"b\"]=>\n  string(3) \"two\"\n" +
		"  [\"c\"]=>\n  array(2) {\n    [0]=>\n    int(1)\n    [1]=>\n    int(2)\n  }\n" +
		"}\n"
	if got != want {
		t.Errorf("yaml_decode =\n%s\nwant:\n%s", got, want)
	}
}

// TestYAMLDecodeKeysAreSorted covers a decision of this runtime's own.
// The decoder answers a Go map and a Go map has no order, so reading one
// document twice would build two differently ordered arrays. A script reads a
// settings file by name, so sorting costs nothing and the same input decodes
// the same way every time.
func TestYAMLDecodeKeysAreSorted(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
foreach (yaml_decode("z: 1\nm: 2\na: 3\n") as $key => $value) {
	echo $key;
}`)

	if want := "amz"; got != want {
		t.Errorf("key order = %q, want %q", got, want)
	}
}

// TestYAMLDecodeWholeNumbersAreInts covers the one conversion that is not the
// decoder's. `max_pages: 400` compared against a count has to be an int; a
// float that happens to be whole would print as 400 and behave as 400.0.
func TestYAMLDecodeWholeNumbersAreInts(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
$doc = yaml_decode("whole: 400\nfraction: 1.5\nnegative: -7\nbig: 9007199254740993\n");
var_dump($doc["whole"], $doc["fraction"], $doc["negative"], $doc["big"]);
var_dump($doc["whole"] === 400);`)

	want := "int(400)\nfloat(1.5)\nint(-7)\nint(9007199254740993)\nbool(true)\n"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestYAMLDecodeRefusesWhatItCannotRead pins the divergence this binding
// carries: invalid input raises an error where php's yaml_decode answers null.
// A settings file that does not parse is a configuration the caller has to know
// about, and a null reads as an empty document.
func TestYAMLDecodeRefusesWhatItCannotRead(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
try {
	yaml_decode("a:\n\tb: 1\n");
	echo "no error";
} catch (Exception $e) {
	echo "caught";
}`)

	if want := "caught"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestYAMLEncode covers what the encoder writes back: an array that is a list
// becomes a sequence, any other array a mapping in key order, and a scalar its
// own plain spelling.
func TestYAMLEncode(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
echo yaml_encode(["b" => 2, "a" => 1]);
echo "---\n";
echo yaml_encode([1, 2, 3]);
echo "---\n";
echo yaml_encode("plain");`)

	want := "b: 2\na: 1\n---\n- 1\n- 2\n- 3\n---\nplain\n"
	if got != want {
		t.Errorf("yaml_encode = %q, want %q", got, want)
	}
}

// TestYAMLRoundTrip covers the pair together, the path a script storing a
// value and reading it back does. The encoder keeps no JSON conversion of its
// own: json_encode builds an ordered object that only the JSON marshaller
// knows how to write, and handing one to the YAML marshaller produced an empty
// document.
func TestYAMLRoundTrip(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
$value = ["name" => "site", "pages" => 400, "tags" => ["a", "b"]];
$back = yaml_decode(yaml_encode($value));
var_dump($back["name"], $back["pages"], $back["tags"][1]);
echo yaml_encode($back) === yaml_encode($back) ? "stable" : "unstable";`)

	want := "string(4) \"site\"\nint(400)\nstring(1) \"b\"\nstable"
	if !strings.HasSuffix(got, "stable") || got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}
