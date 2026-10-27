package core_test

import (
	"testing"
)

// TestMultibyteLength covers the whole reason the family exists: mb_strlen
// counts characters where strlen counts bytes and "héllo" is five to one and
// six to the other.
func TestMultibyteLength(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_dump(mb_strlen("héllo"), strlen("héllo"), mb_strlen(""), mb_strlen("abc"));`)

	want := "int(5)\nint(6)\nint(0)\nint(3)\n"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestMultibyteSubstr covers the offsets, which are in characters: a negative
// start counts back from the end, a missing length runs to it, and a start past
// the end is the empty string and no error.
func TestMultibyteSubstr(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_dump(mb_substr("héllo", 1, 3), mb_substr("héllo", -2), mb_substr("héllo", 1), mb_substr("héllo", 10));
var_dump(mb_substr("héllo", 0, 0), mb_substr("héllo", 1, -1), mb_substr("héllo", -10));`)

	want := "string(4) \"éll\"\nstring(2) \"lo\"\nstring(5) \"éllo\"\nstring(0) \"\"\n" +
		"string(0) \"\"\nstring(4) \"éll\"\nstring(6) \"héllo\"\n"
	if got != want {
		t.Errorf("mb_substr = %q, want %q", got, want)
	}
}

// TestMultibyteStrpos covers the position, which is a character offset and not
// a byte one, and the false a needle nothing holds answers with. The offset
// argument is where the search starts, also counted in characters.
func TestMultibyteStrpos(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_dump(mb_strpos("héllo", "l"), mb_strpos("héllo", "z"), mb_strpos("héllo", "l", 3));
var_dump(mb_strpos("héllo", "h"), mb_strpos("héllo", "é"), mb_strrpos("héllo", "l"));`)

	want := "int(2)\nbool(false)\nint(3)\nint(0)\nint(1)\nint(3)\n"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestMultibyteStrSplit covers the chunking, which is in characters: a
// multibyte character is never cut in half and the last chunk is short rather
// than the first being wrong.
func TestMultibyteStrSplit(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_dump(mb_str_split("héllo", 2));
var_dump(mb_str_split("abc", 1));`)

	want := "array(3) {\n  [0]=>\n  string(3) \"hé\"\n  [1]=>\n  string(2) \"ll\"\n  [2]=>\n  string(1) \"o\"\n}\n" +
		"array(3) {\n  [0]=>\n  string(1) \"a\"\n  [1]=>\n  string(1) \"b\"\n  [2]=>\n  string(1) \"c\"\n}\n"
	if got != want {
		t.Errorf("mb_str_split =\n%s\nwant:\n%s", got, want)
	}
}

// TestMultibyteStrPad covers the padding, measured in characters, and the three
// directions. Padding to a width the string already exceeds leaves it alone,
// and mb_str_pad cuts a pad string longer than the gap to fit and the result is
// exactly the width the call names.
func TestMultibyteStrPad(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_dump(mb_str_pad("hé", 5, "-"), mb_str_pad("hé", 5, "-", STR_PAD_LEFT), mb_str_pad("hé", 5, "ab", STR_PAD_BOTH));
var_dump(mb_strlen(mb_str_pad("hé", 5, "-")), mb_str_pad("héllo", 2, "-"), mb_str_pad("hé", 4, "abc"));`)

	want := "string(6) \"hé---\"\nstring(6) \"---hé\"\nstring(6) \"ahéab\"\n" +
		"int(5)\nstring(6) \"héllo\"\nstring(5) \"héab\"\n"
	if got != want {
		t.Errorf("mb_str_pad = %q, want %q", got, want)
	}
}

// TestMultibyteCaseOfTheFirstCharacter covers mb_ucfirst and mb_lcfirst, which
// change one character and leave the rest: the character is multibyte and a
// byte-wise ucfirst would write a broken sequence where this writes É.
func TestMultibyteCaseOfTheFirstCharacter(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_dump(mb_ucfirst("école"), mb_lcfirst("École"), mb_ucfirst(""), mb_lcfirst(""));
var_dump(mb_ucfirst("abc"), mb_lcfirst("ABC"));`)

	want := "string(6) \"École\"\nstring(6) \"école\"\nstring(0) \"\"\nstring(0) \"\"\n" +
		"string(3) \"Abc\"\nstring(3) \"aBC\"\n"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestMultibyteCase covers the whole-string folds, which are the Unicode ones:
// É folds to é and back, where a byte-wise fold leaves it untouched.
func TestMultibyteCase(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_dump(mb_strtoupper("héllo"), mb_strtolower("HÉLLO"));`)

	want := "string(6) \"HÉLLO\"\nstring(6) \"héllo\"\n"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}
