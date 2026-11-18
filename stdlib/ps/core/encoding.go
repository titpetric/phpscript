package core

import (
	"encoding/base64"
	"encoding/hex"
	"strings"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/modelutil"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/shared"
)

// init contributes the base64, hex, URL and query-string encoders to
// stdlib.Register.
func init() {
	runner.RegisterArea(`Runtime\PS\Core\Encoding`, registerEncoding)
}

func registerEncoding(rt *runner.Runtime) {
	// base64_encode returns $string encoded with the standard base64 alphabet and '=' padding.
	rt.RegisterFunc("base64_encode", phpBase64Encode)
	// base64_decode decodes the base64 in $string, skipping unknown
	// characters unless $strict is true, in which case it returns false for
	// those characters and for misplaced padding.
	rt.RegisterFunc("base64_decode", phpBase64Decode)
	// urlencode encodes $string for application/x-www-form-urlencoded. A
	// space becomes '+' and '~' becomes '%7E'.
	rt.RegisterFunc("urlencode", phpURLEncode)
	// urldecode decodes the application/x-www-form-urlencoded $string,
	// turning '+' into a space and leaving an incomplete '%' sequence
	// literal.
	rt.RegisterFunc("urldecode", phpURLDecode)
	// rawurlencode encodes $string per RFC 3986. A space becomes '%20' and '~' stays literal.
	rt.RegisterFunc("rawurlencode", phpRawURLEncode)
	// rawurldecode decodes the RFC 3986 $string, leaving '+' alone and
	// leaving an incomplete '%' sequence literal.
	rt.RegisterFunc("rawurldecode", phpRawURLDecode)
	// http_build_query joins $data into a query string, urlencoding both
	// halves of every pair and spelling a nested array as key[sub]=value; the
	// $numeric_prefix, $arg_separator and $encoding_type parameters are not
	// supported.
	rt.RegisterFunc("http_build_query", phpHTTPBuildQuery)
	// parse_str decodes the query string $string into $result, reading PHP's
	// bracket syntax so a[b]=1 arrives as a nested array; it inverts
	// http_build_query and is the decoder behind $_GET and $_POST.
	rt.RegisterFunc("parse_str", phpParseStr)
	// bin2hex returns $string spelled as lowercase hexadecimal, two digits per byte.
	rt.RegisterFunc("bin2hex", phpBin2hex)
	// hex2bin decodes the hexadecimal $string back into bytes, returning
	// false for an odd-length string or a non-hex character.
	rt.RegisterFunc("hex2bin", phpHex2bin)
}

func phpBin2hex(str string) string {
	return hex.EncodeToString([]byte(str))
}

// phpHex2bin returns false for odd length or non-hex input, as PHP's string|false does.
func phpHex2bin(str string) any {
	out, err := hex.DecodeString(str)
	if err != nil {
		return false
	}
	return string(out)
}

func phpBase64Encode(str string) string {
	return base64.StdEncoding.EncodeToString([]byte(str))
}

// base64Alphabet is the standard encoding's character set, in value order.
const base64Alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

// base64Values maps a byte to its base64 value, -1 for whitespace and -2 for the rest.
var base64Values = newBase64Values()

func newBase64Values() [256]int8 {
	var table [256]int8
	for i := range table {
		table[i] = -2
	}
	for _, c := range []byte(" \t\n\r\v\f") {
		table[c] = -1
	}
	for i := 0; i < len(base64Alphabet); i++ {
		table[base64Alphabet[i]] = int8(i)
	}
	return table
}

