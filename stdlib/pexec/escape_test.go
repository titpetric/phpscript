package pexec

import (
	"testing"
)

// TestEscapeshellcmd pins the quote pairing against php. A quote with a partner
// later in the string is the author's own quoting and stays; a stray one, and
// one of the other kind found while a quote is open, gets a backslash. Every
// expectation here came from running the same string through php.
func TestEscapeshellcmd(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		in   string
		want string
	}{
		{in: "a; rm -rf /", want: `a\; rm -rf /`},
		{in: "echo 'a b'", want: "echo 'a b'"},
		{in: "it's", want: `it\'s`},
		{in: `a"b"c`, want: `a"b"c`},
		{in: "x*?[]", want: `x\*\?\[\]`},
		{in: "'a'b'", want: `'a'b\'`},
		{in: `a'b"c'd`, want: `a'b\"c'd`},
		{in: `"x", 'y'`, want: `"x", 'y'`},
		{in: "no meta", want: "no meta"},
		{in: "a|b&c", want: `a\|b\&c`},
		{in: `back\slash`, want: `back\\slash`},
		{in: "", want: ""},
	} {
		if got := phpEscapeshellcmd(test.in); got != test.want {
			t.Errorf("escapeshellcmd(%q) = %q, want %q", test.in, got, test.want)
		}
	}
}

// TestEscapeshellarg pins the quoting that is actually safe for an argument:
// everything is inside single quotes, where only the quote itself is special,
// and escapeshellarg splices that quote out and back in escaped.
func TestEscapeshellarg(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		in   string
		want string
	}{
		{in: "a b", want: `'a b'`},
		{in: "it's", want: `'it'\''s'`},
		{in: "", want: `''`},
		{in: "; rm -rf /", want: `'; rm -rf /'`},
	} {
		if got := phpEscapeshellarg(test.in); got != test.want {
			t.Errorf("escapeshellarg(%q) = %q, want %q", test.in, got, test.want)
		}
	}
}

// TestLastLine covers what system() answers with. The output has already gone
// to the page by then and lastLine keeps the last line as the stream passes
// and buffers nothing before it, and a stream that ended without a newline
// still has a last line.
func TestLastLine(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		writes []string
		want   string
	}{
		{name: "trailing newline", writes: []string{"a\nb\n"}, want: "b"},
		{name: "no trailing newline", writes: []string{"a\nb"}, want: "b"},
		{name: "split across writes", writes: []string{"a\nlast", " part\n"}, want: "last part"},
		{name: "one line", writes: []string{"only\n"}, want: "only"},
		{name: "nothing", writes: nil, want: ""},
		{name: "trailing blanks trimmed", writes: []string{"a\nb  \t\n"}, want: "b"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var tail lastLine
			for _, w := range test.writes {
				if _, err := tail.Write([]byte(w)); err != nil {
					t.Fatal(err)
				}
			}
			if got := tail.String(); got != test.want {
				t.Errorf("String() = %q, want %q", got, test.want)
			}
		})
	}
}
