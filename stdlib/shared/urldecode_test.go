package shared_test

import (
	"testing"

	"github.com/titpetric/phpscript/stdlib/shared"
)

// TestURLDecode covers urldecode(): the form encoding a browser posts. Every
// want comes off `var_export(urldecode($in), true)` under php 8. The decoder
// is lenient because php is: `%zz` and a truncated `%4` decode to themselves,
// where net/url refuses the input and drops the whole pair, losing a field the
// browser did send.
func TestURLDecode(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		in   string
		want string
	}{
		{in: "plain", want: "plain"},
		{in: "", want: ""},
		{in: "a+b", want: "a b"},
		{in: "a%20b", want: "a b"},
		{in: "a%2Bb", want: "a+b"},
		{in: "%41%42", want: "AB"},
		{in: "%7e", want: "~"},
		{in: "%7E", want: "~"},
		{in: "%zz", want: "%zz"},
		{in: "a%zzb", want: "a%zzb"},
		{in: "%4", want: "%4"},
		{in: "a%2", want: "a%2"},
		{in: "100%", want: "100%"},
		{in: "%%41", want: "%A"},
	} {
		t.Run(test.in, func(t *testing.T) {
			if got := shared.URLDecode(test.in); got != test.want {
				t.Errorf("URLDecode(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

// TestRawURLDecode covers rawurldecode(): RFC 3986, where `+` is a plus and
// only `%20` is a space. It is the one difference from urldecode(), so a path
// segment holding a plus sign survives being decoded.
func TestRawURLDecode(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		in   string
		want string
	}{
		{in: "plain", want: "plain"},
		{in: "", want: ""},
		{in: "a+b", want: "a+b"},
		{in: "a%20b", want: "a b"},
		{in: "a%2Bb", want: "a+b"},
		{in: "%41%42", want: "AB"},
		{in: "%7e", want: "~"},
		{in: "%zz", want: "%zz"},
		{in: "%4", want: "%4"},
		{in: "%%41", want: "%A"},
	} {
		t.Run(test.in, func(t *testing.T) {
			if got := shared.RawURLDecode(test.in); got != test.want {
				t.Errorf("RawURLDecode(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}

	// The two agree on everything but the plus.
	for _, in := range []string{"plain", "a%20b", "%41%42", "%zz", "100%"} {
		if shared.URLDecode(in) != shared.RawURLDecode(in) {
			t.Errorf("URLDecode(%q) = %q and RawURLDecode(%q) = %q differ on an input holding no plus",
				in, shared.URLDecode(in), in, shared.RawURLDecode(in))
		}
	}
}