// phpBase64Decode returns false in strict mode, spelling out PHP's lenient decode.
func phpBase64Decode(str string, strict ...any) any {
	isStrict := len(strict) > 0 && phpval.Truthy(strict[0])

	out := make([]byte, 0, len(str)/4*3+3)
	var (
		count   int // alphabet characters consumed
		padding int
		acc     byte // the byte under construction
	)
	for i := 0; i < len(str); i++ {
		if str[i] == '=' {
			padding++
			continue
		}
		val := base64Values[str[i]]
		if val < 0 {
			// -1 is whitespace, which both modes skip; -2 is a character
			// outside the alphabet, which only strict mode rejects.
			if val == -2 && isStrict {
				return false
			}
			continue
		}
		if isStrict && padding > 0 {
			// Data after the padding.
			return false
		}
		switch count % 4 {
		case 0:
			acc = byte(val) << 2
		case 1:
			out = append(out, acc|byte(val)>>4)
			acc = (byte(val) & 0x0f) << 4
		case 2:
			out = append(out, acc|byte(val)>>2)
			acc = (byte(val) & 0x03) << 6
		case 3:
			out = append(out, acc|byte(val))
		}
		count++
	}
	if isStrict {
		// A trailing group of one character holds no whole byte. The
		// padding must complete the last group without overshooting it.
		if count%4 == 1 {
			return false
		}
		if padding > 0 && (padding > 2 || (count+padding)%4 != 0) {
			return false
		}
	}
	return string(out)
}

// urlFormSafe and urlRawSafe mark the bytes each encoder leaves literal.
var (
	urlFormSafe = newURLSafe("-_.")
	urlRawSafe  = newURLSafe("-_.~")
)

func newURLSafe(extra string) [256]bool {
	var table [256]bool
	for c := byte('a'); c <= 'z'; c++ {
		table[c] = true
	}
	for c := byte('A'); c <= 'Z'; c++ {
		table[c] = true
	}
	for c := byte('0'); c <= '9'; c++ {
		table[c] = true
	}
	for i := 0; i < len(extra); i++ {
		table[extra[i]] = true
	}
	return table
}

const hexDigits = "0123456789ABCDEF"

func phpURLEncode(str string) string { return urlEncode(str, true) }

func phpRawURLEncode(str string) string { return urlEncode(str, false) }

// urlEncode percent-escapes str, using '+' for a space in form mode.
func urlEncode(str string, form bool) string {
	safe := &urlRawSafe
	if form {
		safe = &urlFormSafe
	}
	escapes := 0
	for i := 0; i < len(str); i++ {
		if !safe[str[i]] {
			escapes++
		}
	}
	if escapes == 0 {
		return str
	}
	var b strings.Builder
	b.Grow(len(str) + 2*escapes)
	for i := 0; i < len(str); i++ {
		c := str[i]
		switch {
		case safe[c]:
			b.WriteByte(c)
		case form && c == ' ':
			b.WriteByte('+')
		default:
			b.WriteByte('%')
			b.WriteByte(hexDigits[c>>4])
			b.WriteByte(hexDigits[c&0x0f])
		}
	}
	return b.String()
}

// phpURLDecode decodes as shared.URLDecode, the rule the runner builds $_GET with.
func phpURLDecode(str string) string { return shared.URLDecode(str) }

func phpRawURLDecode(str string) string { return shared.RawURLDecode(str) }

// phpParseStr writes the decoded query string through the by-reference result setter.
func phpParseStr(str string, result func(any)) {
	if result == nil {
		return
	}
	result(shared.ParseStr(str, shared.Limits{}))
}

// phpHTTPBuildQuery joins data into a query string, reading any collection shape.
func phpHTTPBuildQuery(data any) string {
	var b strings.Builder
	if n, ok := modelutil.LenValues(data); ok {
		// Pairs run to roughly a key, a value, '=' and '&'.
		b.Grow(n * 16)
	}
	buildQuery(&b, "", data)
	return b.String()
}

// buildQuery appends data's pairs to b under prefix, nesting as key[sub].
func buildQuery(b *strings.Builder, prefix string, data any) {
	modelutil.RangeValues(data, func(key, val any) bool {
		name := phpval.String(key)
		if prefix != "" {
			name = prefix + "[" + name + "]"
		}
		switch {
		case val == nil:
			// PHP drops a null entry and emits no empty value.
		case modelutil.IsCollection(val):
			buildQuery(b, name, val)
		default:
			if b.Len() > 0 {
				b.WriteByte('&')
			}
			b.WriteString(urlEncode(name, true))
			b.WriteByte('=')
			b.WriteString(urlEncode(queryValue(val), true))
		}
		return true
	})
}

// queryValue renders a scalar as http_build_query does, false becoming "0".
func queryValue(v any) string {
	if b, ok := v.(bool); ok {
		if b {
			return "1"
		}
		return "0"
	}
	return phpval.String(v)
}
