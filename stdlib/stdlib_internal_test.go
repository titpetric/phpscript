package stdlib

import (
	"fmt"
	"hash/crc32"
	"math"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestPHPCRC32 pins the table implementation to the standard library's result
// across the threshold that switches between them.
func TestPHPCRC32(t *testing.T) {
	inputs := []string{
		"",
		"a",
		"hello world",
		strings.Repeat("phpscript", 3),
		strings.Repeat("x", crc32NativeThreshold),
		strings.Repeat("x", crc32NativeThreshold+1),
		strings.Repeat("The quick brown fox. ", 512),
		"\x00\xff\xfe binary \x01",
	}
	for _, in := range inputs {
		want := int64(crc32.ChecksumIEEE([]byte(in)))
		if got := phpCRC32(in); got != want {
			t.Errorf("phpCRC32(%d bytes) = %d, want %d", len(in), got, want)
		}
	}
}

// TestHTMLSpecialCharsReplacer asserts the hoisted replacer still produces
// PHP's default (ENT_QUOTES-style) escaping.
func TestHTMLSpecialCharsReplacer(t *testing.T) {
	got := htmlSpecialCharsReplacer.Replace(`<a href="x">Tom & Jerry's</a>`)
	want := `&lt;a href=&quot;x&quot;&gt;Tom &amp; Jerry&#039;s&lt;/a&gt;`
	if got != want {
		t.Errorf("htmlspecialchars = %q, want %q", got, want)
	}
}

// TestPHPArrayMergeShapes covers both key regimes. array_merge always returns
// an *model.Array so the result stays appendable (see phpArrayMerge); wantList
// cases assert it came back list-shaped, i.e. dense int64 keys in order.
func TestPHPArrayMergeShapes(t *testing.T) {
	assoc := model.NewArray()
	assoc.Set("a", 1)
	assoc.Set("b", 2)

	list := model.NewArray()
	list.Append("x")
	list.Append("y")

	sparse := model.NewArray()
	sparse.Set(int64(3), "three")

	cases := []struct {
		name     string
		args     []any
		wantList []any
		wantMap  map[string]any
		wantKeys []any
	}{
		{
			name:     "two []string lists",
			args:     []any{[]string{"a", "b"}, []string{"c"}},
			wantList: []any{"a", "b", "c"},
		},
		{
			name:     "[]any and list Array",
			args:     []any{[]any{"q"}, list},
			wantList: []any{"q", "x", "y"},
		},
		{
			name:     "no arguments",
			args:     nil,
			wantList: []any{},
		},
		{
			name:     "nil argument",
			args:     []any{nil, []string{"a"}},
			wantList: []any{"a"},
		},
		{
			name:     "string keys keep the Array",
			args:     []any{assoc, []string{"tail"}},
			wantMap:  map[string]any{"a": 1, "b": 2},
			wantKeys: []any{"a", "b", int64(0)},
		},
		{
			name:     "map argument keeps the Array",
			args:     []any{map[string]any{"k": "v"}},
			wantMap:  map[string]any{"k": "v"},
			wantKeys: []any{"k"},
		},
		{
			name:     "sparse int keys keep the Array",
			args:     []any{sparse, []string{"tail"}},
			wantKeys: []any{int64(0), int64(1)},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := phpArrayMerge(tc.args...)
			arr, ok := got.(*model.Array)
			if !ok {
				t.Fatalf("array_merge returned %T, want *model.Array", got)
			}
			if tc.wantList != nil {
				if !arrayIsList(arr) {
					t.Fatalf("array_merge keys = %v, want the dense 0..n-1 list", arr.Keys())
				}
				var vals []any
				arr.Range(func(_, v any) bool { vals = append(vals, v); return true })
				if len(vals) != len(tc.wantList) {
					t.Fatalf("array_merge = %v, want %v", vals, tc.wantList)
				}
				for i := range vals {
					if vals[i] != tc.wantList[i] {
						t.Fatalf("array_merge = %v, want %v", vals, tc.wantList)
					}
				}
				return
			}
			for k, want := range tc.wantMap {
				if v, ok := arr.Get(k); !ok || v != want {
					t.Errorf("array_merge[%q] = %v (%v), want %v", k, v, ok, want)
				}
			}
			if tc.wantKeys != nil {
				keys := arr.Keys()
				if len(keys) != len(tc.wantKeys) {
					t.Fatalf("array_merge keys = %v, want %v", keys, tc.wantKeys)
				}
				for i := range keys {
					if keys[i] != tc.wantKeys[i] {
						t.Fatalf("array_merge keys = %v, want %v", keys, tc.wantKeys)
					}
				}
			}
		})
	}
}

// TestPHPArrayMergeCopiesInputs asserts array_merge produces a new array, as
// PHP does: writing to the result must not reach back into an argument.
func TestPHPArrayMergeCopiesInputs(t *testing.T) {
	src := []any{"a", "b"}
	out, ok := phpArrayMerge(src).(*model.Array)
	if !ok {
		t.Fatalf("array_merge returned %T, want *model.Array", out)
	}
	out.Set(int64(0), "changed")
	if src[0] != "a" {
		t.Errorf("array_merge aliased its argument: src = %v", src)
	}
}

