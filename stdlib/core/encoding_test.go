package core

import (
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestPHPBase64Encode covers the standard alphabet with '=' padding, the only
// base64 php's encoder writes.
func TestPHPBase64Encode(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{in: "", want: ""},
		{in: "Hello", want: "SGVsbG8="},
		{in: "ab", want: "YWI="},
		{in: "abc", want: "YWJj"},
	} {
		if got := phpBase64Encode(tc.in); got != tc.want {
			t.Errorf("base64_encode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestPHPBase64Decode covers both of php's decode modes, written out here
// because encoding/base64 has neither.
//
// Lenient mode drops every character outside the alphabet, padding included,
// and decodes what is left even when its length is not a multiple of four.
// Strict mode reports a character outside the alphabet by answering false, so
// the return type is any: php's contract is string|false.
func TestPHPBase64Decode(t *testing.T) {
	for _, tc := range []struct {
		name   string
		in     string
		strict []any
		want   any
	}{
		{name: "padded", in: "SGVsbG8=", want: "Hello"},
		{name: "whitespace is skipped", in: "SGVs bG8=", want: "Hello"},
		{name: "whitespace is skipped in strict mode too", in: "SGVs\nbG8=", strict: []any{true}, want: "Hello"},
		{name: "lenient mode drops a stray character", in: "SGVsbG8*", want: "Hello"},
		{name: "strict mode refuses a stray character", in: "SGVsbG8*", strict: []any{true}, want: false},
		{name: "strict mode refuses data after the padding", in: "SGVs=bG8", strict: []any{true}, want: false},
		{name: "strict mode refuses a lone trailing character", in: "SGVsbG8=Q", strict: []any{true}, want: false},
		{name: "the empty string", in: "", want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := phpBase64Decode(tc.in, tc.strict...); got != tc.want {
				t.Errorf("base64_decode(%q, %v) = %#v, want %#v", tc.in, tc.strict, got, tc.want)
			}
		})
	}
}

// TestPHPBin2hex covers the lowercase, two-digits-per-byte spelling, with
// phpHex2bin reading it back. An odd length or a character outside the hex
// alphabet answers false, php's string|false contract again.
func TestPHPBin2hex(t *testing.T) {
	if got, want := phpBin2hex("AB"), "4142"; got != want {
		t.Errorf("bin2hex(%q) = %q, want %q", "AB", got, want)
	}
	if got, want := phpBin2hex("\xff\x00"), "ff00"; got != want {
		t.Errorf("bin2hex of two bytes = %q, want %q", got, want)
	}

	if got := phpHex2bin("4142"); got != "AB" {
		t.Errorf("hex2bin(%q) = %#v, want %q", "4142", got, "AB")
	}
	if got := phpHex2bin("41a"); got != false {
		t.Errorf("hex2bin of an odd-length string = %#v, want false", got)
	}
	if got := phpHex2bin("41zz"); got != false {
		t.Errorf("hex2bin of a non-hex character = %#v, want false", got)
	}
}

// TestURLEncode covers the two tables the encoders keep. Go's url.QueryEscape
// leaves '~' literal where php escapes it in the form spelling, so neither
// encoder passes through the standard library: urlencode writes a space as '+'
// and '~' as '%7E', rawurlencode writes '%20' and leaves '~' alone.
func TestURLEncode(t *testing.T) {
	for _, tc := range []struct {
		in   string
		form string
		raw  string
	}{
		{in: "a b~", form: "a+b%7E", raw: "a%20b~"},
		{in: "safe-_.", form: "safe-_.", raw: "safe-_."},
		{in: "k=v&x", form: "k%3Dv%26x", raw: "k%3Dv%26x"},
		{in: "", form: "", raw: ""},
	} {
		if got := phpURLEncode(tc.in); got != tc.form {
			t.Errorf("urlencode(%q) = %q, want %q", tc.in, got, tc.form)
		}
		if got := phpRawURLEncode(tc.in); got != tc.raw {
			t.Errorf("rawurlencode(%q) = %q, want %q", tc.in, got, tc.raw)
		}
	}
}

// TestURLDecode covers the decoders, which live in stdlib/shared because the
// runner builds $_GET and $_POST with the same rules. Two implementations would
// let a form arrive one way through the superglobals and another through
// parse_str. urldecode reads '+' as a space; rawurldecode leaves it alone.
func TestURLDecode(t *testing.T) {
	if got, want := phpURLDecode("a+b%7E"), "a b~"; got != want {
		t.Errorf("urldecode(%q) = %q, want %q", "a+b%7E", got, want)
	}
	if got, want := phpRawURLDecode("a+b%20"), "a+b "; got != want {
		t.Errorf("rawurldecode(%q) = %q, want %q", "a+b%20", got, want)
	}
	if got, want := phpURLDecode("incomplete%"), "incomplete%"; got != want {
		t.Errorf("urldecode of an incomplete sequence = %q, want %q", got, want)
	}
}

// TestPHPHTTPBuildQuery covers joining pairs into a query string: both halves
// are urlencoded, so a nested key arrives as the escaped key%5Bsub%5D, and the
// insertion order of the array is the order of the pairs. A null entry is
// dropped, with no empty value written for it.
func TestPHPHTTPBuildQuery(t *testing.T) {
	data := model.NewArray()
	data.Set("a", int64(1))
	nested := model.NewArray()
	nested.Set("c", int64(2))
	data.Set("b", nested)

	if got, want := phpHTTPBuildQuery(data), "a=1&b%5Bc%5D=2"; got != want {
		t.Errorf("http_build_query() = %q, want %q", got, want)
	}

	flags := model.NewArray()
	flags.Set("on", true)
	flags.Set("off", false)
	flags.Set("gone", nil)
	if got, want := phpHTTPBuildQuery(flags), "on=1&off=0"; got != want {
		t.Errorf("http_build_query() of booleans = %q, want %q", got, want)
	}
}

// TestPHPParseStr covers the inverse of http_build_query, and the decoder
// behind $_GET and $_POST. php's bracket syntax nests, and every value decodes
// as a string, so a=1 reads back as the string "1".
//
// The setter is nil when a script omitted the by-reference argument. php raises
// an ArgumentCountError for that call; this runtime pads a short call, so the
// decode is skipped and nothing is written nowhere.
func TestPHPParseStr(t *testing.T) {
	var got any
	phpParseStr("a[b]=1&c=2", func(value any) { got = value })

	array, ok := got.(*model.Array)
	if !ok {
		t.Fatalf("parse_str wrote %T, want *model.Array", got)
	}
	if value, _ := array.Get("c"); value != "2" {
		t.Errorf("c decoded as %#v, want the string \"2\"", value)
	}

	nested, _ := array.Get("a")
	inner, ok := nested.(*model.Array)
	if !ok {
		t.Fatalf("a[b] decoded to %T, want *model.Array", nested)
	}
	if value, _ := inner.Get("b"); value != "1" {
		t.Errorf("a[b] decoded as %#v, want the string \"1\"", value)
	}

	phpParseStr("a=1", nil)
}
