package core

import (
	"hash/crc32"
	"strings"
	"testing"
)

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

// TestToString pins the strconv formatting against the fmt formatting it
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

// TestPHPCompare pins the comparison sort() orders by against php 8.4: every