// TestPHPArrayMergeResultIsAppendable is the reason array_merge does not
// return a []any for the all-lists case: a Go slice cannot grow through the
// interface value holding it, so `$x = array_merge($a, $b); $x[] = "z"` would
// be an error. Rule 4 of docs/allocation-performance.md.
func TestPHPArrayMergeResultIsAppendable(t *testing.T) {
	out, ok := phpArrayMerge([]string{"a"}, []string{"b"}).(*model.Array)
	if !ok {
		t.Fatalf("array_merge returned a non-appendable shape")
	}
	out.Append("z")
	if got := out.Len(); got != 3 {
		t.Fatalf("Len after append = %d, want 3", got)
	}
	if v, ok := out.Get(int64(2)); !ok || v != "z" {
		t.Errorf("appended element = %v (%v), want z", v, ok)
	}
}

// TestToString pins the strconv formatting against the fmt formatting it
// replaced.
func TestToString(t *testing.T) {
	values := []any{
		nil, "", "text", true, false,
		int64(0), int64(-1), int64(4096), int64(math.MaxInt64), int64(math.MinInt64),
		0, -1, 4096,
		0.0, 1.5, -0.25, 1e21, 1e-7, math.Inf(1), math.NaN(),
		[]string{"a"},
	}
	for _, v := range values {
		want := legacyToString(v)
		if got := toString(v); got != want {
			t.Errorf("toString(%#v) = %q, want %q", v, got, want)
		}
	}
}

func legacyToString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "1"
		}
		return ""
	case int64:
		return fmt.Sprintf("%d", x)
	case int:
		return fmt.Sprintf("%d", x)
	case float64:
		return fmt.Sprintf("%v", x)
	default:
		return fmt.Sprintf("%v", x)
	}
}

// TestToIntString pins leadingInt against the fmt.Sscanf("%d") behaviour it
// replaced, including the cases where a scan failed and left zero behind.
func TestToIntString(t *testing.T) {
	inputs := []string{
		"", "0", "42", " 42", "\t42", "42abc", "abc", "-7", "+7",
		"0x1f", "3.9", "  -12  ", "007", "12 34", "- 5", "1e3",
		"9223372036854775807", "-9223372036854775807",
		"99999999999999999999", "-99999999999999999999",
	}
	for _, in := range inputs {
		var want int64
		fmt.Sscanf(in, "%d", &want)
		if got := toInt(in); got != want {
			t.Errorf("toInt(%q) = %d, want %d", in, got, want)
		}
	}
	if got := toInt("-9223372036854775808"); got != 0 && got != math.MinInt64 {
		t.Errorf("toInt(MinInt64) = %d, want 0 or MinInt64", got)
	}
	// One deliberate divergence: Sscanf treats a newline as a terminator, so
	// it read "\n 42" as nothing. PHP's integer cast skips all leading
	// whitespace, which is what leadingInt does.
	if got := toInt("\n 42"); got != 42 {
		t.Errorf("toInt(%q) = %d, want 42", "\n 42", got)
	}
}

func TestPHPStrReplace(t *testing.T) {
	cases := []struct {
		search, replace, subject any
		want                     string
	}{
		{"a", "b", "banana", "bbnbnb"},
		{[]string{"a", "n"}, "-", "banana", "b-----"},
		{[]string{"a", "n"}, []string{"1", "2"}, "banana", "b12121"},
		{[]string{"a", "n"}, []string{"1"}, "banana", "b111"},
		{"a", 1, "banana", "b1n1n1"},
	}
	for _, tc := range cases {
		if got := phpStrReplace(tc.search, tc.replace, tc.subject); got != tc.want {
			t.Errorf("str_replace(%v, %v, %v) = %q, want %q", tc.search, tc.replace, tc.subject, got, tc.want)
		}
	}
}

func BenchmarkHTMLSpecialChars(b *testing.B) {
	const s = `<a href="/x?a=1&b=2">Tom & Jerry's</a>`
	b.Run("hoisted", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = htmlSpecialCharsReplacer.Replace(s)
		}
	})
	b.Run("legacy", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#039;")
			_ = r.Replace(s)
		}
	})
}

func BenchmarkArrayMerge(b *testing.B) {
	head := []any{"SELECT 1"}
	tail := []string{"a", "b", "c", "d"}

	b.Run("lists", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = phpArrayMerge(head, tail)
		}
	})
	b.Run("lists_legacy", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = legacyArrayMerge(head, tail)
		}
	})

	assoc := model.NewArray()
	assoc.Set("a", 1)
	assoc.Set("b", 2)
	b.Run("string_keys", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = phpArrayMerge(assoc, tail)
		}
	})
}

// legacyArrayMerge is the pre-fast-path implementation, kept so the benchmark
// measures the change rather than asserting it (see docs/allocation-performance.md).
func legacyArrayMerge(arrs ...any) *model.Array {
	size := 0
	for _, a := range arrs {
		n, _ := model.LenValues(a)
		size += n
	}
	out := model.NewArraySize(size)
	for _, a := range arrs {
		model.RangeValues(a, func(k, v any) bool {
			if _, isInt := k.(int64); isInt {
				out.Append(v)
			} else {
				out.Set(k, v)
			}
			return true
		})
	}
	return out
}

func BenchmarkCRC32(b *testing.B) {
	for _, size := range []int{8, 64, 256, 1024} {
		s := strings.Repeat("a", size)
		b.Run("table/"+itoa(size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = tableCRC32(s)
			}
		})
		b.Run("stdlib/"+itoa(size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = int64(crc32.ChecksumIEEE([]byte(s)))
			}
		})
	}
}

func tableCRC32(s string) int64 {
	crc := ^uint32(0)
	for i := 0; i < len(s); i++ {
		crc = crc32IEEETable[byte(crc)^s[i]] ^ (crc >> 8)
	}
	return int64(^crc)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
