package shared

import (
	"strings"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
)

// Limits bounds what a hostile form can build. A zero field means PHP's
// default; negative means no limit.
type Limits struct {
	// MaxNesting is php.ini's max_input_nesting_level. A variable past it is
	// dropped whole rather than truncated.
	MaxNesting int

	// MaxVars is php.ini's max_input_vars.
	MaxVars int
}

const (
	DefaultMaxNesting = 64
	DefaultMaxVars    = 1000
)

func (l Limits) maxNesting() int {
	if l.MaxNesting == 0 {
		return DefaultMaxNesting
	}
	return l.MaxNesting
}

func (l Limits) maxVars() int {
	if l.MaxVars == 0 {
		return DefaultMaxVars
	}
	return l.MaxVars
}

// ParseStr decodes an application/x-www-form-urlencoded string into the nested
// array PHP builds from it. It backs parse_str(), $_GET and $_POST.
//
// Decoding happens before the brackets are read, so `k%5Ba%20b%5D=1` nests
// under the key `a b`.
func ParseStr(raw string, limits Limits) *model.Array {
	if raw == "" {
		return model.NewArray()
	}
	// PHP 8 splits on `&` only; the arg_separator that also honoured `;` is gone.
	fields := strings.Split(raw, "&")
	pairs := make([]Pair, 0, len(fields))
	for _, field := range fields {
		if field == "" {
			continue
		}
		// No `=` is a name with an empty value: `a&b=1` sets both.
		name, value, _ := strings.Cut(field, "=")
		pairs = append(pairs, Pair{Name: URLDecode(name), Value: URLDecode(value)})
	}
	return ParsePairs(pairs, limits)
}

// Pair is one form field, name and value already decoded.
type Pair struct {
	Name  string
	Value string
}

// ParsePairs is ParseStr for input that arrived as pairs: multipart values and
// the cookie header. Order is the order they are applied in, which decides the
// result when two name the same variable.
func ParsePairs(pairs []Pair, limits Limits) *model.Array {
	out := model.NewArray()
	maxVars, maxNesting := limits.maxVars(), limits.maxNesting()
	for i, pair := range pairs {
		if maxVars >= 0 && i >= maxVars {
			break
		}
		key, path, ok := parseKey(pair.Name, maxNesting)
		if !ok {
			continue
		}
		assign(out, key, path, pair.Value)
	}
	return out
}

// parseKey splits a decoded name into its top-level key and bracket path. The
// final return is false for a name PHP drops: empty, or nested past maxNesting.
//
// Two behaviours here look like bugs and are not. Mangling applies to the
// top-level name only, so `a.b` is the key `a_b` while `e[f.g]` nests under
// `f.g`. And anything after the last well-formed bracket is discarded rather
// than making the name invalid, so `a[b]c` and `a[b]]` are both `a[b]`.
//
// An unterminated *first* bracket is the exception: the name has no path at
// all and is mangled whole, so `a[b` is the flat key `a_b`.
func parseKey(name string, maxNesting int) (string, []string, bool) {
	open := strings.IndexByte(name, '[')
	if open < 0 || strings.IndexByte(name[open:], ']') < 0 {
		return mangle(name), nil, name != ""
	}
	top := mangle(name[:open])
	if top == "" {
		return "", nil, false
	}
	var path []string
	for open < len(name) && name[open] == '[' {
		end := strings.IndexByte(name[open:], ']')
		if end < 0 {
			break
		}
		end += open
		path = append(path, name[open+1:end])
		if maxNesting >= 0 && len(path) > maxNesting {
			return "", nil, false
		}
		open = end + 1
	}
	return top, path, true
}

// mangle substitutes in a top-level name. The set is exactly space, `.` and
// `[`; `a-b`, `a:b` and `a$b` survive as written. A `+` is already a space by
// now, which is why `a+b=1` is `a_b`.
func mangle(name string) string {
	if !strings.ContainsAny(name, " .[") {
		return name
	}
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '.' || r == '[' {
			return '_'
		}
		return r
	}, name)
}

// assign writes value at key/path, creating arrays along the way. The last
// assignment decides the shape: `a=1&a[b]=2` makes `a` an array, `a[b]=1&a=2`
// makes it a string again.
func assign(root *model.Array, key string, path []string, value string) {
	if len(path) == 0 {
		root.Set(phpval.Key(key), value)
		return
	}
	current := child(root, phpval.Key(key))
	for i, segment := range path {
		last := i == len(path)-1
		if segment == "" {
			if last {
				current.Append(value)
				return
			}
			next := model.NewArray()
			current.Append(next)
			current = next
			continue
		}
		k := phpval.Key(segment)
		if last {
			current.Set(k, value)
			return
		}
		current = child(current, k)
	}
}

// child returns the array at key, replacing a scalar in the way rather than
// merging into it: `x[a]=2&x[a][c]=3` leaves `x[a]` holding only `c`.
func child(parent *model.Array, key any) *model.Array {
	if existing, ok := parent.Get(key); ok {
		if arr, ok := existing.(*model.Array); ok {
			return arr
		}
	}
	next := model.NewArray()
	parent.Set(key, next)
	return next
}
