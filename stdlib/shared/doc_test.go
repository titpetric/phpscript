package shared_test

import (
	"testing"

	"github.com/titpetric/phpscript/stdlib/shared"
)

// TestPair covers one already-decoded form field. Multipart values and the
// cookie header arrive decoded, so a Pair carries the name and the value as
// they are and parseKey reads the bracket path off the name afterwards: a name
// that held `%5B` before decoding nests, and a name that holds a literal
// bracket after decoding nests too.
func TestPair(t *testing.T) {
	t.Parallel()
	pairs := []shared.Pair{
		{Name: "a[b]", Value: "1"},
		{Name: "plain", Value: "x y"},
		{Name: "c[]", Value: "first"},
		{Name: "c[]", Value: "second"},
	}

	got := flatten(shared.ParsePairs(shared.Limits{}, pairs))
	want := "a.b=1 c.0=first c.1=second plain=x y"
	if got != want {
		t.Errorf("ParsePairs()\n got %q\nwant %q", got, want)
	}
}

// TestParsePairs covers the entry point for input that did not arrive as a
// query string. Order is the order ParsePairs applies pairs in, so two pairs
// naming one variable leave the last value, and the last assignment decides
// the shape: a scalar written over an array replaces it. The limits are
// php.ini's, and the parse drops a name past the nesting limit whole.
func TestParsePairs(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		pairs  []shared.Pair
		limits shared.Limits
		want   string
	}{
		{
			name:  "no pairs",
			pairs: nil,
		},
		{
			name:  "the last value of a repeated name",
			pairs: []shared.Pair{{Name: "a", Value: "1"}, {Name: "a", Value: "2"}},
			want:  "a=2",
		},
		{
			name:  "an array written over a scalar",
			pairs: []shared.Pair{{Name: "a", Value: "1"}, {Name: "a[b]", Value: "2"}},
			want:  "a.b=2",
		},
		{
			name:  "a scalar written over an array",
			pairs: []shared.Pair{{Name: "a[b]", Value: "1"}, {Name: "a", Value: "2"}},
			want:  "a=2",
		},
		{
			// The dot inside the bracket survives as part of the key, and
			// flatten joins a nested path with a dot of its own.
			name:  "a name is mangled at the top level alone",
			pairs: []shared.Pair{{Name: "a.b", Value: "1"}, {Name: "e[f.g]", Value: "2"}},
			want:  "a_b=1 e.f.g=2",
		},
		{
			name:  "an empty name is dropped",
			pairs: []shared.Pair{{Name: "", Value: "1"}, {Name: "a", Value: "2"}},
			want:  "a=2",
		},
		{
			name:   "pairs past the variable limit are dropped",
			pairs:  []shared.Pair{{Name: "a", Value: "1"}, {Name: "b", Value: "2"}, {Name: "c", Value: "3"}},
			limits: shared.Limits{MaxVars: 2},
			want:   "a=1 b=2",
		},
		{
			name:   "a name past the nesting limit is dropped whole",
			pairs:  []shared.Pair{{Name: "a[b][c]", Value: "1"}, {Name: "d[e]", Value: "2"}},
			limits: shared.Limits{MaxNesting: 1},
			want:   "d.e=2",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := flatten(shared.ParsePairs(test.limits, test.pairs)); got != test.want {
				t.Errorf("ParsePairs()\n got %q\nwant %q", got, test.want)
			}
		})
	}
}

// TestFormDecodingIsOneImplementation covers the reason this package exists.
// runner builds $_GET and $_POST and stdlib's parse_str() reads a string, and
// both go through this decoder, so the nested array a request produces is the
// one parse_str() produces for the same bytes. Decoding runs before parseKey
// reads the brackets, so `k%5Ba%20b%5D=1` nests under the key `a b`, and a
// plus in a top-level name is a space by then and mangles to an underscore.
func TestFormDecodingIsOneImplementation(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		raw  string
		want string
	}{
		{raw: "k%5Ba%20b%5D=1", want: "k.a b=1"},
		{raw: "a+b=1", want: "a_b=1"},
		{raw: "a=%2B1", want: "a=+1"},
	} {
		t.Run(test.raw, func(t *testing.T) {
			if got := flatten(shared.ParseStr(test.raw, shared.Limits{})); got != test.want {
				t.Errorf("ParseStr(%q)\n got %q\nwant %q", test.raw, got, test.want)
			}
		})
	}

	// The string form is the pair form with the splitting done: an `&` field
	// with no `=` is a name with an empty value, and both paths agree on it.
	raw := shared.ParseStr("a&b=1", shared.Limits{})
	pairs := shared.ParsePairs(shared.Limits{}, []shared.Pair{{Name: "a"}, {Name: "b", Value: "1"}})
	if flatten(raw) != flatten(pairs) {
		t.Errorf("ParseStr() = %q and ParsePairs() = %q, want the same array", flatten(raw), flatten(pairs))
	}
}
